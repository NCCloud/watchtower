package pkg

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"time"

	"dario.cat/mergo"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/nccloud/watchtower/pkg/apis/v1alpha2"
	"github.com/nccloud/watchtower/pkg/common"
	"github.com/puzpuzpuz/xsync/v3"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	cache2 "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/cache"
)

const (
	baseDelay = 100 * time.Millisecond
	maxDelay  = 15 * time.Minute
)

type WatcherManager interface {
	Add(ctx context.Context, watcher *v1alpha2.Watcher)
	Remove(ctx context.Context, watcher *v1alpha2.Watcher)
}

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

type watcherManager struct {
	cache    cache.Cache
	watchers *xsync.MapOf[string, *WatcherItem]
}

func NewManager(cache cache.Cache) WatcherManager {
	return &watcherManager{
		cache:    cache,
		watchers: xsync.NewMapOf[string, *WatcherItem](),
	}
}

func (m *watcherManager) Add(ctx context.Context, watcher *v1alpha2.Watcher) {
	var (
		logger         = slog.With("watcher", fmt.Sprintf("%s/%s", watcher.Namespace, watcher.Name))
		watcherID      = string(watcher.GetUID())
		processor      = NewProcessor(m.cache, watcher)
		sourceInstance = watcher.Spec.Source.NewInstance()
	)

	sourceInformer, getInformerErr := m.cache.GetInformer(ctx, sourceInstance)
	if getInformerErr != nil {
		logger.Error("Failed to get informer", "error", getInformerErr)

		return
	}

	if handleValuesFromErr := m.handleValuesFrom(ctx, watcher); handleValuesFromErr != nil {
		logger.Error("Failed to handle valuesFrom", "error", handleValuesFromErr)

		return
	}

	watcherItem := &WatcherItem{
		watcher: watcher.DeepCopy(),
		queue: workqueue.NewTypedRateLimitingQueue[WorkItem](
			workqueue.NewTypedItemExponentialFailureRateLimiter[WorkItem](baseDelay, maxDelay)),
		stopCh:     make(chan bool),
		processing: xsync.NewMapOf[string, bool](),
	}

	registration, addEventHandlerErr := sourceInformer.AddEventHandler(cache2.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			m.produceItem(watcherItem, obj, nil, logger)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			m.produceItem(watcherItem, newObj, oldObj, logger)
		},
	})
	if addEventHandlerErr != nil {
		logger.Error("Failed to add event handler", "error", addEventHandlerErr)

		return
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
					m.consumeItem(ctx, watcherItem, processor, logger)
				}
			}
		}()
	}
}

func (m *watcherManager) Remove(ctx context.Context, watcher *v1alpha2.Watcher) {
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

func (m *watcherManager) handleValuesFrom(ctx context.Context, watcher *v1alpha2.Watcher) error {
	for _, valuesFrom := range watcher.Spec.ValuesFrom {
		objectKey := client.ObjectKey{
			Name:      valuesFrom.Name,
			Namespace: watcher.Namespace,
		}

		var data []byte

		switch valuesFrom.Kind {
		case v1alpha2.ValuesFromKindSecret:
			secret := &corev1.Secret{}
			if getErr := m.cache.Get(ctx, objectKey, secret); getErr != nil {
				return getErr
			}

			secretData, keyExist := secret.Data[valuesFrom.Key]
			if !keyExist {
				return fmt.Errorf("key %s in secret %s not found", valuesFrom.Key, secret.Name)
			}

			data = secretData
		case v1alpha2.ValuesFromKindConfigMap:
			config := &corev1.ConfigMap{}
			if getErr := m.cache.Get(ctx, objectKey, config); getErr != nil {
				return getErr
			}

			configData, keyExist := config.Data[valuesFrom.Key]
			if !keyExist {
				return fmt.Errorf("key %s in configmap %s not found", valuesFrom.Key, config.Name)
			}

			data = []byte(configData)
		default:
			return fmt.Errorf("unsupported ValuesFrom kind %s", valuesFrom.Kind)
		}

		watcherToMerge := &v1alpha2.Watcher{}
		if unmarshalErr := yaml.Unmarshal(data, watcherToMerge); unmarshalErr != nil {
			return unmarshalErr
		}

		if mergeErr := mergo.Merge(watcher, watcherToMerge, mergo.WithOverride); mergeErr != nil {
			return mergeErr
		}
	}

	return nil
}

func (m *watcherManager) produceItem(watcherItem *WatcherItem, newObj interface{}, oldObj interface{},
	logger *slog.Logger,
) {
	var (
		oldObject *unstructured.Unstructured
		newObject = newObj.(*unstructured.Unstructured)
		objectID  = string(newObject.GetUID())
	)

	if _, isProcessing := watcherItem.processing.Load(objectID); isProcessing {
		return
	}

	if oldObj != nil {
		oldObject = oldObj.(*unstructured.Unstructured)
	}

	logger.Info("Object queued", "apiVersion", newObject.GetAPIVersion(),
		"kind", newObject.GetKind(), "name", newObject.GetName(), "namespace", newObject.GetNamespace())

	watcherItem.processing.Store(objectID, true)
	watcherItem.queue.Add(WorkItem{
		newObject: newObject,
		oldObject: oldObject,
	})
}

func (m *watcherManager) consumeItem(ctx context.Context, watcherItem *WatcherItem, processor WatcherProcessor,
	logger *slog.Logger,
) {
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
		slog.Info("Sending object",
			"name", watcherItem.watcher.GetName(), "namespace", watcherItem.watcher.GetNamespace())

		if sendErr := processor.Send(ctx, workItem.newObject); sendErr != nil {
			logger.Error("Error sending object", "error", sendErr,
				"name", workItem.newObject.GetName(), "namespace", workItem.newObject.GetNamespace())

			watcherItem.queue.AddRateLimited(workItem)

			return
		}
	}
}
