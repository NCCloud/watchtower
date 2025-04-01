package pkg

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"time"

	"github.com/nccloud/watchtower/pkg/apis/v1alpha2"
	"github.com/nccloud/watchtower/pkg/common"
	"github.com/puzpuzpuz/xsync/v3"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	cache2 "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"sigs.k8s.io/controller-runtime/pkg/cache"
)

const (
	baseDelay = 100 * time.Millisecond
	maxDelay  = 5 * time.Minute
)

type WatcherItem struct {
	watcher      *v1alpha2.Watcher
	queue        workqueue.TypedRateLimitingInterface[WorkItem]
	registration cache2.ResourceEventHandlerRegistration
	processing   *xsync.MapOf[string, bool]
	stopCh       chan bool
}

type WorkItem struct {
	newObject *unstructured.Unstructured
	oldObject *unstructured.Unstructured
}

type Manager struct {
	cache    cache.Cache
	watchers *xsync.MapOf[string, *WatcherItem]
}

func NewManager(cache cache.Cache) *Manager {
	return &Manager{
		cache:    cache,
		watchers: xsync.NewMapOf[string, *WatcherItem](),
	}
}

func (m *Manager) Add(ctx context.Context, watcher *v1alpha2.Watcher) {
	var (
		logger         = slog.With("watcher", fmt.Sprintf("%s/%s", watcher.Namespace, watcher.Name))
		processor      = NewProcessor(m.cache, watcher)
		watcherID      = string(watcher.GetUID())
		rateLimiter    = workqueue.NewTypedItemExponentialFailureRateLimiter[WorkItem](baseDelay, maxDelay)
		sourceInformer = common.MustReturn(m.cache.GetInformer(ctx, watcher.Spec.Source.NewInstance()))
	)

	watcherItem := &WatcherItem{
		watcher:    watcher.DeepCopy(),
		queue:      workqueue.NewTypedRateLimitingQueue[WorkItem](rateLimiter),
		stopCh:     make(chan bool),
		processing: xsync.NewMapOf[string, bool](),
	}

	registration, addEventHandlerErr := sourceInformer.AddEventHandler(cache2.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			produce(watcherItem, obj, nil, logger)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			produce(watcherItem, newObj, oldObj, logger)
		},
	})
	if addEventHandlerErr != nil {
		panic(addEventHandlerErr)
	}

	watcherItem.registration = registration
	m.watchers.Store(watcherID, watcherItem)

	for range watcher.Spec.Source.GetConcurrency() {
		go func() {
			for {
				select {
				case <-watcherItem.stopCh:
					return
				default:
					consume(ctx, watcherItem, processor, logger)
				}
			}
		}()
	}
}

func (m *Manager) Remove(ctx context.Context, watcher *v1alpha2.Watcher) {
	watcherID := string(watcher.GetUID())

	watcherItem, exists := m.watchers.Load(watcherID)
	if !exists {
		return
	}

	close(watcherItem.stopCh)

	watcherItem.queue.ShutDown()

	sourceInstance := watcher.Spec.Source.NewInstance()
	sourceInformer := common.MustReturn(m.cache.GetInformer(ctx, sourceInstance))

	common.Must(sourceInformer.RemoveEventHandler(watcherItem.registration))

	informerHasWatchers := false

	m.watchers.Range(func(key string, otherWatcherItem *WatcherItem) bool {
		if key == watcherID {
			return true
		}

		if reflect.DeepEqual(*watcherItem.watcher.Spec.Source.NewInstance(),
			*otherWatcherItem.watcher.Spec.Source.NewInstance()) {
			informerHasWatchers = true

			return false
		}

		return true
	})

	if !informerHasWatchers {
		slog.Info("Removing informer", "source", sourceInstance.GroupVersionKind().String())

		if removeInformerErr := m.cache.RemoveInformer(ctx, sourceInstance); removeInformerErr != nil {
			slog.Error("Failed to remove informer", "error", removeInformerErr)
		}
	}

	m.watchers.Delete(watcherID)
}

func produce(watcherItem *WatcherItem, newObj interface{}, oldObj interface{}, logger *slog.Logger) {
	var (
		oldObject *unstructured.Unstructured
		newObject = newObj.(*unstructured.Unstructured)
		objectID  = string(newObject.GetUID())
	)

	if _, isProcessing := watcherItem.processing.Load(objectID); isProcessing {
		return
	}

	if oldObj != nil {
		oldObject = newObj.(*unstructured.Unstructured)
	}

	logger.Info("Object queued", "apiVersion", newObject.GetAPIVersion(),
		"kind", newObject.GetKind(), "name", newObject.GetName(), "namespace", newObject.GetNamespace())

	watcherItem.processing.Store(objectID, true)
	watcherItem.queue.Add(WorkItem{
		newObject: newObject,
		oldObject: oldObject,
	})
}

func consume(ctx context.Context, watcherItem *WatcherItem, processor *Processor, logger *slog.Logger) {
	workItem, shutdown := watcherItem.queue.Get()
	if shutdown {
		return
	}

	var (
		objectID   = string(workItem.newObject.GetUID())
		filterPass bool
		filterErr  error
	)

	defer watcherItem.queue.Done(workItem)
	defer watcherItem.processing.Delete(objectID)

	filterPass, filterErr = processor.Filter(ctx, workItem.oldObject, workItem.newObject)
	if filterErr != nil {
		logger.Error("Error filtering object", "error", filterErr,
			"name", workItem.newObject.GetName(), "namespace", workItem.newObject.GetNamespace())
		watcherItem.queue.AddRateLimited(workItem)

		return
	}

	if filterPass {
		if sendErr := processor.Send(ctx, workItem.newObject); sendErr != nil {
			logger.Error("Error sending object", "error", filterErr,
				"name", workItem.newObject.GetName(), "namespace", workItem.newObject.GetNamespace())

			watcherItem.queue.AddRateLimited(workItem)

			return
		}
	}
}
