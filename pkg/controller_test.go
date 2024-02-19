package pkg

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/brianvoe/gofakeit/v6"
	"github.com/go-logr/logr"
	http2 "github.com/nccloud/watchtower/mocks/net/http"
	cache2 "github.com/nccloud/watchtower/mocks/sigs.k8s.io/controller-runtime/pkg/cache"
	client2 "github.com/nccloud/watchtower/mocks/sigs.k8s.io/controller-runtime/pkg/client"
	"github.com/nccloud/watchtower/mocks/sigs.k8s.io/controller-runtime/pkg/manager"
	"github.com/nccloud/watchtower/pkg/apis/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/config"
	controller2 "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var testVars = struct {
	kubeConfig *rest.Config
	scheme     *runtime.Scheme
	logger     logr.Logger
}{
	scheme: runtime.NewScheme(),
	logger: zap.New(),
}

func init() {
	ctrl.SetLogger(testVars.logger)
	utilruntime.Must(clientgoscheme.AddToScheme(testVars.scheme))
	utilruntime.Must(v1alpha1.AddToScheme(testVars.scheme))

	kubeConfig, testEnvStartErr := (&envtest.Environment{
		ControlPlane: envtest.ControlPlane{
			APIServer: &envtest.APIServer{
				StartTimeout: 5 * time.Minute,
				StopTimeout:  5 * time.Minute,
			},
			Etcd: &envtest.Etcd{
				StartTimeout: 5 * time.Minute,
				StopTimeout:  5 * time.Minute,
			},
		},
		ErrorIfCRDPathMissing: true,
		CRDDirectoryPaths: []string{
			filepath.Join("..", "deploy", "crds"), filepath.Join("..", ".envtest", "crds"),
		},
		BinaryAssetsDirectory:    "../.envtest/bins",
		ControlPlaneStartTimeout: 5 * time.Minute,
		ControlPlaneStopTimeout:  5 * time.Minute,
	}).Start()
	if testEnvStartErr != nil {
		panic(testEnvStartErr)
	}

	testVars.kubeConfig = kubeConfig
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
		secret           = &unstructured.Unstructured{
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
		controller = NewController(mockClient, &http.Client{Transport: mockRoundTripper}, watcher)
	)
	mockClient.EXPECT().Get(mock.Anything, client.ObjectKeyFromObject(secret),
		mock.AnythingOfType("*unstructured.Unstructured")).RunAndReturn(
		func(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
			secret.DeepCopyInto(obj.(*unstructured.Unstructured))
			return nil
		})
	mockClient.EXPECT().Get(mock.Anything, client.ObjectKeyFromObject(secret), mock.Anything).Return(nil)
	mockRoundTripper.EXPECT().RoundTrip(mock.Anything).Return(&http.Response{StatusCode: 200}, nil)

	// when
	result, reconcileErr := controller.Reconcile(ctx, ctrl.Request{
		NamespacedName: client.ObjectKeyFromObject(secret),
	})

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

func TestController_ReconcileIntegration(t *testing.T) {
	// given
	ctx, cancel := context.WithCancel(context.Background())
	var request *http.Request
	var body []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request = r
		body, _ = io.ReadAll(r.Body)
		w.Write([]byte("OK"))
	}))

	manager, managerErr := ctrl.NewManager(testVars.kubeConfig, ctrl.Options{
		Scheme: testVars.scheme, Logger: zap.New(),
	})
	if managerErr != nil {
		panic(managerErr)
	}

	watcher := (&v1alpha1.Watcher{
		Spec: v1alpha1.WatcherSpec{
			Source: v1alpha1.Source{
				APIVersion:  "v1",
				Kind:        "Secret",
				Concurrency: ptr.To(1),
			},
			Filter: v1alpha1.Filter{
				Object: v1alpha1.ObjectFilter{
					Name:      ptr.To(".*my.*"),
					Namespace: ptr.To(".*efaul.*"),
					Labels: ptr.To(map[string]string{
						"my-label": "true",
					}),
					Annotations: ptr.To(map[string]string{
						"my-annotation": "true",
					}),
					Custom: &v1alpha1.ObjectFilterCustom{
						Template: "{{ index .data \"my-key\" | b64dec }}",
						Result:   "my-value",
					},
				},
			},
			Destination: v1alpha1.Destination{
				URLTemplate:  fmt.Sprintf("http://%s/{{ .data.id | b64dec }}", server.Listener.Addr().String()),
				BodyTemplate: "{{ .data.value }}",
				Method:       "POST",
				Headers:      map[string][]string{"Bearer": {gofakeit.UUID()}},
			},
		},
	}).Compile()

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-secret",
			Namespace: "default",
			Labels: map[string]string{
				"my-label": "true",
			},
			Annotations: map[string]string{
				"my-annotation": "true",
			},
		},
		Data: map[string][]byte{
			"my-key": []byte("my-value"),
			"id":     []byte(gofakeit.UUID()),
			"value":  []byte(gofakeit.UUID()),
		},
	}

	if setupErr := (&Controller{
		client:     manager.GetClient(),
		watcher:    watcher,
		httpClient: server.Client(),
	}).SetupWithManager(manager); setupErr != nil {
		panic(setupErr)
	}

	go func() {
		if managerStartErr := manager.Start(ctx); managerStartErr != nil {
			panic(managerStartErr)
		}
	}()

	// when
	createErr := manager.GetClient().Create(ctx, secret)

	// then
	assert.Nil(t, createErr)
	assert.Eventually(t, func() bool {
		if request != nil {
			methodMatches := request.Method == watcher.Spec.Destination.Method
			headerMatches := request.Header.Get("Bearer") == watcher.Spec.Destination.Headers["Bearer"][0]
			urlMatches := fmt.Sprintf("http://%s%s", request.Host, request.URL.Path) == strings.ReplaceAll(watcher.Spec.Destination.URLTemplate,
				"{{ .data.id | b64dec }}", string(secret.Data["id"]))
			body, _ = io.ReadAll(base64.NewDecoder(base64.StdEncoding, bytes.NewBuffer(body)))
			bodyMatches := string(body) == strings.ReplaceAll(watcher.Spec.Destination.BodyTemplate,
				"{{ .data.value }}", string(secret.Data["value"]))
			return methodMatches && headerMatches && urlMatches && bodyMatches
		}

		return false
	}, 10*time.Second, 100*time.Millisecond)

	cancel()
}

