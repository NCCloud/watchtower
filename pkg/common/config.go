package common

import (
	"time"

	"github.com/caarlos0/env/v10"
)

type Config struct {
	EnableLeaderElection bool          `env:"ENABLE_LEADER_ELECTION" envDefault:"false"`
	SyncPeriod           time.Duration `env:"SYNC_PERIOD" envDefault:"24h"`
	WatcherRefreshPeriod time.Duration `env:"WATCHER_REFRESH_PERIOD" envDefault:"15s"`
}

func NewConfig() *Config {
	operatorConfig := &Config{}
	Must(env.Parse(operatorConfig))

	return operatorConfig
}
