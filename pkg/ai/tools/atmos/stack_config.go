package atmos

import (
	"errors"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// currentStackConfig returns a stack-processed config for live stack graph
// operations. MCP servers are long-running, so stack files can be added or fixed
// after startup; loaded runtime configs are refreshed when stack tools run.
func currentStackConfig(atmosConfig *schema.AtmosConfiguration) (*schema.AtmosConfiguration, error) {
	if atmosConfig == nil {
		clearStackProcessingCaches()
		return refreshStackConfig()
	}

	if !atmosConfig.Initialized {
		return atmosConfig, nil
	}

	clearStackProcessingCaches()
	return refreshStackConfig()
}

// refreshStackConfig reprocesses stack manifests, treating "no stacks yet" as an
// empty (but valid) config rather than an error. This is expected for brand-new
// projects that don't have any stack manifests written yet.
func refreshStackConfig() (*schema.AtmosConfiguration, error) {
	config, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	if err != nil {
		if errors.Is(err, errUtils.ErrFailedToFindImport) || errors.Is(err, errUtils.ErrNoStackManifestsFound) {
			log.Warn(
				"No Atmos stack manifests found; treating as an empty project",
				"hint", "This is expected for new projects that don't have any stacks written yet",
				"error", err,
			)
			config, err = cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
			if err != nil {
				return nil, err
			}
			return &config, nil
		}
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
