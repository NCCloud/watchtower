package pkg

import (
	"context"
	"github.com/go-logr/logr"
	http2 "github.com/nccloud/watchtower/mocks/net/http"
	cache2 "github.com/nccloud/watchtower/mocks/sigs.k8s.io/controller-runtime/pkg/cache"
	client2 "github.com/nccloud/watchtower/mocks/sigs.k8s.io/controller-runtime/pkg/client"
	"github.com/nccloud/watchtower/mocks/sigs.k8s.io/controller-runtime/pkg/manager"
	"github.com/nccloud/watchtower/pkg/apis/v1alpha1"
	"github.com/nccloud/watchtower/pkg/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"io"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	"net/http"
	"reflect"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/config"
	controller2 "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"testing"
	"time"
)

var testVars = struct {
	scheme    *runtime.Scheme
	config    *common.Config
	k8sClient client.Client
	logger    logr.Logger
	watcher   *v1alpha1.Watcher
}{
	scheme: runtime.NewScheme(),
	config: common.NewConfig(),
	logger: zap.New(),
}

func init() {
	ctrl.SetLogger(testVars.logger)
	utilruntime.Must(clientgoscheme.AddToScheme(testVars.scheme))
	utilruntime.Must(v1alpha1.AddToScheme(testVars.scheme))

	//kubeConfig, testEnvStartErr := (&envtest.Environment{
	//	ControlPlane: envtest.ControlPlane{
	//		APIServer: &envtest.APIServer{
	//			StartTimeout: 5 * time.Minute,
	//			StopTimeout:  5 * time.Minute,
	//		},
	//		Etcd: &envtest.Etcd{
	//			StartTimeout: 5 * time.Minute,
	//			StopTimeout:  5 * time.Minute,
	//		},
	//	},
	//	ErrorIfCRDPathMissing: true,
	//	CRDDirectoryPaths: []string{
	//		filepath.Join("..", "deploy", "crds"), filepath.Join("..", ".envtest", "crds"),
	//	},
	//	BinaryAssetsDirectory:    "../.envtest/bins",
	//	ControlPlaneStartTimeout: 5 * time.Minute,
	//	ControlPlaneStopTimeout:  5 * time.Minute,
	//}).Start()
	//if testEnvStartErr != nil {
	//	panic(testEnvStartErr)
	//}

	//manager, managerErr := ctrl.NewManager(kubeConfig, ctrl.Options{
	//	Scheme: testVars.scheme,
	//	Metrics: server.Options{
	//		BindAddress: ":0",
	//	},
	//	Logger: testVars.logger,
	//})
	//if managerErr != nil {
	//	panic(managerErr)
	//}
	//
	//if setupErr := (&Controller{
	//	client:  manager.GetClient(),
	//	watcher: testVars.watcher,
	//}).SetupWithManager(manager); setupErr != nil {
	//	panic(setupErr)
	//}
	//
	//testVars.k8sClient = manager.GetClient()

	//go func() {
	//	if managerStartErr := manager.Start(context.Background()); managerStartErr != nil {
	//		panic(managerStartErr)
	//	}
	//}()
}

func TestController_New(t *testing.T) {
	// given
	var (
		watcher          = (&v1alpha1.Watcher{}).Compile()
		mockClient       = new(client2.MockClient)
		mockRoundTripper = new(http2.MockRoundTripper)
	)

	// when
	controller := NewController(mockClient, &http.Client{Transport: mockRoundTripper}, watcher)

	// then
	assert.NotNil(t, controller)
	assert.IsType(t, controller, &Controller{})
}

