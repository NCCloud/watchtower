package common

import (
	"time"

	"github.com/caarlos0/env/v9"
)

type Config struct {
	EnableLeaderElection bool          `env:"ENABLE_LEADER_ELECTION" envDefault:"false"`
	SyncPeriod           time.Duration `env:"ENABLE_WEBHOOKS" envDefault:"24h"`
	WatcherRefreshPeriod time.Duration `env:"WATCHER_REFRESH_PERIOD" envDefault:"10s"`
}

func NewConfig() *Config {
	operatorConfig := &Config{}
	if err := env.Parse(operatorConfig); err != nil {
		panic(err)
	}

	return operatorConfig
}
