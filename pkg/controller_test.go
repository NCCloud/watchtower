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
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

	http2 "github.com/nccloud/watchtower/mocks/net/http"
	cache2 "github.com/nccloud/watchtower/mocks/sigs.k8s.io/controller-runtime/pkg/cache"
	client2 "github.com/nccloud/watchtower/mocks/sigs.k8s.io/controller-runtime/pkg/client"
	"github.com/nccloud/watchtower/mocks/sigs.k8s.io/controller-runtime/pkg/manager"
	"github.com/nccloud/watchtower/pkg/apis/v1alpha1"
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
		watcher          = (&v1alpha1.Watcher{}).MustCompile()
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
				Destination: v1alpha1.Destination{
					URLTemplate:  "www.test.com/{{ index .data \"my-key\" }}-in-url",
					BodyTemplate: "{{ index .data \"my-key\" }}-in-template",
					Method:       "POST",
					Headers: map[string]string{
						"key":  "{{ index .data \"my-key\" }}",
						"key2": "{{ index .data \"my-key2\" }}",
					},
				},
			},
		}).MustCompile()
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
					"my-key":  "my-value",
					"my-key2": "my-value2",
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
		headerMatched := reflect.DeepEqual(r.Header["Key"], []string{"my-value"}) &&
			reflect.DeepEqual(r.Header["Key2"], []string{"my-value2"}) &&
			len(r.Header) == 2
		methodMatched := reflect.DeepEqual(r.Method, "POST")
		body, _ := io.ReadAll(r.Body)
		bodyMatched := string(body) == "my-value-in-template"
		return headerMatched && methodMatched && bodyMatched && urlMatched
	}))
}

func TestController_Reconcile_DefaultsMethodAndTimeout(t *testing.T) {
	// given
	var (
		ctx     = context.Background()
		watcher = (&v1alpha1.Watcher{
			Spec: v1alpha1.WatcherSpec{
				Destination: v1alpha1.Destination{
					URLTemplate:  "www.test.com/x",
					BodyTemplate: "body",
					// Method and Timeout intentionally omitted
				},
			},
		}).MustCompile()
	)

	// then defaults are applied
	assert.Equal(t, v1alpha1.DefaultDestinationMethod, watcher.Spec.Destination.Compiled.Method)
	assert.Equal(t, v1alpha1.DefaultDestinationTimeout, watcher.Spec.Destination.Compiled.Timeout)
	_ = ctx
}

