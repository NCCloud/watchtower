package pkg

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/nccloud/watchtower/mocks/github.com/nccloud/watchtower/pkg"
	cache3 "github.com/nccloud/watchtower/mocks/k8s.io/client-go/tools/cache"
	"github.com/nccloud/watchtower/mocks/sigs.k8s.io/controller-runtime/pkg/cache"
	mockCache "github.com/nccloud/watchtower/mocks/sigs.k8s.io/controller-runtime/pkg/cache"
	"github.com/nccloud/watchtower/pkg/apis/v1alpha2"
	"github.com/puzpuzpuz/xsync/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/util/workqueue"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestNewManager(t *testing.T) {
	// given
	mockCache := &mockCache.MockCache{}

	// when
	manager := NewManager(mockCache, nil)

	// then
	assert.NotNil(t, manager)
	assert.IsType(t, &watcherManager{}, manager)
}

func TestWatcherManager_Add(t *testing.T) {
	// given
	ctx := context.Background()
	watcher := &v1alpha2.Watcher{}
	watcher.SetUID("test-uid")
	watcher.Namespace = "test-namespace"
	watcher.Name = "test-name"
	watcher.Spec.Source = v1alpha2.Source{
		APIVersion: "v1",
		Kind:       "Pod",
	}

	mockCache := &mockCache.MockCache{}
	mockInformer := &cache.MockInformer{}
	mockRegistration := &cache3.MockResourceEventHandlerRegistration{}

	mockCache.On("GetInformer", mock.Anything,
		mock.AnythingOfType("*unstructured.Unstructured")).Return(mockInformer, nil)
	mockInformer.On("AddEventHandler",
		mock.AnythingOfType("ResourceEventHandlerFuncs")).Return(mockRegistration, nil)

	manager := &watcherManager{
		cache:    mockCache,
		watchers: xsync.NewMapOf[string, *WatcherItem](),
	}

	// when
	manager.Add(ctx, watcher)

	// then
	watcherItem, exists := manager.watchers.Load("test-uid")
	assert.True(t, exists)
	assert.Equal(t, watcher.Name, watcherItem.watcher.Name)
	assert.Equal(t, watcher.Namespace, watcherItem.watcher.Namespace)
	mockCache.AssertExpectations(t)
	mockInformer.AssertExpectations(t)
}

func TestWatcherManager_Add_WithValuesFrom_Secret(t *testing.T) {
	// given
	ctx := context.Background()
	watcher := &v1alpha2.Watcher{}
	watcher.SetUID("test-uid")
	watcher.Namespace = "test-namespace"
	watcher.Name = "test-name"
	watcher.Spec.Source = v1alpha2.Source{
		APIVersion: "v1",
	}
	watcher.Spec.ValuesFrom = []v1alpha2.ValuesFrom{
		{
			Kind: v1alpha2.ValuesFromKindSecret,
			Name: "test-secret",
			Key:  "test-key",
		},
	}

	secret := &corev1.Secret{}
	secret.Data = map[string][]byte{
		"test-key": []byte("spec:\n  source:\n    apiVersion: \"v1\"\n    kind: \"Pod\""),
	}

	mockCache := &mockCache.MockCache{}
	mockInformer := &cache.MockInformer{}
	mockRegistration := &cache3.MockResourceEventHandlerRegistration{}

	mockCache.On("Get", mock.Anything,
		client.ObjectKey{Name: "test-secret", Namespace: "test-namespace"},
		mock.AnythingOfType("*v1.Secret")).Run(func(args mock.Arguments) {
		arg := args.Get(2).(*corev1.Secret)
		*arg = *secret
	}).Return(nil)
	mockCache.On("GetInformer", mock.Anything,
		mock.AnythingOfType("*unstructured.Unstructured")).Return(mockInformer, nil)
	mockInformer.On("AddEventHandler",
		mock.AnythingOfType("ResourceEventHandlerFuncs")).Return(mockRegistration, nil)

	manager := &watcherManager{
		cache:    mockCache,
		watchers: xsync.NewMapOf[string, *WatcherItem](),
	}

	// when
	manager.Add(ctx, watcher)

	// then
	watcherItem, exists := manager.watchers.Load("test-uid")
	assert.True(t, exists)
	assert.Equal(t, watcher.Name, watcherItem.watcher.Name)
	assert.Equal(t, watcher.Namespace, watcherItem.watcher.Namespace)
	assert.Equal(t, watcher.Spec.Source.APIVersion, "v1")
	assert.Equal(t, watcher.Spec.Source.Kind, "Pod")
	mockCache.AssertExpectations(t)
	mockInformer.AssertExpectations(t)
}