func TestController_Reconcile(t *testing.T) {
	// given
	var (
		ctx     = context.Background()
		watcher = (&v1alpha1.Watcher{
			Spec: v1alpha1.WatcherSpec{
				Filter: v1alpha1.Filter{
					Object: v1alpha1.ObjectFilter{
						Name:      ptr.To(".*my.*"),
						Namespace: ptr.To(".*my.*"),
						Labels: ptr.To(map[string]string{
							"my-label": "true",
						}),
						Annotations: ptr.To(map[string]string{
							"my-annotation": "true",
						}),
						Custom: &v1alpha1.ObjectFilterCustom{
							Template: "{{ index .data \"my-key\" }}",
							Result:   "my-value",
						},
					},
				},
				Destination: v1alpha1.Destination{
					URLTemplate:  "www.test.com/{{ index .data \"my-key\" }}-in-url",
					BodyTemplate: "{{ index .data \"my-key\" }}-in-template",
					Method:       "POST",
					Headers: map[string][]string{
						"Content-Type": {"application/custom"},
					},
				},
			},
		}).Compile()
		mockClient       = new(client2.MockClient)
		mockRoundTripper = new(http2.MockRoundTripper)
		controller       = NewController(mockClient, &http.Client{Transport: mockRoundTripper}, watcher)
	)

	mockRoundTripper.EXPECT().RoundTrip(mock.Anything).Return(&http.Response{StatusCode: 200}, nil)

	// when
	secret := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"type":       "Opaque",
			"metadata": map[string]interface{}{
				"name":      "my-secret",
				"namespace": "my-namespace",
				"labels": map[string]interface{}{
					"my-label": "true",
				},
				"annotations": map[string]interface{}{
					"my-annotation": "true",
				},
			},
			"data": map[string]interface{}{
				"my-key": "my-value",
			},
		},
	}

	result, reconcileErr := controller.Reconcile(ctx, secret)

	// then
	assert.Nil(t, reconcileErr)
	assert.False(t, result.Requeue)
	mockRoundTripper.AssertCalled(t, "RoundTrip", mock.MatchedBy(func(r *http.Request) bool {
		urlMatched := reflect.DeepEqual(r.URL.String(), "www.test.com/my-value-in-url")
		headerMatched := reflect.DeepEqual(r.Header.Get("Content-Type"), "application/custom") && len(r.Header) == 1
		methodMatched := reflect.DeepEqual(r.Method, "POST")
		body, _ := io.ReadAll(r.Body)
		bodyMatched := string(body) == "my-value-in-template"
		return headerMatched && methodMatched && bodyMatched && urlMatched
	}))
}

func TestController_Reconcile_FilterByObjectName(t *testing.T) {
	// given
	var (
		ctx              = context.Background()
		mockClient       = new(client2.MockClient)
		mockRoundTripper = new(http2.MockRoundTripper)
		watcher          = (&v1alpha1.Watcher{
			Spec: v1alpha1.WatcherSpec{
				Filter: v1alpha1.Filter{
					Object: v1alpha1.ObjectFilter{
						Name: ptr.To("non-related-name"),
					},
				},
			},
		}).Compile()
		controller = NewController(mockClient, &http.Client{Transport: mockRoundTripper}, watcher)
	)

	// when
	secret := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{"name": "my-secret"},
		},
	}
	result, reconcileErr := controller.Reconcile(ctx, secret)

	// then
	mockRoundTripper.AssertNotCalled(t, "RoundTrip")
	assert.Nil(t, reconcileErr)
	assert.False(t, result.Requeue)
}

func TestController_Reconcile_FilterByObjectNamespace(t *testing.T) {
	// given
	var (
		ctx              = context.Background()
		mockClient       = new(client2.MockClient)
		mockRoundTripper = new(http2.MockRoundTripper)
		watcher          = (&v1alpha1.Watcher{
			Spec: v1alpha1.WatcherSpec{
				Filter: v1alpha1.Filter{
					Object: v1alpha1.ObjectFilter{
						Namespace: ptr.To("non-related-name"),
					},
				},
			},
		}).Compile()
		controller = NewController(mockClient, &http.Client{Transport: mockRoundTripper}, watcher)
	)

	// when
	secret := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{"namespace": "my-secret"},
		},
	}
	result, reconcileErr := controller.Reconcile(ctx, secret)

	// then
	mockRoundTripper.AssertNotCalled(t, "RoundTrip")
	assert.Nil(t, reconcileErr)
	assert.False(t, result.Requeue)
}