func TestController_Reconcile_CustomTimeoutParsed(t *testing.T) {
	// given
	watcher := (&v1alpha1.Watcher{
		Spec: v1alpha1.WatcherSpec{
			Destination: v1alpha1.Destination{
				URLTemplate: "www.test.com/x", BodyTemplate: "b",
				Timeout: ptr.To("5s"),
			},
		},
	}).MustCompile()

	// then
	assert.Equal(t, 5*time.Second, watcher.Spec.Destination.Compiled.Timeout)
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

	mgr, mgrErr := ctrl.NewManager(testVars.kubeConfig, ctrl.Options{
		Scheme: testVars.scheme, Logger: zap.New(),
	})
	if mgrErr != nil {
		panic(mgrErr)
	}

	watcher := (&v1alpha1.Watcher{
		ObjectMeta: metav1.ObjectMeta{
			Name: uuid.NewString(),
		},
		Spec: v1alpha1.WatcherSpec{
			Source: v1alpha1.Source{
				APIVersion:  "v1",
				Kind:        "Secret",
				Concurrency: ptr.To(1),
			},
			Filter: v1alpha1.Filter{
				Create: `object.metadata.name.matches('.*my.*')
					&& object.metadata.namespace.matches('.*efaul.*')
					&& object.metadata.labels['my-label'] == 'true'
					&& object.metadata.annotations['my-annotation'] == 'true'`,
			},
			Destination: v1alpha1.Destination{
				URLTemplate:  fmt.Sprintf("http://%s/{{ .data.id | b64dec }}", server.Listener.Addr().String()),
				BodyTemplate: "{{ .data.value }}",
				Method:       "POST",
				Headers: map[string]string{
					"Authorization": "{{ .data.authorization | b64dec }}",
				},
			},
		},
	}).MustCompile()

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
			"id":            []byte(gofakeit.UUID()),
			"value":         []byte(gofakeit.UUID()),
			"authorization": []byte(gofakeit.UUID()),
		},
	}

	if setupErr := (&Controller{
		client:     mgr.GetClient(),
		watcher:    watcher,
		httpClient: server.Client(),
	}).SetupWithManager(mgr); setupErr != nil {
		panic(setupErr)
	}

	go func() {
		if managerStartErr := mgr.Start(ctx); managerStartErr != nil {
			panic(managerStartErr)
		}
	}()

	// when
	createErr := mgr.GetClient().Create(ctx, secret)

	// then
	assert.Nil(t, createErr)
	assert.Eventually(t, func() bool {
		if request != nil {
			methodMatches := request.Method == watcher.Spec.Destination.Method
			headerMatches := request.Header.Get("Authorization") == string(secret.Data["authorization"])
			urlMatches := fmt.Sprintf("http://%s%s", request.Host, request.URL.Path) ==
				strings.ReplaceAll(watcher.Spec.Destination.URLTemplate, "{{ .data.id | b64dec }}", string(secret.Data["id"]))
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

	mgr, mgrErr := ctrl.NewManager(testVars.kubeConfig, ctrl.Options{
		Scheme: testVars.scheme, Logger: zap.New(),
	})
	if mgrErr != nil {
		panic(mgrErr)
	}

	watcher := (&v1alpha1.Watcher{
		ObjectMeta: metav1.ObjectMeta{
			Name: uuid.NewString(),
		},
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
	}).MustCompile()

	if setupErr := (&Controller{
		client:     mgr.GetClient(),
		watcher:    watcher,
		httpClient: server.Client(),
	}).SetupWithManager(mgr); setupErr != nil {
		panic(setupErr)
	}

	go func() {
		if managerStartErr := mgr.Start(ctx); managerStartErr != nil {
			panic(managerStartErr)
		}
	}()

	// when
	for i := 0; i < testCount; i++ {
		assert.Nil(t, mgr.GetClient().Create(ctx, &v1.Secret{
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

func TestController_Reconcile_DeleteObjectOnSuccess(t *testing.T) {
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
					Headers: map[string]string{
						"key": "{{ index .data \"my-key\" }}",
					},
				},
			},
		}).MustCompile()
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
		headerMatched := reflect.DeepEqual(r.Header["Key"], []string{"my-value"}) && len(r.Header) == 1
		methodMatched := reflect.DeepEqual(r.Method, "POST")
		body, _ := io.ReadAll(r.Body)
		bodyMatched := string(body) == "my-value-in-template"
		return headerMatched && methodMatched && bodyMatched && urlMatched
	}))

	mockClient.AssertCalled(t, "Delete", mock.Anything, secret,
		mock.MatchedBy(func(opts []client.DeleteOption) bool {
			return opts[0] == client.PropagationPolicy(metav1.DeletePropagationBackground)
		}))
}

func TestController_FilterEvent_EmptyExpressionsPassEverything(t *testing.T) {
	// given
	watcher := (&v1alpha1.Watcher{}).MustCompile()
	pred := NewController(new(client2.MockClient), &http.Client{}, watcher).FilterEvent()

	obj := unstructuredObj(map[string]any{"metadata": map[string]any{"name": "x"}})

	// then
	assert.True(t, pred.Create(event.CreateEvent{Object: obj}))
	assert.True(t, pred.Update(event.UpdateEvent{ObjectOld: obj, ObjectNew: obj}))
}

func TestController_FilterEvent_CreateExpression(t *testing.T) {
	// given: only accept Create events for objects in "default" namespace
	watcher := (&v1alpha1.Watcher{
		Spec: v1alpha1.WatcherSpec{
			Filter: v1alpha1.Filter{
				Create: `object.metadata.namespace == 'default'`,
			},
		},
	}).MustCompile()
	pred := NewController(new(client2.MockClient), &http.Client{}, watcher).FilterEvent()

	// when
	matched := pred.Create(event.CreateEvent{Object: unstructuredObj(map[string]any{
		"metadata": map[string]any{"namespace": "default"},
	})})
	skipped := pred.Create(event.CreateEvent{Object: unstructuredObj(map[string]any{
		"metadata": map[string]any{"namespace": "kube-system"},
	})})

	// then
	assert.True(t, matched)
	assert.False(t, skipped)
}

