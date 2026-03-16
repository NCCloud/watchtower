package main

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"dario.cat/mergo"
	"github.com/mitchellh/hashstructure/v2"
	"github.com/nccloud/watchtower/pkg"
	"github.com/nccloud/watchtower/pkg/apis/v1alpha1"
	"github.com/nccloud/watchtower/pkg/common"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var (
	metricPort = 8083
	healthPort = 8084
	scheme     = runtime.NewScheme()
)

func main() {
	logger := zap.New()
	ctrl.SetLogger(logger)
	config := common.NewConfig()
	ctx := ctrl.SetupSignalHandler()

	common.Must(clientgoscheme.AddToScheme(scheme))
	common.Must(v1alpha1.AddToScheme(scheme))

	mgr := common.MustReturn(ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Logger: logger,
		Cache: cache.Options{
			SyncPeriod: &config.SyncPeriod,
		},
		Metrics: server.Options{
			BindAddress: fmt.Sprintf(":%d", metricPort),
		},
		HealthProbeBindAddress:        fmt.Sprintf(":%d", healthPort),
		LeaderElection:                config.EnableLeaderElection,
		LeaderElectionID:              "watchtower.cloud.spaceship.com",
		LeaderElectionReleaseOnCancel: true,
	}))

	common.Must((&WatcherReconciler{
		parentCtx:     ctx,
		manager:       mgr,
		refreshPeriod: config.WatcherRefreshPeriod,
		active:        make(map[string]*activeWatcher),
	}).SetupWithManager(mgr))

	common.Must(mgr.AddHealthzCheck("healthz", healthz.Ping))
	common.Must(mgr.AddReadyzCheck("readyz", healthz.Ping))
	common.Must(mgr.Start(ctx))
}

type activeWatcher struct {
	cancel context.CancelFunc
	hash   uint64
}

type WatcherReconciler struct {
	parentCtx     context.Context
	manager       ctrl.Manager
	refreshPeriod time.Duration
	mu            sync.Mutex
	active        map[string]*activeWatcher
}

func (r *WatcherReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Watcher{}).
		Complete(r)
}

func (r *WatcherReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var watcher v1alpha1.Watcher
	if err := r.manager.GetClient().Get(ctx, req.NamespacedName, &watcher); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}

		r.stopController(req.Name)
		logger.Info("Watcher removed")

		return ctrl.Result{}, nil
	}

	resolved, resolveErr := r.resolveWatcher(ctx, &watcher)
	if resolveErr != nil {
		return ctrl.Result{}, resolveErr
	}

	hash := common.MustReturn(hashstructure.Hash(resolved.Spec, hashstructure.FormatV2, nil))

	changed, startErr := r.ensureController(resolved.Compile(), hash)
	if startErr != nil {
		return ctrl.Result{}, startErr
	}

	if changed {
		logger.Info("Watcher configured",
			"source", fmt.Sprintf("%s/%s", resolved.Spec.Source.APIVersion, resolved.Spec.Source.Kind))
	}

	return ctrl.Result{RequeueAfter: r.refreshPeriod}, nil
}

func (r *WatcherReconciler) resolveWatcher(ctx context.Context, watcher *v1alpha1.Watcher) (*v1alpha1.Watcher, error) {
	kubeClient := r.manager.GetClient()

	for _, sel := range watcher.Spec.ValuesFrom.Secrets {
		var (
			secret v1.Secret
			spec   v1alpha1.WatcherSpec
		)

		if err := kubeClient.Get(ctx, types.NamespacedName{
			Name: sel.Name, Namespace: sel.Namespace,
		}, &secret); err != nil {
			return nil, err
		}

		if err := yaml.Unmarshal(secret.Data[sel.Key], &spec); err != nil {
			return nil, err
		}

		if err := mergo.Merge(watcher, v1alpha1.Watcher{Spec: spec},
			mergo.WithOverride, mergo.WithAppendSlice); err != nil {
			return nil, err
		}
	}

	return watcher, nil
}

func (r *WatcherReconciler) ensureController(watcher *v1alpha1.Watcher, hash uint64) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.active[watcher.Name]; ok && existing.hash == hash {
		return false, nil
	}

	if existing, ok := r.active[watcher.Name]; ok {
		existing.cancel()
		delete(r.active, watcher.Name)
	}

	watcherCtrl := pkg.NewController(r.manager.GetClient(), &http.Client{}, watcher)

	c, err := controller.NewUnmanaged(watcher.Name, controller.Options{
		Reconciler:              watcherCtrl,
		MaxConcurrentReconciles: watcher.Spec.GetConcurrency(),
	})
	if err != nil {
		return false, err
	}

	if watchErr := c.Watch(source.Kind[client.Object](
		r.manager.GetCache(),
		watcher.Spec.Source.NewObject(),
		&handler.EnqueueRequestForObject{},
		watcherCtrl.FilterEvent(),
	)); watchErr != nil {
		return false, watchErr
	}

	childCtx, cancel := context.WithCancel(r.parentCtx)
	r.active[watcher.Name] = &activeWatcher{cancel: cancel, hash: hash}

	go func() {
		if startErr := c.Start(childCtx); startErr != nil {
			ctrl.Log.Error(startErr, "Controller failed", "watcher", watcher.Name)
		}
	}()

	return true, nil
}

func (r *WatcherReconciler) stopController(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.active[name]; ok {
		existing.cancel()
		delete(r.active, name)
	}
}