func TestController_Reconcile_FilterByLabels(t *testing.T) {
	// given
	var (
		ctx              = context.Background()
		mockClient       = new(client2.MockClient)
		mockRoundTripper = new(http2.MockRoundTripper)
		watcher          = (&v1alpha1.Watcher{
			Spec: v1alpha1.WatcherSpec{
				Filter: v1alpha1.Filter{
					Object: v1alpha1.ObjectFilter{
						Labels: ptr.To(map[string]string{
							"non-related-label-key": "non-related-label-value",
						}),
					},
				},
			},
		}).Compile()
		controller = NewController(mockClient, &http.Client{Transport: mockRoundTripper}, watcher)
	)

	// when
	secret := &unstructured.Unstructured{
		Object: map[string]interface{}{},
	}
	result, reconcileErr := controller.Reconcile(ctx, secret)

	// then
	mockRoundTripper.AssertNotCalled(t, "RoundTrip")
	assert.Nil(t, reconcileErr)
	assert.False(t, result.Requeue)
}

func TestController_Reconcile_FilterByAnnotations(t *testing.T) {
	// given
	var (
		ctx              = context.Background()
		mockClient       = new(client2.MockClient)
		mockRoundTripper = new(http2.MockRoundTripper)
		watcher          = (&v1alpha1.Watcher{
			Spec: v1alpha1.WatcherSpec{
				Filter: v1alpha1.Filter{
					Object: v1alpha1.ObjectFilter{
						Annotations: ptr.To(map[string]string{
							"non-related-label-key": "non-related-label-value",
						}),
					},
				},
			},
		}).Compile()
		controller = NewController(mockClient, &http.Client{Transport: mockRoundTripper}, watcher)
	)

	// when
	secret := &unstructured.Unstructured{
		Object: map[string]interface{}{},
	}
	result, reconcileErr := controller.Reconcile(ctx, secret)

	// then
	mockRoundTripper.AssertNotCalled(t, "RoundTrip")
	assert.Nil(t, reconcileErr)
	assert.False(t, result.Requeue)
}

func TestController_Reconcile_FilterByCustom(t *testing.T) {
	// given
	var (
		ctx              = context.Background()
		mockClient       = new(client2.MockClient)
		mockRoundTripper = new(http2.MockRoundTripper)
		watcher          = (&v1alpha1.Watcher{
			Spec: v1alpha1.WatcherSpec{
				Filter: v1alpha1.Filter{
					Object: v1alpha1.ObjectFilter{
						Custom: &v1alpha1.ObjectFilterCustom{
							Template: "{{ .data.key }}",
							Result:   "non-related-value",
						},
					},
				},
			},
		}).Compile()
		controller = NewController(mockClient, &http.Client{Transport: mockRoundTripper}, watcher)
	)

	// when
	secret := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{"namespace": "my-secret"},
			"data":     map[string]interface{}{"key": "value"},
		},
	}
	result, reconcileErr := controller.Reconcile(ctx, secret)

	// then
	mockRoundTripper.AssertNotCalled(t, "RoundTrip")
	assert.Nil(t, reconcileErr)
	assert.False(t, result.Requeue)
}

func TestController_FilterEvent(t *testing.T) {
	// given
	var (
		mockClient = new(client2.MockClient)
		oldSecret  = &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				CreationTimestamp: metav1.Time{Time: time.Now()},
				Generation:        int64(1),
			},
		}
		newSecret = &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				CreationTimestamp: metav1.Time{Time: time.Now()},
				Generation:        int64(2),
			},
		}
		watcher = (&v1alpha1.Watcher{
			Spec: v1alpha1.WatcherSpec{
				Filter: v1alpha1.Filter{
					Event: v1alpha1.EventFilter{
						Create: v1alpha1.CreateEventFilter{
							CreationTimeout: ptr.To("1h"),
						},
						Update: v1alpha1.UpdateEventFilter{
							GenerationChanged: ptr.To(true),
						},
					},
				},
			},
		}).Compile()
		controller = NewController(mockClient, &http.Client{}, watcher).FilterEvent()
	)

	// when
	filtered := controller.Create(event.CreateEvent{
		Object: newSecret,
	}) && controller.Update(event.UpdateEvent{
		ObjectOld: oldSecret,
		ObjectNew: newSecret,
	}) == false

	// then
	assert.False(t, filtered)
}

