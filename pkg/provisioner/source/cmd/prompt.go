package cmd

import (
	"errors"
	"sort"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/provisioner/source"
	"github.com/cloudposse/atmos/pkg/schema"
)

// PromptForStack shows an interactive selector for stack selection.
// If component is provided, filters stacks to only those containing the component with source configured.
func PromptForStack(cmd *cobra.Command, component string) (string, error) {
	defer perf.Track(nil, "source.cmd.PromptForStack")()

	var args []string
	if component != "" {
		args = []string{component}
	}
	return flags.PromptForMissingRequired(
		"stack",
		"Choose a stack",
		StackFlagCompletion,
		cmd,
		args,
	)
}

// HandlePromptError processes errors from interactive prompts.
// Returns nil if the error should be ignored, or the error if it should propagate.
func HandlePromptError(err error, name string) error {
	defer perf.Track(nil, "source.cmd.HandlePromptError")()

	if err == nil {
		return nil
	}
	if errors.Is(err, errUtils.ErrUserAborted) {
		log.Debug("User aborted selection, exiting with SIGINT code", "prompt", name)
		errUtils.Exit(errUtils.ExitCodeSIGINT)
	}
	if errors.Is(err, errUtils.ErrInteractiveModeNotAvailable) {
		return nil // Fall through to validation.
	}
	return err
}

// StackFlagCompletion provides shell completion for the --stack flag.
// If a component was provided as the first argument, filters stacks to only those
// containing that component with source configured.
func StackFlagCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// If a component was provided as the first argument, filter stacks by that component.
	if len(args) > 0 && args[0] != "" {
		output, err := listStacksWithSourceForComponent(args[0])
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return output, cobra.ShellCompDirectiveNoFileComp
	}

	// Otherwise, list all stacks with any source-configured components.
	output, err := listStacksWithSource()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return output, cobra.ShellCompDirectiveNoFileComp
}

// listStacksWithSourceForComponent returns stacks that contain the specified component with source configured.
func listStacksWithSourceForComponent(component string) ([]string, error) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return nil, err
	}

	stacksMap, err := e.ExecuteDescribeStacks(&atmosConfig, "", nil, nil, nil, false, false, false, false, nil, nil)
	if err != nil {
		return nil, err
	}

	// Filter stacks that contain the specified component with source.
	var stacks []string
	for stackName, stackData := range stacksMap {
		if stackContainsComponentWithSource(stackData, component) {
			stacks = append(stacks, stackName)
		}
	}
	sort.Strings(stacks)
	return stacks, nil
}

// stackContainsComponentWithSource checks if a stack contains the specified terraform component with source.
func stackContainsComponentWithSource(stackData any, component string) bool {
	stackMap, ok := stackData.(map[string]any)
	if !ok {
		return false
	}
	components, ok := stackMap["components"].(map[string]any)
	if !ok {
		return false
	}
	terraform, ok := components["terraform"].(map[string]any)
	if !ok {
		return false
	}
	componentData, hasComponent := terraform[component]
	if !hasComponent {
		return false
	}
	componentMap, ok := componentData.(map[string]any)
	if !ok {
		return false
	}
	return source.HasSource(componentMap)
}

// listStacksWithSource returns all stacks that have at least one component with source configured.
func listStacksWithSource() ([]string, error) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return nil, err
	}

	stacksMap, err := e.ExecuteDescribeStacks(&atmosConfig, "", nil, nil, nil, false, false, false, false, nil, nil)
	if err != nil {
		return nil, err
	}

	// Filter stacks that have any component with source.
	var stacks []string
	for stackName, stackData := range stacksMap {
		if stackHasAnySource(stackData) {
			stacks = append(stacks, stackName)
		}
	}
	sort.Strings(stacks)
	return stacks, nil
}

// stackHasAnySource checks if a stack has any terraform component with source configured.
func stackHasAnySource(stackData any) bool {
	stackMap, ok := stackData.(map[string]any)
	if !ok {
		return false
	}
	components, ok := stackMap["components"].(map[string]any)
	if !ok {
		return false
	}
	terraform, ok := components["terraform"].(map[string]any)
	if !ok {
		return false
	}
	for _, componentData := range terraform {
		componentMap, ok := componentData.(map[string]any)
		if !ok {
			continue
		}
		if source.HasSource(componentMap) {
			return true
		}
	}
	return false
}
