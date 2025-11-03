package manager

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"dario.cat/mergo"
	"github.com/nccloud/watchtower/pkg/apis/v1alpha2"
	"github.com/nccloud/watchtower/pkg/common"
	"github.com/nccloud/watchtower/pkg/processor"
	"github.com/puzpuzpuz/xsync/v3"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	cache2 "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	baseDelay = 100 * time.Millisecond
	maxDelay  = 15 * time.Minute
)

type Manager interface {
	Add(ctx context.Context, watcher *v1alpha2.Watcher)
	Delete(ctx context.Context, watcher *v1alpha2.Watcher)
	Update(ctx context.Context, watcher *v1alpha2.Watcher)
}

func New(logger *slog.Logger, cache cache.Cache, client client.Client) Manager {
	return &manager{
		logger:   logger,
		cache:    cache,
		client:   client,
		watchers: xsync.NewMapOf[string, *Watcher](),
	}
}

func (m *manager) Add(ctx context.Context, watcher *v1alpha2.Watcher) {
	var (
		logger         = m.logger.With("watcher", fmt.Sprintf("%s/%s", watcher.Name, watcher.Namespace))
		watcherID      = string(watcher.GetUID())
		sourceInstance = watcher.Spec.Source.NewInstance()
	)

	processorInstance, processorErr := processor.New(m.cache, m.client, watcher)
	if processorErr != nil {
		logger.Error("Failed to add watcher", "error", processorErr)

		return
	}

	informer, getInformerErr := m.cache.GetInformer(ctx, sourceInstance)
	if getInformerErr != nil {
		logger.Error("Failed to get informer", "error", getInformerErr)

		return
	}

	if handleValuesFromErr := m.mergeValuesFrom(ctx, watcher); handleValuesFromErr != nil {
		logger.Error("Failed to handle valuesFrom", "error", handleValuesFromErr)

		return
	}

	watcherItem := &Watcher{
		workqueue: workqueue.NewTypedRateLimitingQueue[WorkqueueItem](
			workqueue.NewTypedItemExponentialFailureRateLimiter[WorkqueueItem](baseDelay, maxDelay)),
		stopCh: make(chan bool),
	}

	registration, addEventHandlerErr := informer.AddEventHandler(cache2.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			watcherItem.workqueue.Add(WorkqueueItem{
				eventType: EventTypeCreate,
				newObject: obj.(*unstructured.Unstructured).DeepCopy(),
			})
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			watcherItem.workqueue.Add(WorkqueueItem{
				eventType: EventTypeUpdate,
				oldObject: oldObj.(*unstructured.Unstructured).DeepCopy(),
				newObject: newObj.(*unstructured.Unstructured).DeepCopy(),
			})
		},
		DeleteFunc: func(obj interface{}) {
			watcherItem.workqueue.Add(WorkqueueItem{
				eventType: EventTypeDelete,
				newObject: obj.(*unstructured.Unstructured).DeepCopy(),
			})
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
					workqueueItem, shutdown := watcherItem.workqueue.Get()
					if shutdown {
						return
					}

					objLogger := m.logger.With("watcher", fmt.Sprintf("%s/%s",
						watcher.GetName(), watcher.GetNamespace()), "eventType", string(workqueueItem.eventType),
						"object", fmt.Sprintf("%s/%s", workqueueItem.newObject.GetName(),
							workqueueItem.newObject.GetNamespace()))

					processErr := processorInstance.Process(ctx,
						string(workqueueItem.eventType), workqueueItem.oldObject, workqueueItem.newObject)
					if processErr != nil {
						objLogger.Error("Error processing workqueue item.", "error", processErr)

						watcherItem.workqueue.AddRateLimited(workqueueItem)
					} else {
						objLogger.Info("Workqueue item processed.")
					}

					watcherItem.workqueue.Done(workqueueItem)
				}
			}
		}()
	}
}

func (m *manager) Delete(ctx context.Context, watcher *v1alpha2.Watcher) {
	var (
		logger    = m.logger.With("watcher", fmt.Sprintf("%s/%s", watcher.Name, watcher.Namespace))
		watcherID = string(watcher.GetUID())
	)

	watcherItem, exists := m.watchers.Load(watcherID)
	if !exists {
		return
	}

	close(watcherItem.stopCh)

	watcherItem.workqueue.ShutDown()

	sourceInstance := watcher.Spec.Source.NewInstance()
	sourceInformer := common.MustReturn(m.cache.GetInformer(ctx, sourceInstance))
	common.Must(sourceInformer.RemoveEventHandler(watcherItem.registration))

	m.watchers.Delete(watcherID)

	logger.Info("Watcher removed.")
}

func (m *manager) Update(ctx context.Context, watcher *v1alpha2.Watcher) {
	m.Delete(ctx, watcher)
	m.Add(ctx, watcher)
}

func (m *manager) mergeValuesFrom(ctx context.Context, watcher *v1alpha2.Watcher) error {
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