func TestWatcherManager_Add_WithValuesFrom_ConfigMap(t *testing.T) {
	// given
	ctx := context.Background()
	watcher := &v1alpha2.Watcher{}
	watcher.SetUID("test-uid")
	watcher.Namespace = "test-namespace"
	watcher.Name = "test-name"
	watcher.Spec.Source = v1alpha2.Source{
		APIVersion: "v1",
	}
	watcher.Spec.ValuesFrom = []v1alpha2.ValuesFrom{
		{
			Kind: v1alpha2.ValuesFromKindConfigMap,
			Name: "test-configmap",
			Key:  "test-key",
		},
	}

	configMap := &corev1.ConfigMap{}
	configMap.Data = map[string]string{
		"test-key": "spec:\n  source:\n    apiVersion: \"v1\"\n    kind: \"Pod\"",
	}

	mockCache := &mockCache.MockCache{}
	mockInformer := &cache.MockInformer{}
	mockRegistration := &cache3.MockResourceEventHandlerRegistration{}

	mockCache.On("Get", mock.Anything, client.ObjectKey{Name: "test-configmap", Namespace: "test-namespace"},
		mock.AnythingOfType("*v1.ConfigMap")).Run(func(args mock.Arguments) {
		arg := args.Get(2).(*corev1.ConfigMap)
		*arg = *configMap
	}).Return(nil)
	mockCache.On("GetInformer", mock.Anything,
		mock.AnythingOfType("*unstructured.Unstructured")).Return(mockInformer, nil)
	mockInformer.On("AddEventHandler",
		mock.AnythingOfType("ResourceEventHandlerFuncs")).Return(mockRegistration, nil)

	manager := &watcherManager{
		cache:    mockCache,
		watchers: xsync.NewMapOf[string, *WatcherItem](),
	}

	// when
	manager.Add(ctx, watcher)

	// then
	watcherItem, exists := manager.watchers.Load("test-uid")
	assert.True(t, exists)
	assert.Equal(t, watcher.Name, watcherItem.watcher.Name)
	assert.Equal(t, watcher.Namespace, watcherItem.watcher.Namespace)
	assert.Equal(t, watcher.Spec.Source.APIVersion, "v1")
	assert.Equal(t, watcher.Spec.Source.Kind, "Pod")
	mockCache.AssertExpectations(t)
	mockInformer.AssertExpectations(t)
}

func TestWatcherManager_Add_WithValuesFrom_UnsupportedKind(t *testing.T) {
	// given
	ctx := context.Background()
	watcher := &v1alpha2.Watcher{}
	watcher.SetUID("test-uid")
	watcher.Namespace = "test-namespace"
	watcher.Name = "test-name"
	watcher.Spec.ValuesFrom = []v1alpha2.ValuesFrom{
		{
			Kind: "UnsupportedKind",
			Name: "test-name",
			Key:  "test-key",
		},
	}

	mockCache := &mockCache.MockCache{}
	mockInformer := &cache.MockInformer{}

	mockCache.On("GetInformer", mock.Anything,
		mock.AnythingOfType("*unstructured.Unstructured")).Return(mockInformer, nil)

	manager := &watcherManager{
		cache:    mockCache,
		watchers: xsync.NewMapOf[string, *WatcherItem](),
	}

	// when
	manager.Add(ctx, watcher)

	// then
	_, exists := manager.watchers.Load("test-uid")
	assert.False(t, exists)
	mockCache.AssertExpectations(t)
}