func TestController_ReconcileMultipleIntegration(t *testing.T) {
	// given
	ctx, cancel := context.WithCancel(context.Background())
	testCount := gofakeit.IntRange(5, 30)
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount = callCount + 1
		w.Write([]byte("OK"))
	}))

	manager, managerErr := ctrl.NewManager(testVars.kubeConfig, ctrl.Options{
		Scheme: testVars.scheme, Logger: zap.New(),
	})
	if managerErr != nil {
		panic(managerErr)
	}

	watcher := (&v1alpha1.Watcher{
		Spec: v1alpha1.WatcherSpec{
			Source: v1alpha1.Source{
				APIVersion:  "v1",
				Kind:        "Secret",
				Concurrency: ptr.To(1),
			},
			Destination: v1alpha1.Destination{
				URLTemplate:  fmt.Sprintf("http://%s/test", server.Listener.Addr().String()),
				BodyTemplate: "test",
				Method:       "POST",
			},
		},
	}).Compile()

	if setupErr := (&Controller{
		client:     manager.GetClient(),
		watcher:    watcher,
		httpClient: server.Client(),
	}).SetupWithManager(manager); setupErr != nil {
		panic(setupErr)
	}

	go func() {
		if managerStartErr := manager.Start(ctx); managerStartErr != nil {
			panic(managerStartErr)
		}
	}()

	// when
	for i := 0; i < testCount; i++ {
		assert.Nil(t, manager.GetClient().Create(ctx, &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      strings.ToLower(strings.ReplaceAll(gofakeit.Name(), " ", "")),
				Namespace: "default",
			},
		}))
	}

	// then
	assert.Eventually(t, func() bool {
		return callCount >= testCount
	}, 1000*time.Second, 100*time.Millisecond)

	cancel()
}