func TestController_FilterEventCreateCreationTimeout(t *testing.T) {
	// given
	var (
		mockClient = new(client2.MockClient)
		secret     = &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				CreationTimestamp: metav1.Time{Time: time.Now().Add(-1 * time.Hour)},
			},
		}
		watcher = (&v1alpha1.Watcher{
			Spec: v1alpha1.WatcherSpec{
				Filter: v1alpha1.Filter{
					Event: v1alpha1.EventFilter{
						Create: v1alpha1.CreateEventFilter{
							CreationTimeout: ptr.To("10s"),
						},
					},
				},
			},
		}).Compile()
		controller = NewController(mockClient, &http.Client{}, watcher).FilterEvent()
	)

	// when
	filtered := controller.Create(event.CreateEvent{
		Object: secret,
	}) == false

	// then
	assert.True(t, filtered)
}

func TestController_FilterEventUpdateGenerationChangedTrue(t *testing.T) {
	// given
	var (
		mockClient = new(client2.MockClient)
		oldSecret  = &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Generation: int64(2),
			},
		}
		newSecret = &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Generation: int64(2),
			},
		}
		watcher = (&v1alpha1.Watcher{
			Spec: v1alpha1.WatcherSpec{
				Filter: v1alpha1.Filter{
					Event: v1alpha1.EventFilter{
						Update: v1alpha1.UpdateEventFilter{
							GenerationChanged: ptr.To(true),
						},
					},
				},
			},
		}).Compile()
		controller = NewController(mockClient, &http.Client{}, watcher).FilterEvent()
	)

	// when
	filtered := controller.Update(event.UpdateEvent{
		ObjectOld: oldSecret,
		ObjectNew: newSecret,
	}) == false

	// then
	assert.True(t, filtered)
}

func TestController_FilterEventUpdateGenerationChangedFalse(t *testing.T) {
	// given
	var (
		mockClient = new(client2.MockClient)
		oldSecret  = &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Generation: int64(1),
			},
		}
		newSecret = &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Generation: int64(1),
			},
		}
		watcher = (&v1alpha1.Watcher{
			Spec: v1alpha1.WatcherSpec{
				Filter: v1alpha1.Filter{
					Event: v1alpha1.EventFilter{
						Update: v1alpha1.UpdateEventFilter{
							GenerationChanged: ptr.To(false),
						},
					},
				},
			},
		}).Compile()
		controller = NewController(mockClient, &http.Client{}, watcher).FilterEvent()
	)

	// when
	filtered := controller.Update(event.UpdateEvent{
		ObjectOld: oldSecret,
		ObjectNew: newSecret,
	}) == false

	// then
	assert.False(t, filtered)
}

func TestController_SetupWithManager(t *testing.T) {
	// given
	var (
		mockClient       = new(client2.MockClient)
		mockManager      = new(manager.MockManager)
		mockCache        = new(cache2.MockCache)
		mockRoundTripper = new(http2.MockRoundTripper)
		watcher          = (&v1alpha1.Watcher{
			Spec: v1alpha1.WatcherSpec{
				Source: v1alpha1.Source{
					APIVersion:  "v1",
					Kind:        "Secret",
					Concurrency: ptr.To(2),
				},
			},
		}).Compile()
		controller = NewController(mockClient, &http.Client{Transport: mockRoundTripper}, watcher)
	)

	mockManager.EXPECT().GetControllerOptions().Return(config.Controller{})
	mockManager.EXPECT().GetScheme().Return(testVars.scheme)
	mockManager.EXPECT().GetCache().Return(mockCache)
	mockManager.EXPECT().GetRESTMapper().Return(meta.MultiRESTMapper{})
	mockManager.EXPECT().GetLogger().Return(zap.New())
	mockManager.EXPECT().GetFieldIndexer().Return(mockCache)
	mockManager.EXPECT().Add(mock.MatchedBy(func(ct controller2.Controller) bool {
		return ct != nil
	})).Return(nil)

	// when
	setupErr := controller.SetupWithManager(mockManager)

	// then
	assert.Nil(t, setupErr)
}