func TestWatcherManager_Remove(t *testing.T) {
	// given
	ctx := context.Background()
	watcher := &v1alpha2.Watcher{}
	watcher.SetUID("test-uid")
	watcher.Spec.Source = v1alpha2.Source{
		APIVersion: "v1",
		Kind:       "Pod",
	}

	mockCache := &mockCache.MockCache{}
	mockInformer := &cache.MockInformer{}
	mockRegistration := &cache3.MockResourceEventHandlerRegistration{}

	mockCache.On("GetInformer", mock.Anything, mock.AnythingOfType("*unstructured.Unstructured")).Return(mockInformer, nil)
	mockInformer.On("RemoveEventHandler", mockRegistration).Return(nil)
	mockCache.On("RemoveInformer", mock.Anything, mock.AnythingOfType("*unstructured.Unstructured")).Return(nil)

	watcherItem := &WatcherItem{
		watcher: watcher.DeepCopy(),
		queue: workqueue.NewTypedRateLimitingQueue[WorkItem](
			workqueue.NewTypedItemExponentialFailureRateLimiter[WorkItem](baseDelay, maxDelay)),
		registration: mockRegistration,
		stopCh:       make(chan bool),
		processing:   xsync.NewMapOf[string, bool](),
	}

	watchers := xsync.NewMapOf[string, *WatcherItem]()
	watchers.Store("test-uid", watcherItem)

	manager := &watcherManager{
		cache:    mockCache,
		watchers: watchers,
	}

	// when
	manager.Remove(ctx, watcher)

	// then
	_, exists := manager.watchers.Load("test-uid")
	assert.False(t, exists)
	mockCache.AssertExpectations(t)
	mockInformer.AssertExpectations(t)
}

func TestWatcherManager_Remove_WithMultipleWatchers(t *testing.T) {
	// given
	ctx := context.Background()
	watcher1 := &v1alpha2.Watcher{}
	watcher1.SetUID("test-uid-1")
	watcher1.Spec.Source = v1alpha2.Source{
		APIVersion: "v1",
		Kind:       "Pod",
	}

	watcher2 := &v1alpha2.Watcher{}
	watcher2.SetUID("test-uid-2")
	watcher2.Spec.Source = v1alpha2.Source{
		APIVersion: "v1",
		Kind:       "Pod",
	}

	mockCache := &mockCache.MockCache{}
	mockInformer := &cache.MockInformer{}
	mockRegistration := &cache3.MockResourceEventHandlerRegistration{}

	mockCache.On("GetInformer", mock.Anything, mock.AnythingOfType("*unstructured.Unstructured")).Return(mockInformer, nil)
	mockInformer.On("RemoveEventHandler", mockRegistration).Return(nil)

	watcherItem1 := &WatcherItem{
		watcher: watcher1.DeepCopy(),
		queue: workqueue.NewTypedRateLimitingQueue[WorkItem](
			workqueue.NewTypedItemExponentialFailureRateLimiter[WorkItem](baseDelay, maxDelay)),
		registration: mockRegistration,
		stopCh:       make(chan bool),
		processing:   xsync.NewMapOf[string, bool](),
	}

	watcherItem2 := &WatcherItem{
		watcher: watcher2.DeepCopy(),
		queue: workqueue.NewTypedRateLimitingQueue[WorkItem](
			workqueue.NewTypedItemExponentialFailureRateLimiter[WorkItem](baseDelay, maxDelay)),
		registration: mockRegistration,
		stopCh:       make(chan bool),
		processing:   xsync.NewMapOf[string, bool](),
	}

	watchers := xsync.NewMapOf[string, *WatcherItem]()
	watchers.Store("test-uid-1", watcherItem1)
	watchers.Store("test-uid-2", watcherItem2)

	manager := &watcherManager{
		cache:    mockCache,
		watchers: watchers,
	}

	// when
	manager.Remove(ctx, watcher1)

	// then
	_, exists1 := manager.watchers.Load("test-uid-1")
	_, exists2 := manager.watchers.Load("test-uid-2")
	assert.False(t, exists1)
	assert.True(t, exists2)
	mockCache.AssertExpectations(t)
	mockInformer.AssertExpectations(t)
}

