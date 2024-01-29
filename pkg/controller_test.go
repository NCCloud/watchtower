package pkg

import (
	"context"
	"github.com/brianvoe/gofakeit/v6"
	"github.com/go-logr/logr"
	"github.com/nccloud/watchtower/mocks"
	http2 "github.com/nccloud/watchtower/mocks/net/http"
	client2 "github.com/nccloud/watchtower/mocks/sigs.k8s.io/controller-runtime/pkg/client"
	"github.com/nccloud/watchtower/pkg/apis/v1alpha1"
	"github.com/nccloud/watchtower/pkg/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"net/http"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"testing"
)

var testVars = struct {
	scheme    *runtime.Scheme
	config    *common.Config
	k8sClient client.Client
	logger    logr.Logger
	watcher   *v1alpha1.Watcher
}{
	scheme:  runtime.NewScheme(),
	config:  common.NewConfig(),
	logger:  zap.New(),
	watcher: mocks.NewWatcher(),
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
		watcher          = mocks.NewWatcher()
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
		ctx              = context.Background()
		watcher          = mocks.NewWatcher()
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
			"metadata": map[string]interface{}{
				"name":      "my-secret",
				"namespace": "default",
			},
			"type": "Opaque",
			"data": map[string]interface{}{
				"data": []byte(gofakeit.AppName()),
			},
		},
	}
	result, reconcileErr := controller.Reconcile(ctx, secret)

	// then
	assert.Nil(t, reconcileErr)
	assert.False(t, result.Requeue)
}
