package config

import (
	"os"
	"testing"

	"github.com/spf13/viper"
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
