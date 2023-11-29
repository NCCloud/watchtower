package main

import (
	"context"
	"fmt"
	"time"

	"dario.cat/mergo"
	"github.com/go-co-op/gocron"
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
	metricPort   = 8083
	healthPort   = 8084
	logger       = zap.New()
	config       = common.NewConfig()
	scheme       = runtime.NewScheme()
	interruptCtx = ctrl.SetupSignalHandler()
	restartCtx   context.Context
	restart      context.CancelFunc
	kubeClient   client.Client
	scheduler    = gocron.NewScheduler(time.UTC)
	watchers     []v1alpha1.Watcher
)

func main() {
	ctrl.SetLogger(logger)
	common.Must(clientgoscheme.AddToScheme(scheme))
	common.Must(v1alpha1.AddToScheme(scheme))

	kubeClient = common.MustReturn(client.New(ctrl.GetConfigOrDie(), client.Options{
		Scheme: scheme,
	}))

	common.Must(RefreshWatchers(context.Background(), kubeClient))

	common.MustReturn(scheduler.Every(config.WatcherRefreshPeriod).SingletonMode().WaitForSchedule().Do(func() {
		hash := common.MustReturn(hashstructure.Hash(watchers, hashstructure.FormatV2, nil))

		if refreshErr := RefreshWatchers(interruptCtx, kubeClient); refreshErr != nil {
			logger.Error(refreshErr, "An error occurred while refreshing watchers.")

			return
		}

		if hash != common.MustReturn(hashstructure.Hash(watchers, hashstructure.FormatV2, nil)) {
			logger.Info("Watchers updated, restarting")
			restart()
		}
	}))

	scheduler.StartAsync()

	for interruptCtx.Err() == nil {
		restartCtx, restart = context.WithCancel(interruptCtx)
		StartManager(restartCtx, watchers)
	}

	scheduler.Stop()
}

func RefreshWatchers(ctx context.Context, kubeClient client.Reader) error {
	watcherList := v1alpha1.WatcherList{}
	if listErr := kubeClient.List(ctx, &watcherList); listErr != nil {
		return listErr
	}

	watchers = []v1alpha1.Watcher{}

	for _, watcher := range watcherList.Items {
		tmpWatcher := watcher

		for _, secretKeySelector := range tmpWatcher.Spec.ValuesFrom.Secrets {
			var (
				secret         v1.Secret
				specFromSecret v1alpha1.WatcherSpec
			)

			if getErr := kubeClient.Get(ctx, types.NamespacedName{
				Name: secretKeySelector.Name, Namespace: secretKeySelector.Namespace,
			}, &secret); getErr != nil {
				return getErr
			}

			if unmarshallErr := yaml.Unmarshal(secret.Data[secretKeySelector.Key],
				&specFromSecret); unmarshallErr != nil {
				return unmarshallErr
			}

			if mergeErr := mergo.Merge(&tmpWatcher, v1alpha1.Watcher{Spec: specFromSecret},
				mergo.WithOverride, mergo.WithAppendSlice); mergeErr != nil {
				return mergeErr
			}
		}

		watchers = append(watchers, tmpWatcher)
	}

	return nil
}

func StartManager(ctx context.Context, watchers []v1alpha1.Watcher) {
	manager := common.MustReturn(ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
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

	for _, watcher := range watchers {
		common.Must(pkg.NewController(manager.GetClient(), watcher.Compile()).SetupWithManager(manager))
	}

	common.Must(manager.AddHealthzCheck("healthz", healthz.Ping))
	common.Must(manager.AddReadyzCheck("readyz", healthz.Ping))
	common.Must(manager.Start(ctx))
}
