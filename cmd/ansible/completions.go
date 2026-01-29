package ansible

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	l "github.com/cloudposse/atmos/pkg/list"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// stackFlagCompletion provides completion values for the --stack flag.
// This is set on the flag registry to avoid import cycle with internal/exec.
func stackFlagCompletion(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	defer perf.Track(nil, "ansible.stackFlagCompletion")()

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
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

// componentArgCompletion provides completion values for positional component arguments.
func componentArgCompletion(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	defer perf.Track(nil, "ansible.componentArgCompletion")()

	// Skip component completion if one was already provided.
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	stacksMap, err := e.ExecuteDescribeStacks(&atmosConfig, "", nil, nil, nil, false, false, false, false, nil, nil)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	components, err := l.FilterAndListComponents("", stacksMap)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	return components, cobra.ShellCompDirectiveNoFileComp
}

// RegisterAnsibleCompletions registers completion functions for ansible commands.
func RegisterAnsibleCompletions(cmd *cobra.Command) {
	defer perf.Track(nil, "ansible.RegisterAnsibleCompletions")()

	// Set completion for component argument on all subcommands that accept it.
	for _, subCmd := range cmd.Commands() {
		if subCmd.Name() == "playbook" {
			subCmd.ValidArgsFunction = componentArgCompletion
		}
	}
}
