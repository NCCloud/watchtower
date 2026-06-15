package main

import (
	"context"
	"fmt"
	"net/http"

	"dario.cat/mergo"
	"github.com/go-co-op/gocron/v2"
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
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

var (
	metricPort = 8083
	healthPort = 8084
	logger     = zap.New()
	config     = common.NewConfig()
	scheme     = runtime.NewScheme()
)

func main() {
	ctrl.SetLogger(logger)
	common.Must(clientgoscheme.AddToScheme(scheme))
	common.Must(v1alpha1.AddToScheme(scheme))

	interruptCtx := ctrl.SetupSignalHandler()
	kubeClient := common.MustReturn(client.New(ctrl.GetConfigOrDie(), client.Options{Scheme: scheme}))
	watchers := common.MustReturn(RefreshWatchers(context.Background(), kubeClient))

	scheduler := common.MustReturn(gocron.NewScheduler())
	restartCtx, restart := context.WithCancel(interruptCtx)

	common.MustReturn(scheduler.NewJob(gocron.DurationJob(config.WatcherRefreshPeriod), gocron.NewTask(func() {
		hash := common.MustReturn(hashstructure.Hash(watchers, hashstructure.FormatV2, nil))

		refreshed, refreshErr := RefreshWatchers(interruptCtx, kubeClient)
		if refreshErr != nil {
			logger.Error(refreshErr, "An error occurred while refreshing watchers.")

			return
		}

		if hash != common.MustReturn(hashstructure.Hash(refreshed, hashstructure.FormatV2, nil)) {
			logger.Info("Watchers updated, restarting")
			watchers = refreshed
			restart()
		}
	}), gocron.WithSingletonMode(gocron.LimitModeReschedule)))

	scheduler.Start()

	for interruptCtx.Err() == nil {
		if err := StartManager(restartCtx, watchers); err != nil && restartCtx.Err() == nil {
			common.Must(err)
		}

		restartCtx, restart = context.WithCancel(interruptCtx)
	}

	_ = scheduler.Shutdown()
}

func RefreshWatchers(ctx context.Context, kubeClient client.Reader) ([]v1alpha1.Watcher, error) {
	watcherList := v1alpha1.WatcherList{}
	if listErr := kubeClient.List(ctx, &watcherList); listErr != nil {
		return nil, listErr
	}

	watchers := make([]v1alpha1.Watcher, 0, len(watcherList.Items))

	for _, watcher := range watcherList.Items {
		for _, secretKeySelector := range watcher.Spec.ValuesFrom.Secrets {
			var (
				secret         v1.Secret
				specFromSecret v1alpha1.WatcherSpec
			)

			if getErr := kubeClient.Get(ctx, types.NamespacedName{
				Name: secretKeySelector.Name, Namespace: secretKeySelector.Namespace,
			}, &secret); getErr != nil {
				return nil, getErr
			}

			if unmarshallErr := yaml.Unmarshal(secret.Data[secretKeySelector.Key],
				&specFromSecret); unmarshallErr != nil {
				return nil, unmarshallErr
			}

			if mergeErr := mergo.Merge(&watcher, v1alpha1.Watcher{Spec: specFromSecret},
				mergo.WithOverride, mergo.WithAppendSlice); mergeErr != nil {
				return nil, mergeErr
			}
		}

		watchers = append(watchers, watcher)
	}

	return watchers, nil
}

func StartManager(ctx context.Context, watchers []v1alpha1.Watcher) error {
	manager, managerErr := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
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
	})
	if managerErr != nil {
		return managerErr
	}

	for _, watcher := range watchers {
		if setupErr := pkg.NewController(manager.GetClient(), &http.Client{}, watcher.Compile()).
			SetupWithManager(manager); setupErr != nil {
			return setupErr
		}
	}

	common.Must(manager.AddHealthzCheck("healthz", healthz.Ping))
	common.Must(manager.AddReadyzCheck("readyz", healthz.Ping))

	return manager.Start(ctx)
}
