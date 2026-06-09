package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"dario.cat/mergo"
	"github.com/go-logr/logr"
	"github.com/mitchellh/hashstructure/v2"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/nccloud/watchtower/pkg"
	"github.com/nccloud/watchtower/pkg/apis/v1alpha1"
	"github.com/nccloud/watchtower/pkg/common"
)

const (
	metricsPort      = 8083
	healthPort       = 8084
	leaderElectionID = "watchtower.cloud.spaceship.com"
)

type app struct {
	cfg        *common.Config
	scheme     *runtime.Scheme
	restConfig *rest.Config
	kubeClient client.Client
	logger     logr.Logger
}

func main() {
	logger := zap.New()
	ctrl.SetLogger(logger)

	scheme := runtime.NewScheme()
	common.Must(clientgoscheme.AddToScheme(scheme))
	common.Must(v1alpha1.AddToScheme(scheme))

	restConfig := ctrl.GetConfigOrDie()
	kubeClient := common.MustReturn(client.New(restConfig, client.Options{Scheme: scheme}))

	a := &app{
		cfg:        common.NewConfig(),
		scheme:     scheme,
		restConfig: restConfig,
		kubeClient: kubeClient,
		logger:     logger,
	}

	a.run(ctrl.SetupSignalHandler())
}

func (a *app) run(ctx context.Context) {
	for ctx.Err() == nil {
		watchers, loadErr := a.loadWatchers(ctx)
		if loadErr != nil {
			a.logger.Error(loadErr, "Failed to load watchers; retrying")

			timer := time.NewTimer(a.cfg.WatcherRefreshPeriod)
			select {
			case <-ctx.Done():
			case <-timer.C:
			}
			timer.Stop()

			continue
		}

		runCtx, cancel := context.WithCancel(ctx)
		go a.watchForChanges(runCtx, watchers, cancel)

		if runErr := a.runManager(runCtx, watchers); runErr != nil && ctx.Err() == nil {
			a.logger.Error(runErr, "Manager exited with error; restarting")
		}

		cancel()
	}
}

func (a *app) loadWatchers(ctx context.Context) ([]v1alpha1.Watcher, error) {
	var list v1alpha1.WatcherList
	if listErr := a.kubeClient.List(ctx, &list); listErr != nil {
		return nil, listErr
	}

	out := make([]v1alpha1.Watcher, 0, len(list.Items))

	for _, watcher := range list.Items {
		merged, mergeErr := a.mergeSecretValues(ctx, watcher)
		if mergeErr != nil {
			return nil, mergeErr
		}

		out = append(out, merged)
	}

	return out, nil
}

func (a *app) mergeSecretValues(ctx context.Context, w v1alpha1.Watcher) (v1alpha1.Watcher, error) {
	for _, sel := range w.Spec.ValuesFrom.Secrets {
		var secret v1.Secret
		if getErr := a.kubeClient.Get(ctx, types.NamespacedName{
			Name:      sel.Name,
			Namespace: sel.Namespace,
		}, &secret); getErr != nil {
			return w, getErr
		}

		var spec v1alpha1.WatcherSpec
		if unmarshalErr := yaml.Unmarshal(secret.Data[sel.Key], &spec); unmarshalErr != nil {
			return w, unmarshalErr
		}

		if mergeErr := mergo.Merge(&w, v1alpha1.Watcher{Spec: spec},
			mergo.WithOverride, mergo.WithAppendSlice); mergeErr != nil {
			return w, mergeErr
		}
	}

	return w, nil
}

func (a *app) watchForChanges(ctx context.Context, baseline []v1alpha1.Watcher, onChange context.CancelFunc) {
	baselineHash, hashErr := hashstructure.Hash(baseline, hashstructure.FormatV2, nil)
	if hashErr != nil {
		a.logger.Error(hashErr, "Failed to hash baseline watchers; change detection disabled")

		return
	}

	ticker := time.NewTicker(a.cfg.WatcherRefreshPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			current, loadErr := a.loadWatchers(ctx)
			if loadErr != nil {
				a.logger.Error(loadErr, "Failed to refresh watchers; will retry next tick")

				continue
			}

			currentHash, hashErr := hashstructure.Hash(current, hashstructure.FormatV2, nil)
			if hashErr != nil {
				a.logger.Error(hashErr, "Failed to hash current watchers; skipping")

				continue
			}

			if currentHash != baselineHash {
				a.logger.Info("Watchers changed; restarting manager")
				onChange()

				return
			}
		}
	}
}

func (a *app) runManager(ctx context.Context, watchers []v1alpha1.Watcher) error {
	mgr, mgrErr := ctrl.NewManager(a.restConfig, ctrl.Options{
		Scheme: a.scheme,
		Logger: a.logger,
		Cache: cache.Options{
			SyncPeriod: &a.cfg.SyncPeriod,
		},
		Metrics: server.Options{
			BindAddress: fmt.Sprintf(":%d", metricsPort),
		},
		HealthProbeBindAddress:        fmt.Sprintf(":%d", healthPort),
		LeaderElection:                a.cfg.EnableLeaderElection,
		LeaderElectionID:              leaderElectionID,
		LeaderElectionReleaseOnCancel: true,
	})
	if mgrErr != nil {
		return fmt.Errorf("new manager: %w", mgrErr)
	}

	for _, watcher := range watchers {
		compiled, compileErr := watcher.Compile()
		if compileErr != nil {
			a.logger.Error(compileErr, "Skipping watcher with invalid config", "name", watcher.Name)

			continue
		}

		if setupErr := pkg.NewController(mgr.GetClient(), &http.Client{}, compiled).
			SetupWithManager(mgr); setupErr != nil {
			a.logger.Error(setupErr, "Skipping watcher; controller setup failed", "name", watcher.Name)
		}
	}

	if healthErr := mgr.AddHealthzCheck("healthz", healthz.Ping); healthErr != nil {
		return fmt.Errorf("add healthz: %w", healthErr)
	}

	if readyErr := mgr.AddReadyzCheck("readyz", healthz.Ping); readyErr != nil {
		return fmt.Errorf("add readyz: %w", readyErr)
	}

	return mgr.Start(ctx)
}
