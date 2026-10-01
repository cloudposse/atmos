package config

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestFrozenLockFileEnvOverridesConfig(t *testing.T) {
	for _, value := range []string{"true", "false"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("ATMOS_TOOLCHAIN_FROZEN_LOCK_FILE", value)
			v := viper.New()
			v.SetDefault("toolchain.frozen_lock_file", value != "true")
			setEnv(v)
			var config schema.AtmosConfiguration
			require.NoError(t, v.Unmarshal(&config))
			require.Equal(t, value == "true", config.Toolchain.FrozenLockFile)
		})
	}
}