func TestController_Reconcile_DeleteObjectOnSuccessOption(t *testing.T) {
	// given
	var (
		ctx     = context.Background()
		watcher = (&v1alpha1.Watcher{
			Spec: v1alpha1.WatcherSpec{
				Source: v1alpha1.Source{
					Options: v1alpha1.SourceOptions{
						OnSuccess: v1alpha1.OnSuccessSourceOptions{
							DeleteObject: true,
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
		secret           = &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Secret",
				"type":       "Opaque",
				"metadata": map[string]interface{}{
					"name":      "my-secret",
					"namespace": "my-namespace",
				},
				"data": map[string]interface{}{
					"my-key": "my-value",
				},
			},
		}
		controller = NewController(mockClient, &http.Client{Transport: mockRoundTripper}, watcher)
	)
	mockClient.EXPECT().Get(mock.Anything, client.ObjectKeyFromObject(secret),
		mock.AnythingOfType("*unstructured.Unstructured")).RunAndReturn(
		func(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
			secret.DeepCopyInto(obj.(*unstructured.Unstructured))
			return nil
		})
	mockClient.EXPECT().Get(mock.Anything, client.ObjectKeyFromObject(secret), mock.Anything).Return(nil)
	mockClient.EXPECT().Delete(mock.Anything, mock.Anything, mock.Anything).Return(nil)
	mockRoundTripper.EXPECT().RoundTrip(mock.Anything).Return(&http.Response{StatusCode: 200}, nil)

	// when
	result, reconcileErr := controller.Reconcile(ctx, ctrl.Request{
		NamespacedName: client.ObjectKeyFromObject(secret),
	})

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
	mockClient.AssertCalled(t, "Delete", mock.Anything, secret, client.PropagationPolicy("Background"))
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
		secret = &unstructured.Unstructured{
			Object: map[string]interface{}{
				"metadata": map[string]interface{}{"name": "my-secret"},
			},
		}
		controller = NewController(mockClient, &http.Client{Transport: mockRoundTripper}, watcher)
	)
	mockClient.EXPECT().Get(mock.Anything, client.ObjectKeyFromObject(secret),
		mock.AnythingOfType("*unstructured.Unstructured")).RunAndReturn(
		func(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
			secret.DeepCopyInto(obj.(*unstructured.Unstructured))
			return nil
		})

	// when
	result, reconcileErr := controller.Reconcile(ctx, ctrl.Request{
		NamespacedName: client.ObjectKeyFromObject(secret),
	})

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
		secret = &unstructured.Unstructured{
			Object: map[string]interface{}{
				"metadata": map[string]interface{}{"namespace": "my-secret"},
			},
		}
		controller = NewController(mockClient, &http.Client{Transport: mockRoundTripper}, watcher)
	)
	mockClient.EXPECT().Get(mock.Anything, client.ObjectKeyFromObject(secret),
		mock.AnythingOfType("*unstructured.Unstructured")).RunAndReturn(
		func(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
			secret.DeepCopyInto(obj.(*unstructured.Unstructured))
			return nil
		})

	// when
	result, reconcileErr := controller.Reconcile(ctx, ctrl.Request{
		NamespacedName: client.ObjectKeyFromObject(secret),
	})

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
		secret = &unstructured.Unstructured{
			Object: map[string]interface{}{},
		}
		controller = NewController(mockClient, &http.Client{Transport: mockRoundTripper}, watcher)
	)
	mockClient.EXPECT().Get(mock.Anything, client.ObjectKeyFromObject(secret),
		mock.AnythingOfType("*unstructured.Unstructured")).RunAndReturn(
		func(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
			secret.DeepCopyInto(obj.(*unstructured.Unstructured))
			return nil
		})

	// when
	result, reconcileErr := controller.Reconcile(ctx, ctrl.Request{
		NamespacedName: client.ObjectKeyFromObject(secret),
	})

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
		secret = &unstructured.Unstructured{
			Object: map[string]interface{}{},
		}
		controller = NewController(mockClient, &http.Client{Transport: mockRoundTripper}, watcher)
	)
	mockClient.EXPECT().Get(mock.Anything, client.ObjectKeyFromObject(secret),
		mock.AnythingOfType("*unstructured.Unstructured")).RunAndReturn(
		func(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
			secret.DeepCopyInto(obj.(*unstructured.Unstructured))
			return nil
		})

	// when
	result, reconcileErr := controller.Reconcile(ctx, ctrl.Request{
		NamespacedName: client.ObjectKeyFromObject(secret),
	})
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
		secret = &unstructured.Unstructured{
			Object: map[string]interface{}{
				"metadata": map[string]interface{}{"namespace": "my-secret"},
				"data":     map[string]interface{}{"key": "value"},
			},
		}
		controller = NewController(mockClient, &http.Client{Transport: mockRoundTripper}, watcher)
	)
	mockClient.EXPECT().Get(mock.Anything, client.ObjectKeyFromObject(secret),
		mock.AnythingOfType("*unstructured.Unstructured")).RunAndReturn(
		func(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
			secret.DeepCopyInto(obj.(*unstructured.Unstructured))
			return nil
		})

	// when
	result, reconcileErr := controller.Reconcile(ctx, ctrl.Request{
		NamespacedName: client.ObjectKeyFromObject(secret),
	})
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
				ResourceVersion:   "1",
			},
		}
		newSecret = &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				CreationTimestamp: metav1.Time{Time: time.Now()},
				Generation:        int64(2),
				ResourceVersion:   "2",
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
							GenerationChanged:      ptr.To(true),
							ResourceVersionChanged: ptr.To(true),
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

