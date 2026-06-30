package atmos

import (
	"github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// currentStackConfig returns a stack-processed config for live stack graph
// operations. MCP servers are long-running, so stack files can be added or fixed
// after startup; loaded runtime configs are refreshed when stack tools run.
func currentStackConfig(atmosConfig *schema.AtmosConfiguration) (*schema.AtmosConfiguration, error) {
	if atmosConfig == nil {
		clearStackProcessingCaches()
		config, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
		if err != nil {
			return nil, err
		}
		return &config, nil
	}

	if !atmosConfig.Initialized {
		return atmosConfig, nil
	}

	clearStackProcessingCaches()
	config, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func clearStackProcessingCaches() {
	exec.ClearFindStacksMapCache()
	exec.ClearBaseComponentConfigCache()
	exec.ClearLocalsExtractionCache()
	exec.ClearFileContentCache()
}
