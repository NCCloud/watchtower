package main

import (
	"context"
	"fmt"
	"github.com/go-co-op/gocron"
	"github.com/mitchellh/hashstructure/v2"
	"github.com/nccloud/watchtower/pkg"
	"github.com/nccloud/watchtower/pkg/apis/v1alpha1"
	"github.com/nccloud/watchtower/pkg/common"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"time"
)

const (
	metricPort = 8083
	healthPort = 8084
)

var (
	logger       = zap.New()
	config       = common.NewConfig()
	scheme       = runtime.NewScheme()
	interruptCtx = ctrl.SetupSignalHandler()
	restartCtx   context.Context
	restart      context.CancelFunc
	kubeClient   client.Client
	scheduler    = gocron.NewScheduler(time.UTC)
	watchers     []v1alpha1.WatcherSpec
)

func main() {
	common.Must(clientgoscheme.AddToScheme(scheme))
	common.Must(v1alpha1.AddToScheme(scheme))

	kubeClient = common.MustReturn(client.New(ctrl.GetConfigOrDie(), client.Options{
		Scheme: scheme,
	}))

	RefreshWatchers(context.Background(), kubeClient)

	common.MustReturn(scheduler.Every(config.WatcherRefreshPeriod).WaitForSchedule().Do(func() {
		hash := common.MustReturn(hashstructure.Hash(watchers, hashstructure.FormatV2, nil))
		RefreshWatchers(interruptCtx, kubeClient)
		if hash != common.MustReturn(hashstructure.Hash(watchers, hashstructure.FormatV2, nil)) {
			logger.Info("Restarting watchtower")
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

func RefreshWatchers(ctx context.Context, kubeClient client.Client) {
	watcherList := v1alpha1.WatcherList{}
	common.Must(kubeClient.List(ctx, &watcherList))
	
	watchers = []v1alpha1.WatcherSpec{}
	for _, watcher := range watcherList.Items {
		watchers = append(watchers, watcher.Spec)
	}
}

func StartManager(ctx context.Context, watchers []v1alpha1.WatcherSpec) {
	ctrl.SetLogger(logger)

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
		LeaderElection:                true,
		LeaderElectionNamespace:       "default",
		LeaderElectionID:              "watchtower.cloud.spaceship.com",
		LeaderElectionReleaseOnCancel: true,
	}))

	for _, watcher := range watchers {
		watcher.Compile()
		common.Must(pkg.NewController(manager.GetClient(), watcher).SetupWithManager(manager))
	}

	common.Must(manager.AddHealthzCheck("healthz", healthz.Ping))
	common.Must(manager.AddHealthzCheck("readyz", healthz.Ping))
	common.Must(manager.Start(ctx))

	print("\n\n\n\n\n\n")
}