func TestWatcherManager_ProduceItem(t *testing.T) {
	// given
	logger := slog.With()
	watcher := &v1alpha2.Watcher{}
	watcher.SetUID("test-uid")

	newObj := &unstructured.Unstructured{}
	newObj.SetUID("obj-uid")
	newObj.SetName("test-obj")
	newObj.SetNamespace("test-namespace")

	watcherItem := &WatcherItem{
		watcher: watcher,
		queue: workqueue.NewTypedRateLimitingQueue[WorkItem](
			workqueue.NewTypedItemExponentialFailureRateLimiter[WorkItem](baseDelay, maxDelay)),
		processing: xsync.NewMapOf[string, bool](),
	}

	manager := &watcherManager{}

	// when
	manager.produceItem(watcherItem, newObj, nil, logger)

	// then
	assert.Equal(t, 1, watcherItem.queue.Len())
	isProcessing, _ := watcherItem.processing.Load("obj-uid")
	assert.True(t, isProcessing)
}

func TestWatcherManager_ConsumeItem_Success(t *testing.T) {
	// given
	ctx := context.Background()
	logger := slog.With()
	watcher := &v1alpha2.Watcher{}
	watcher.SetUID("test-uid")

	newObj := &unstructured.Unstructured{}
	newObj.SetUID("obj-uid")

	watcherItem := &WatcherItem{
		watcher: watcher,
		queue: workqueue.NewTypedRateLimitingQueue[WorkItem](
			workqueue.NewTypedItemExponentialFailureRateLimiter[WorkItem](baseDelay, maxDelay)),
		processing: xsync.NewMapOf[string, bool](),
	}
	watcherItem.processing.Store("obj-uid", true)
	watcherItem.queue.Add(WorkItem{
		newObject: newObj,
	})

	mockProcessor := &pkg.MockWatcherProcessor{}
	mockProcessor.On("Filter", mock.Anything, mock.Anything, newObj).Return(true, nil)
	mockProcessor.On("Send", mock.Anything, newObj).Return(nil)

	manager := &watcherManager{}

	// when
	manager.consumeItem(ctx, watcherItem, mockProcessor, logger)

	// then
	assert.Equal(t, 0, watcherItem.queue.Len())
	_, exists := watcherItem.processing.Load("obj-uid")
	assert.False(t, exists)
	mockProcessor.AssertExpectations(t)
}

func TestWatcherManager_ConsumeItem_FilterFail(t *testing.T) {
	// given
	ctx := context.Background()
	logger := slog.With()
	watcher := &v1alpha2.Watcher{}
	watcher.SetUID("test-uid")

	newObj := &unstructured.Unstructured{}
	newObj.SetUID("obj-uid")
	newObj.SetName("test-obj")
	newObj.SetNamespace("test-namespace")

	watcherItem := &WatcherItem{
		watcher: watcher,
		queue: workqueue.NewTypedRateLimitingQueue[WorkItem](
			workqueue.NewTypedItemExponentialFailureRateLimiter[WorkItem](baseDelay, maxDelay)),
		processing: xsync.NewMapOf[string, bool](),
	}
	watcherItem.processing.Store("obj-uid", true)
	watcherItem.queue.Add(WorkItem{
		newObject: newObj,
	})

	mockProcessor := &pkg.MockWatcherProcessor{}
	filterErr := fmt.Errorf("filter error")
	mockProcessor.On("Filter", mock.Anything, mock.Anything, newObj).Return(false, filterErr)

	manager := &watcherManager{}

	// when
	manager.consumeItem(ctx, watcherItem, mockProcessor, logger)

	// then
	assert.Eventually(t, func() bool {
		count := watcherItem.queue.Len()
		_, exists := watcherItem.processing.Load("obj-uid")
		return !exists && count == 1
	}, time.Second*5, time.Millisecond)
	mockProcessor.AssertExpectations(t)
}

