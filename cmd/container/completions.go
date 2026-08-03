package container

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	l "github.com/cloudposse/atmos/pkg/list"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// globalInfoForCompletion builds a minimal ConfigAndStacksInfo from global flags
// so completion honors config-selection flags.
func globalInfoForCompletion(cmd *cobra.Command) schema.ConfigAndStacksInfo {
	globalFlags := flags.ParseGlobalFlags(cmd, viper.GetViper())
	return schema.ConfigAndStacksInfo{
		AtmosBasePath:           globalFlags.BasePath,
		AtmosConfigFilesFromArg: globalFlags.Config,
		AtmosConfigDirsFromArg:  globalFlags.ConfigPath,
		ProfilesFromArg:         globalFlags.Profile,
	}
}

// stackFlagCompletion provides completion values for the --stack flag.
func stackFlagCompletion(cmd *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	defer perf.Track(nil, "container.stackFlagCompletion")()

	atmosConfig, err := cfg.InitCliConfig(globalInfoForCompletion(cmd), true)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	stacksMap, err := e.ExecuteDescribeStacks(&atmosConfig, "", nil, nil, nil, false, false, false, false, nil, nil)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	stacks, err := l.FilterAndListStacks(stacksMap, "")
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return stacks, cobra.ShellCompDirectiveNoFileComp
}

// componentArgCompletion provides completion values for the component argument.
func componentArgCompletion(cmd *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	defer perf.Track(nil, "container.componentArgCompletion")()

	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	// Honor the --stack flag and restrict suggestions to container components so
	// completion never offers non-container or wrong-stack components.
	stack := ""
	if stackFlag := cmd.Flag("stack"); stackFlag != nil {
		stack = stackFlag.Value.String()
	}
	atmosConfig, err := cfg.InitCliConfig(globalInfoForCompletion(cmd), true)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	stacksMap, err := e.ExecuteDescribeStacks(&atmosConfig, stack, nil, []string{cfg.ContainerComponentType}, nil, false, false, false, false, nil, nil)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	components, err := component.ListAllComponents(context.Background(), cfg.ContainerComponentType, stacksMap)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return components, cobra.ShellCompDirectiveNoFileComp
}

// RegisterContainerCompletions registers completion functions for the container
// command. Every subcommand takes a component argument.
func RegisterContainerCompletions(cmd *cobra.Command) {
	defer perf.Track(nil, "container.RegisterContainerCompletions")()

	for _, subCmd := range cmd.Commands() {
		subCmd.ValidArgsFunction = componentArgCompletion
	}
}
