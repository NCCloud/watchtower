package main

import (
	"fmt"

	"github.com/nccloud/watchtower/pkg"
	"github.com/nccloud/watchtower/pkg/common"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"k8s.io/apimachinery/pkg/runtime"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const (
	metricPort = 8083
	healthPort = 8084
)

func main() {
	config := common.MustReturn(common.NewConfig("./config.yaml"))
	logger := zap.New()
	manager := common.MustReturn(ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: runtime.NewScheme(),
		Logger: logger,
		Cache: cache.Options{
			SyncPeriod: config.GetSyncPeriod(),
		},
		Metrics: server.Options{
			BindAddress: fmt.Sprintf(":%d", metricPort),
		},
		HealthProbeBindAddress: fmt.Sprintf(":%d", healthPort),
		LeaderElection:         config.LeaderElection,
		LeaderElectionID:       "watchtower.cloud.spaceship.com",
	}))

	ctrl.SetLogger(logger)

	for _, watcher := range config.Watchers {
		watcher.Compile()
		common.Must(pkg.NewController(manager.GetClient(), watcher).SetupWithManager(manager))
	}

	common.Must(manager.AddHealthzCheck("healthz", healthz.Ping))
	common.Must(manager.AddHealthzCheck("readyz", healthz.Ping))
	common.Must(manager.Start(ctrl.SetupSignalHandler()))
}