func TestController_FilterEvent_CreateRestartSafety(t *testing.T) {
	// given: only accept Create events for objects created within the last hour (restart safety)
	watcher := (&v1alpha1.Watcher{
		Spec: v1alpha1.WatcherSpec{
			Filter: v1alpha1.Filter{
				Create: `now - timestamp(object.metadata.creationTimestamp) < duration('1h')`,
			},
		},
	}).MustCompile()
	pred := NewController(new(client2.MockClient), &http.Client{}, watcher).FilterEvent()

	now := time.Now().UTC().Format(time.RFC3339)
	old := time.Now().Add(-8 * time.Hour).UTC().Format(time.RFC3339)

	// when
	fresh := pred.Create(event.CreateEvent{Object: unstructuredObj(map[string]any{
		"metadata": map[string]any{"creationTimestamp": now},
	})})
	stale := pred.Create(event.CreateEvent{Object: unstructuredObj(map[string]any{
		"metadata": map[string]any{"creationTimestamp": old},
	})})

	// then
	assert.True(t, fresh)
	assert.False(t, stale)
}

func TestController_FilterEvent_UpdateComparesOldAndNew(t *testing.T) {
	// given: fire only when status.phase changes
	watcher := (&v1alpha1.Watcher{
		Spec: v1alpha1.WatcherSpec{
			Filter: v1alpha1.Filter{
				Update: `object.status.phase != oldObject.status.phase`,
			},
		},
	}).MustCompile()
	pred := NewController(new(client2.MockClient), &http.Client{}, watcher).FilterEvent()

	phasePending := unstructuredObj(map[string]any{"status": map[string]any{"phase": "Pending"}})
	phaseRunning := unstructuredObj(map[string]any{"status": map[string]any{"phase": "Running"}})

	// when
	changed := pred.Update(event.UpdateEvent{ObjectOld: phasePending, ObjectNew: phaseRunning})
	unchanged := pred.Update(event.UpdateEvent{ObjectOld: phaseRunning, ObjectNew: phaseRunning})

	// then
	assert.True(t, changed)
	assert.False(t, unchanged)
}

func TestController_FilterEvent_UpdateScaleUpOnly(t *testing.T) {
	// given: fire only on replica scale-ups (not scale-downs)
	watcher := (&v1alpha1.Watcher{
		Spec: v1alpha1.WatcherSpec{
			Filter: v1alpha1.Filter{
				Update: `int(object.spec.replicas) > int(oldObject.spec.replicas)`,
			},
		},
	}).MustCompile()
	pred := NewController(new(client2.MockClient), &http.Client{}, watcher).FilterEvent()

	mk := func(r int64) *unstructured.Unstructured {
		return unstructuredObj(map[string]any{"spec": map[string]any{"replicas": r}})
	}

	// when
	up := pred.Update(event.UpdateEvent{ObjectOld: mk(2), ObjectNew: mk(3)})
	down := pred.Update(event.UpdateEvent{ObjectOld: mk(3), ObjectNew: mk(2)})
	flat := pred.Update(event.UpdateEvent{ObjectOld: mk(2), ObjectNew: mk(2)})

	// then
	assert.True(t, up)
	assert.False(t, down)
	assert.False(t, flat)
}

func TestController_FilterEvent_DeepEqualOnList(t *testing.T) {
	// given: replicate the old fields:[".status.conditions"] semantic
	watcher := (&v1alpha1.Watcher{
		Spec: v1alpha1.WatcherSpec{
			Filter: v1alpha1.Filter{
				Update: `object.status.conditions != oldObject.status.conditions`,
			},
		},
	}).MustCompile()
	pred := NewController(new(client2.MockClient), &http.Client{}, watcher).FilterEvent()

	mk := func(types ...string) *unstructured.Unstructured {
		conds := make([]any, 0, len(types))
		for _, ty := range types {
			conds = append(conds, map[string]any{"type": ty})
		}
		return unstructuredObj(map[string]any{"status": map[string]any{"conditions": conds}})
	}

	// when
	changed := pred.Update(event.UpdateEvent{ObjectOld: mk("A"), ObjectNew: mk("A", "B")})
	unchanged := pred.Update(event.UpdateEvent{ObjectOld: mk("A", "B"), ObjectNew: mk("A", "B")})

	// then
	assert.True(t, changed)
	assert.False(t, unchanged)
}

