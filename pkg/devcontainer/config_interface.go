package devcontainer

//go:generate go run go.uber.org/mock/mockgen@latest -source=config_interface.go -destination=mock_config_test.go -package=devcontainer

import (
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ConfigLoader handles loading devcontainer configurations.
type ConfigLoader interface {
	// LoadConfig loads configuration for a specific devcontainer by name.
	LoadConfig(atmosConfig *schema.AtmosConfiguration, name string) (*Config, *Settings, error)

	// LoadAllConfigs loads all devcontainer configurations.
	LoadAllConfigs(atmosConfig *schema.AtmosConfiguration) (map[string]*Config, error)
}

// configLoaderImpl implements ConfigLoader using existing functions.
type configLoaderImpl struct{}

// NewConfigLoader creates a new ConfigLoader.
func NewConfigLoader() ConfigLoader {
	defer perf.Track(nil, "devcontainer.NewConfigLoader")()

	return &configLoaderImpl{}
}

// LoadConfig loads configuration for a specific devcontainer.
func (c *configLoaderImpl) LoadConfig(atmosConfig *schema.AtmosConfiguration, name string) (*Config, *Settings, error) {
	defer perf.Track(atmosConfig, "devcontainer.configLoaderImpl.LoadConfig")()

	// Reload config to ensure we have the latest with all fields populated.
	freshConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return nil, nil, err
	}

	return LoadConfig(&freshConfig, name)
}

// LoadAllConfigs loads all devcontainer configurations.
func (c *configLoaderImpl) LoadAllConfigs(atmosConfig *schema.AtmosConfiguration) (map[string]*Config, error) {
	defer perf.Track(atmosConfig, "devcontainer.configLoaderImpl.LoadAllConfigs")()

	// Reload config to ensure we have the latest with all fields populated.
	freshConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return nil, err
	}

	return LoadAllConfigs(&freshConfig)
}
