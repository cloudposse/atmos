package vendor

import (
	"fmt"

	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	l "github.com/cloudposse/atmos/pkg/list"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Flag name constants used across vendor commands.
const (
	flagComponent = "component"
	flagType      = "type"
)

// AtmosConfigSetter is an interface for options that need AtmosConfig.
type AtmosConfigSetter interface {
	SetAtmosConfig(cfg *schema.AtmosConfiguration)
}

// initAtmosConfig initializes Atmos configuration and stores it in opts.
func initAtmosConfig[T AtmosConfigSetter](opts T, skipStackValidation bool) error {
	defer perf.Track(nil, "vendor.initAtmosConfig")()

	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, skipStackValidation)
	if err != nil {
		return fmt.Errorf("failed to initialize Atmos config: %w", err)
	}

	opts.SetAtmosConfig(&atmosConfig)
	return nil
}

// componentsArgCompletion provides shell completion for --component flag.
func componentsArgCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	defer perf.Track(nil, "vendor.componentsArgCompletion")()

	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	// Get all stacks to extract components from them.
	stacksMap, err := e.ExecuteDescribeStacks(&atmosConfig, "", nil, nil, nil, false, false, false, false, nil, nil)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	// Extract components from all stacks.
	components, err := l.FilterAndListComponents("", stacksMap)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	return components, cobra.ShellCompDirectiveNoFileComp
}

// addStackCompletion adds --stack flag completion to a command.
func addStackCompletion(cmd *cobra.Command) {
	defer perf.Track(nil, "vendor.addStackCompletion")()

	_ = cmd.RegisterFlagCompletionFunc("stack", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		configAndStacksInfo := schema.ConfigAndStacksInfo{}
		atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		// Get all stacks.
		stacksMap, err := e.ExecuteDescribeStacks(&atmosConfig, "", nil, nil, nil, false, false, false, false, nil, nil)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		stacks, err := l.FilterAndListStacks(stacksMap, "")
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		return stacks, cobra.ShellCompDirectiveNoFileComp
	})
}
