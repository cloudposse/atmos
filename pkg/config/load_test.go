package config

import (
	"os"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestReadEnvAmosConfigPath(t *testing.T) {
	os.Setenv("ATMOS_CLI_CONFIG_PATH", "../../tests/fixtures/scenarios/env")
	defer os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
	v := viper.New()
	readEnvAmosConfigPath(v)
	if v.ConfigFileUsed() == "" {
		t.Errorf("Expected config file to be set, but got empty string")
	}
}

func TestLoadConfigSources(t *testing.T) {
	t.Run("readSystemConfigFunc", func(t *testing.T) {
		readSystemConfigFunc = func(v *viper.Viper) error {
			return ErrAtmosArgConfigNotFound
		}
		defer func() {
			readSystemConfigFunc = readSystemConfig
		}()
		assert.Error(t, loadConfigSources(viper.New(), ""))
	})
	t.Run("readHomeConfigFunc", func(t *testing.T) {
		readHomeConfigFunc = func(v *viper.Viper) error {
			return ErrAtmosArgConfigNotFound
		}
		defer func() {
			readHomeConfigFunc = readHomeConfig
		}()
		assert.Error(t, loadConfigSources(viper.New(), ""))
	})

	t.Run("readEnvAmosConfigPathFunc", func(t *testing.T) {
		readEnvAmosConfigPathFunc = func(v *viper.Viper) error {
			return ErrAtmosArgConfigNotFound
		}
		defer func() {
			readEnvAmosConfigPathFunc = readEnvAmosConfigPath
		}()
		assert.Error(t, loadConfigSources(viper.New(), ""))
	})

	t.Run("readAtmosConfigCliFunc", func(t *testing.T) {
		readAtmosConfigCliFunc = func(v *viper.Viper, atmosCliConfigPath string) error {
			return ErrAtmosArgConfigNotFound
		}
		defer func() {
			readAtmosConfigCliFunc = readAtmosConfigCli
		}()
		assert.Error(t, loadConfigSources(viper.New(), ""))
	})
}

func TestBindEnv(t *testing.T) {
	assert.Panics(t, func() {
		bindEnv(viper.New())
	})
}