func TestWatcherManager_ConsumeItem_SendFail(t *testing.T) {
	// given
	ctx := context.Background()
	logger := slog.With()
	watcher := &v1alpha2.Watcher{}
	watcher.SetUID("test-uid")

	newObj := &unstructured.Unstructured{}
	newObj.SetUID("obj-uid")
	newObj.SetName("test-obj")
	newObj.SetNamespace("test-namespace")

	watcherItem := &WatcherItem{
		watcher: watcher,
		queue: workqueue.NewTypedRateLimitingQueue[WorkItem](
			workqueue.NewTypedItemExponentialFailureRateLimiter[WorkItem](baseDelay, maxDelay)),
		processing: xsync.NewMapOf[string, bool](),
	}
	watcherItem.processing.Store("obj-uid", true)
	watcherItem.queue.Add(WorkItem{
		newObject: newObj,
	})

	mockProcessor := &pkg.MockWatcherProcessor{}
	sendErr := fmt.Errorf("send error")
	mockProcessor.On("Filter", mock.Anything, mock.Anything, newObj).Return(true, nil)
	mockProcessor.On("Send", mock.Anything, newObj).Return(sendErr)

	manager := &watcherManager{}

	// when
	manager.consumeItem(ctx, watcherItem, mockProcessor, logger)

	// then
	assert.Eventually(t, func() bool {
		count := watcherItem.queue.Len()
		_, exists := watcherItem.processing.Load("obj-uid")
		return !exists && count == 1
	}, time.Second*5, time.Millisecond)
	mockProcessor.AssertExpectations(t)
}

func TestWatcherManager_HandleValuesFrom_Secret(t *testing.T) {
	// given
	ctx := context.Background()
	watcher := &v1alpha2.Watcher{}
	watcher.Namespace = "test-namespace"
	watcher.Spec.ValuesFrom = []v1alpha2.ValuesFrom{
		{
			Kind: v1alpha2.ValuesFromKindSecret,
			Name: "test-secret",
			Key:  "test-key",
		},
	}

	secret := &corev1.Secret{}
	secret.Data = map[string][]byte{
		"test-key": []byte(`spec:
  source:
    apiVersion: "v1"
    kind: "Pod"`),
	}

	mockCache := &mockCache.MockCache{}
	mockCache.On("Get", mock.Anything, client.ObjectKey{Name: "test-secret", Namespace: "test-namespace"},
		mock.AnythingOfType("*v1.Secret")).Run(func(args mock.Arguments) {
		arg := args.Get(2).(*corev1.Secret)
		*arg = *secret
	}).Return(nil)

	manager := &watcherManager{
		cache: mockCache,
	}

	// when
	err := manager.handleValuesFrom(ctx, watcher)

	// then
	require.NoError(t, err)
	assert.Equal(t, "v1", watcher.Spec.Source.APIVersion)
	assert.Equal(t, "Pod", watcher.Spec.Source.Kind)
	mockCache.AssertExpectations(t)
}