func TestController_FilterEvent_RejectsNonUnstructured(t *testing.T) {
	// given
	watcher := (&v1alpha1.Watcher{
		Spec: v1alpha1.WatcherSpec{
			Filter: v1alpha1.Filter{
				Create: `true`,
				Update: `true`,
			},
		},
	}).MustCompile()
	pred := NewController(new(client2.MockClient), &http.Client{}, watcher).FilterEvent()

	// when typed (non-unstructured) objects flow in, the filter rejects them
	// (CEL bindings expect dynamic maps from unstructured)
	create := pred.Create(event.CreateEvent{
		Object: &v1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "a"}},
	})
	update := pred.Update(event.UpdateEvent{
		ObjectOld: &v1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "a"}},
		ObjectNew: &v1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "b"}},
	})

	// then
	assert.False(t, create)
	assert.False(t, update)
}

func TestController_FilterEvent_NonBoolReturnsFalse(t *testing.T) {
	// given: expression returns a non-bool string -> treat as filtered out
	watcher := (&v1alpha1.Watcher{
		Spec: v1alpha1.WatcherSpec{
			Filter: v1alpha1.Filter{
				Create: `"hello"`,
			},
		},
	}).MustCompile()
	pred := NewController(new(client2.MockClient), &http.Client{}, watcher).FilterEvent()

	obj := unstructuredObj(map[string]any{"metadata": map[string]any{"name": "x"}})

	// then
	assert.False(t, pred.Create(event.CreateEvent{Object: obj}))
}

func TestController_FilterEvent_DeleteDisabledByDefault(t *testing.T) {
	// given: no filter.delete -> Delete events are dropped (conservative default)
	watcher := (&v1alpha1.Watcher{}).MustCompile()
	pred := NewController(new(client2.MockClient), &http.Client{}, watcher).FilterEvent()

	obj := unstructuredObj(map[string]any{"metadata": map[string]any{"name": "x"}})

	// then
	assert.False(t, pred.Delete(event.DeleteEvent{Object: obj}))
}

func TestController_FilterEvent_DeleteEnabledByExpression(t *testing.T) {
	// given: filter.delete = "true" -> every Delete event passes
	watcher := (&v1alpha1.Watcher{
		Spec: v1alpha1.WatcherSpec{
			Filter: v1alpha1.Filter{Delete: "true"},
		},
	}).MustCompile()
	pred := NewController(new(client2.MockClient), &http.Client{}, watcher).FilterEvent()

	obj := unstructuredObj(map[string]any{"metadata": map[string]any{
		"name":      "x",
		"namespace": "default",
	}})

	// then
	assert.True(t, pred.Delete(event.DeleteEvent{Object: obj}))
}

func TestController_FilterEvent_DeleteCachesObjectForReconcile(t *testing.T) {
	// given
	watcher := (&v1alpha1.Watcher{
		Spec: v1alpha1.WatcherSpec{
			Filter: v1alpha1.Filter{Delete: `object.metadata.namespace == 'default'`},
		},
	}).MustCompile()
	c := NewController(new(client2.MockClient), &http.Client{}, watcher)
	pred := c.FilterEvent()

	obj := unstructuredObj(map[string]any{"metadata": map[string]any{
		"name":      "my-secret",
		"namespace": "default",
	}})

	// when: predicate accepts the delete and caches the object
	passed := pred.Delete(event.DeleteEvent{Object: obj})

	// then
	assert.True(t, passed)
	cached, ok := c.deleted.Load(types.NamespacedName{Name: "my-secret", Namespace: "default"})
	assert.True(t, ok)
	assert.Equal(t, "my-secret", cached.(*unstructured.Unstructured).GetName())
}

func TestController_Reconcile_FiresFromDeleteCacheWhenGetReturnsNotFound(t *testing.T) {
	// given: object is gone from the API server but cached as a tracked delete
	var (
		ctx     = context.Background()
		watcher = (&v1alpha1.Watcher{
			Spec: v1alpha1.WatcherSpec{
				Destination: v1alpha1.Destination{
					URLTemplate:  "www.test.com/{{ .metadata.name }}",
					BodyTemplate: "{{ .metadata.name }}-deleted",
					Method:       "POST",
				},
			},
		}).MustCompile()
		mockClient       = new(client2.MockClient)
		mockRoundTripper = new(http2.MockRoundTripper)
		deleted          = &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Secret",
				"metadata": map[string]interface{}{
					"name":      "my-secret",
					"namespace": "default",
				},
			},
		}
		c   = NewController(mockClient, &http.Client{Transport: mockRoundTripper}, watcher)
		key = client.ObjectKeyFromObject(deleted)
	)

	c.deleted.Store(key, deleted)
	mockClient.EXPECT().Get(mock.Anything, key, mock.Anything).Return(
		apierrors.NewNotFound(schema.GroupResource{Resource: "secrets"}, "my-secret"))
	mockRoundTripper.EXPECT().RoundTrip(mock.Anything).Return(&http.Response{StatusCode: 200}, nil)

	// when
	_, reconcileErr := c.Reconcile(ctx, ctrl.Request{NamespacedName: key})

	// then: send fired with cached object, cache evicted
	assert.Nil(t, reconcileErr)
	mockRoundTripper.AssertCalled(t, "RoundTrip", mock.MatchedBy(func(r *http.Request) bool {
		body, _ := io.ReadAll(r.Body)
		return r.URL.String() == "www.test.com/my-secret" && string(body) == "my-secret-deleted"
	}))
	_, stillCached := c.deleted.Load(key)
	assert.False(t, stillCached)
}

