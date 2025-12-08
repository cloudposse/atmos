package exec

import (
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ExecuteTerraformQuery executes `atmos terraform <command> --query <yq-expression --stack <stack>`.
func ExecuteTerraformQuery(info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(nil, "exec.ExecuteTerraformQuery")()

	atmosConfig, err := cfg.InitCliConfig(*info, true)
	if err != nil {
		return err
	}

	var logFunc func(msg any, keyvals ...any)
	if info.DryRun {
		logFunc = log.Info
	} else {
		logFunc = log.Debug
	}

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

	err = walkTerraformComponents(stacks, func(stackName, componentName string, componentSection map[string]any) error {
		return processTerraformComponent(&atmosConfig, info, stackName, componentName, componentSection, logFunc)
	})
	if err != nil {
		return err
	}

	return nil
}
