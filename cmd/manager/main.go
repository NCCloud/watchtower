package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/nccloud/watchtower/pkg"
	"github.com/nccloud/watchtower/pkg/models"
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
	config, configErr := models.NewConfig("./config.yaml")
	if configErr != nil {
		panic(configErr)
	}

	compiledConfig, compiledConfigErr := config.Compile()
	if compiledConfigErr != nil {
		panic(compiledConfigErr)
	}

	manager, managerErr := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 runtime.NewScheme(),
		Logger:                 zap.New(),
		MetricsBindAddress:     fmt.Sprintf(":%d", metricPort),
		HealthProbeBindAddress: fmt.Sprintf(":%d", healthPort),
		LeaderElection:         strings.ToLower(os.Getenv("ENABLE_LEADER_ELECTION")) == "true",
		LeaderElectionID:       "watchtower.spaceship.com",
	})
	if managerErr != nil {
		panic(managerErr)
	}

	client := manager.GetClient()

	for _, flow := range compiledConfig.Flows {
		if newControllerErr := pkg.NewController(client, flow).SetupWithManager(manager); newControllerErr != nil {
			panic(newControllerErr)
		}
	}

	if healthCheckErr := manager.AddHealthzCheck("healthz", healthz.Ping); healthCheckErr != nil {
		panic(healthCheckErr)
	}

	if readyCheckErr := manager.AddReadyzCheck("readyz", healthz.Ping); readyCheckErr != nil {
		panic(readyCheckErr)
	}

	if startManagerErr := manager.Start(ctrl.SetupSignalHandler()); startManagerErr != nil {
		panic(startManagerErr)
	}
}