func TestWatcherManager_HandleValuesFrom_ConfigMap(t *testing.T) {
	// given
	ctx := context.Background()
	watcher := &v1alpha2.Watcher{}
	watcher.Namespace = "test-namespace"
	watcher.Spec.ValuesFrom = []v1alpha2.ValuesFrom{
		{
			Kind: v1alpha2.ValuesFromKindConfigMap,
			Name: "test-configmap",
			Key:  "test-key",
		},
	}

	configMap := &corev1.ConfigMap{}
	configMap.Data = map[string]string{
		"test-key": `spec:
  source:
    apiVersion: "v1"
    kind: "Pod"`,
	}

	mockCache := &mockCache.MockCache{}
	mockCache.On("Get", mock.Anything, client.ObjectKey{Name: "test-configmap", Namespace: "test-namespace"}, mock.AnythingOfType("*v1.ConfigMap")).Run(func(args mock.Arguments) {
		arg := args.Get(2).(*corev1.ConfigMap)
		*arg = *configMap
	}).Return(nil)

	manager := &watcherManager{
		cache: mockCache,
	}

	// when
	err := manager.handleValuesFrom(ctx, watcher)

	// then
	require.NoError(t, err)
	assert.Equal(t, "v1", watcher.Spec.Source.APIVersion)
	assert.Equal(t, "Pod", watcher.Spec.Source.Kind)
	mockCache.AssertExpectations(t)
}

func TestWatcherManager_HandleValuesFrom_SecretKeyNotFound(t *testing.T) {
	// given
	ctx := context.Background()
	watcher := &v1alpha2.Watcher{}
	watcher.Namespace = "test-namespace"
	watcher.Spec.ValuesFrom = []v1alpha2.ValuesFrom{
		{
			Kind: v1alpha2.ValuesFromKindSecret,
			Name: "test-secret",
			Key:  "test-key",
		},
	}

	secret := &corev1.Secret{}
	secret.Data = map[string][]byte{
		"wrong-key": []byte(`test`),
	}

	mockCache := &mockCache.MockCache{}
	mockCache.On("Get", mock.Anything, client.ObjectKey{Name: "test-secret", Namespace: "test-namespace"},
		mock.AnythingOfType("*v1.Secret")).Run(func(args mock.Arguments) {
		arg := args.Get(2).(*corev1.Secret)
		*arg = *secret
	}).Return(nil)

	manager := &watcherManager{
		cache: mockCache,
	}

	// when
	err := manager.handleValuesFrom(ctx, watcher)

	// then
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
	mockCache.AssertExpectations(t)
}

func TestWatcherManager_HandleValuesFrom_ConfigMapKeyNotFound(t *testing.T) {
	// given
	ctx := context.Background()
	watcher := &v1alpha2.Watcher{}
	watcher.Namespace = "test-namespace"
	watcher.Spec.ValuesFrom = []v1alpha2.ValuesFrom{
		{
			Kind: v1alpha2.ValuesFromKindConfigMap,
			Name: "test-configmap",
			Key:  "test-key",
		},
	}

	configMap := &corev1.ConfigMap{}
	configMap.Data = map[string]string{
		"wrong-key": "test",
	}

	mockCache := &mockCache.MockCache{}
	mockCache.On("Get", mock.Anything, client.ObjectKey{Name: "test-configmap", Namespace: "test-namespace"}, mock.AnythingOfType("*v1.ConfigMap")).Run(func(args mock.Arguments) {
		arg := args.Get(2).(*corev1.ConfigMap)
		*arg = *configMap
	}).Return(nil)

	manager := &watcherManager{
		cache: mockCache,
	}

	// when
	err := manager.handleValuesFrom(ctx, watcher)

	// then
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
	mockCache.AssertExpectations(t)
}

func TestWatcherManager_HandleValuesFrom_UnsupportedKind(t *testing.T) {
	// given
	ctx := context.Background()
	watcher := &v1alpha2.Watcher{}
	watcher.Spec.ValuesFrom = []v1alpha2.ValuesFrom{
		{
			Kind: "UnsupportedKind",
			Name: "test-name",
			Key:  "test-key",
		},
	}

	manager := &watcherManager{}

	// when
	err := manager.handleValuesFrom(ctx, watcher)

	// then
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported ValuesFrom kind")
}