func TestController_FilterEvent_CreationTimeouts(t *testing.T) {
	// given
	var (
		mockClient = new(client2.MockClient)
		newSecret  = &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				CreationTimestamp: metav1.Time{Time: time.Now()},
			},
		}
		oldSecret = &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				CreationTimestamp: metav1.Time{Time: time.Now().Add(-8 * time.Hour)},
			},
		}
		watcher = (&v1alpha1.Watcher{
			Spec: v1alpha1.WatcherSpec{
				Filter: v1alpha1.Filter{
					Event: v1alpha1.EventFilter{
						Create: v1alpha1.CreateEventFilter{
							CreationTimeout: ptr.To("1h"),
						},
					},
				},
			},
		}).Compile()
		controller = NewController(mockClient, &http.Client{}, watcher).FilterEvent()
	)

	// when
	oldSecretFiltered := controller.Create(event.CreateEvent{
		Object: oldSecret,
	}) == false
	newSecretFiltered := controller.Create(event.CreateEvent{
		Object: newSecret,
	}) == false

	// then
	assert.True(t, oldSecretFiltered)
	assert.False(t, newSecretFiltered)
}

func TestController_FilterEvent_GenerationChanged(t *testing.T) {
	// given
	var (
		mockClient = new(client2.MockClient)
		gen1Secret = &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Generation: int64(1),
			},
		}
		gen2Secret = &v1.Secret{
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
	differentGenFiltered := controller.Update(event.UpdateEvent{
		ObjectOld: gen1Secret,
		ObjectNew: gen2Secret,
	}) == false
	sameGenFiltered := controller.Update(event.UpdateEvent{
		ObjectOld: gen2Secret,
		ObjectNew: gen2Secret,
	}) == false

	// then
	assert.False(t, differentGenFiltered)
	assert.True(t, sameGenFiltered)
}

func TestController_FilterEvent_ResourceVersionChanged(t *testing.T) {
	// given
	var (
		mockClient = new(client2.MockClient)
		res1Secret = &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				ResourceVersion: "1",
			},
		}
		res2Secret = &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				ResourceVersion: "2",
			},
		}
		watcher = (&v1alpha1.Watcher{
			Spec: v1alpha1.WatcherSpec{
				Filter: v1alpha1.Filter{
					Event: v1alpha1.EventFilter{
						Update: v1alpha1.UpdateEventFilter{
							ResourceVersionChanged: ptr.To(true),
						},
					},
				},
			},
		}).Compile()
		controller = NewController(mockClient, &http.Client{}, watcher).FilterEvent()
	)

	// when
	differentResFiltered := controller.Update(event.UpdateEvent{
		ObjectOld: res1Secret,
		ObjectNew: res2Secret,
	}) == false
	sameResFiltered := controller.Update(event.UpdateEvent{
		ObjectOld: res2Secret,
		ObjectNew: res2Secret,
	}) == false

	// then
	assert.False(t, differentResFiltered)
	assert.True(t, sameResFiltered)
}

func TestController_FilterEvent_CreationTimeout(t *testing.T) {
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

func TestController_FilterEvent_UpdateGenerationChangedTrue(t *testing.T) {
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

func TestController_FilterEvent_UpdateGenerationChangedFalse(t *testing.T) {
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
	mockManager.EXPECT().GetScheme().Return(runtime.NewScheme())
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
