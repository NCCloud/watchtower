package common

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewConfig(t *testing.T) {
	//given
	os.Clearenv()

	//when
	config := NewConfig()

	//then
	assert.NotNil(t, config)
	assert.False(t, config.EnableLeaderElection)
	assert.Equal(t, 24*time.Hour, config.SyncPeriod)
}

func TestNewConfig_WithCustomValues(t *testing.T) {
	//given
	os.Clearenv()
	os.Setenv("ENABLE_LEADER_ELECTION", "true")
	os.Setenv("SYNC_PERIOD", "2h")
	defer os.Clearenv()

	//when
	config := NewConfig()

	//then
	assert.True(t, config.EnableLeaderElection)
	assert.Equal(t, 2*time.Hour, config.SyncPeriod)
}

func TestNewConfig_WithInvalidSyncPeriod(t *testing.T) {
	//given
	os.Clearenv()
	os.Setenv("SYNC_PERIOD", "invalid")
	defer os.Clearenv()

	//when
	defer func() {
		errRecover := recover()

		//then
		assert.NotNil(t, errRecover)
		assert.Contains(t, errRecover.(error).Error(), "parse error")
	}()

	NewConfig()
}

func TestConfig_Defaults(t *testing.T) {
	//given
	config := Config{}

	//when

	//then
	assert.False(t, config.EnableLeaderElection, "EnableLeaderElection default should be false")
	assert.Zero(t, config.SyncPeriod, "SyncPeriod default should be zero before env parsing")
}
