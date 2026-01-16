package exec

import (
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// ExecuteTerraformQuery executes `atmos terraform <command> --query <yq-expression --stack <stack>`.
func ExecuteTerraformQuery(info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(nil, "exec.ExecuteTerraformQuery")()

	atmosConfig, err := cfg.InitCliConfig(*info, true)
	if err != nil {
		return err
	}

	// Always use debug level for internal logging.
	logFunc := log.Debug

	stacks, err := ExecuteDescribeStacks(
		&atmosConfig,
		info.Stack,
		info.Components,
		[]string{cfg.TerraformComponentType},
		nil,
		false,
		info.ProcessTemplates,
		info.ProcessFunctions,
		false,
		info.Skip,
		nil, // AuthManager - not needed for terraform query
	)
	if err != nil {
		return err
	}

	// Track how many components were processed.
	processedCount := 0

	err = walkTerraformComponents(stacks, func(stackName, componentName string, componentSection map[string]any) error {
		processed, err := processTerraformComponent(&atmosConfig, info, stackName, componentName, componentSection, logFunc)
		if processed {
			processedCount++
		}
		return err
	})
	if err != nil {
		return err
	}

	// Show success message if no components matched the criteria.
	if processedCount == 0 {
		_ = ui.Success("No components matched")
	}

	return nil
}