func TestController_Reconcile_LiveObjectClearsStaleDeleteCacheEntry(t *testing.T) {
	// given: a recreate scenario — same name, but a live object now exists
	var (
		ctx     = context.Background()
		watcher = (&v1alpha1.Watcher{
			Spec: v1alpha1.WatcherSpec{
				Destination: v1alpha1.Destination{
					URLTemplate:  "www.test.com/{{ .metadata.name }}",
					BodyTemplate: "{{ .metadata.name }}",
					Method:       "POST",
				},
			},
		}).MustCompile()
		mockClient       = new(client2.MockClient)
		mockRoundTripper = new(http2.MockRoundTripper)
		live             = &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Secret",
				"metadata": map[string]interface{}{
					"name":      "my-secret",
					"namespace": "default",
				},
			},
		}
		stale = &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Secret",
				"metadata": map[string]interface{}{
					"name":      "my-secret",
					"namespace": "default",
				},
			},
		}
		c   = NewController(mockClient, &http.Client{Transport: mockRoundTripper}, watcher)
		key = client.ObjectKeyFromObject(live)
	)

	c.deleted.Store(key, stale)
	mockClient.EXPECT().Get(mock.Anything, key, mock.Anything).RunAndReturn(
		func(ctx context.Context, _ types.NamespacedName, obj client.Object, _ ...client.GetOption) error {
			live.DeepCopyInto(obj.(*unstructured.Unstructured))
			return nil
		})
	mockRoundTripper.EXPECT().RoundTrip(mock.Anything).Return(&http.Response{StatusCode: 200}, nil)

	// when
	_, reconcileErr := c.Reconcile(ctx, ctrl.Request{NamespacedName: key})

	// then: send fired against live object; stale entry pruned
	assert.Nil(t, reconcileErr)
	_, stillCached := c.deleted.Load(key)
	assert.False(t, stillCached)
}

func TestController_FilterEvent_RuntimeErrorReturnsFalse(t *testing.T) {
	// given: expression references a field that doesn't exist on the object -> runtime error
	watcher := (&v1alpha1.Watcher{
		Spec: v1alpha1.WatcherSpec{
			Filter: v1alpha1.Filter{
				Create: `object.spec.foo == 'bar'`,
			},
		},
	}).MustCompile()
	pred := NewController(new(client2.MockClient), &http.Client{}, watcher).FilterEvent()

	// when: object has no spec, expression blows up at eval time -> false
	obj := unstructuredObj(map[string]any{"metadata": map[string]any{"name": "x"}})

	// then
	assert.False(t, pred.Create(event.CreateEvent{Object: obj}))
}

func TestController_Compile_ReturnsErrorOnInvalidExpression(t *testing.T) {
	_, err := (&v1alpha1.Watcher{
		Spec: v1alpha1.WatcherSpec{
			Filter: v1alpha1.Filter{Create: "this is not (valid CEL"},
		},
	}).Compile()
	assert.Error(t, err)
}

func TestController_Compile_ReturnsErrorOnInvalidTimeout(t *testing.T) {
	_, err := (&v1alpha1.Watcher{
		Spec: v1alpha1.WatcherSpec{
			Destination: v1alpha1.Destination{Timeout: ptr.To("not-a-duration")},
		},
	}).Compile()
	assert.Error(t, err)
}

func TestController_MustCompile_PanicsOnInvalidExpression(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on invalid CEL expression")
		}
	}()
	(&v1alpha1.Watcher{
		Spec: v1alpha1.WatcherSpec{
			Filter: v1alpha1.Filter{Create: "this is not (valid CEL"},
		},
	}).MustCompile()
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
		}).MustCompile()
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

func unstructuredObj(content map[string]any) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: content}
}
