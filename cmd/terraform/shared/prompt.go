// Package shared provides utilities shared between terraform and its subpackages.
package shared

import (
	"errors"
	"sort"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// PromptForComponent shows an interactive selector for component selection.
func PromptForComponent(cmd *cobra.Command) (string, error) {
	return flags.PromptForPositionalArg(
		"component",
		"Choose a component",
		ComponentsArgCompletion,
		cmd,
		nil,
	)
}

// PromptForStack shows an interactive selector for stack selection.
// If component is provided, filters stacks to only those containing the component.
func PromptForStack(cmd *cobra.Command, component string) (string, error) {
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

// ComponentsArgCompletion provides shell completion for component positional arguments.
func ComponentsArgCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) == 0 {
		output, err := listTerraformComponents()
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return output, cobra.ShellCompDirectiveNoFileComp
	}
	return nil, cobra.ShellCompDirectiveNoFileComp
}

// StackFlagCompletion provides shell completion for the --stack flag.
// If a component was provided as the first positional argument, it filters stacks
// to only those containing that component.
func StackFlagCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// If a component was provided as the first argument, filter stacks by that component.
	if len(args) > 0 && args[0] != "" {
		output, err := listStacksForComponent(args[0])
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return output, cobra.ShellCompDirectiveNoFileComp
	}

	// Otherwise, list all stacks.
	output, err := listAllStacks()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return output, cobra.ShellCompDirectiveNoFileComp
}

// listTerraformComponents lists all terraform components.
func listTerraformComponents() ([]string, error) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return nil, err
	}

	stacksMap, err := e.ExecuteDescribeStacks(&atmosConfig, "", nil, nil, nil, false, false, false, false, nil, nil)
	if err != nil {
		return nil, err
	}

	// Collect unique component names from all stacks.
	componentSet := make(map[string]struct{})
	for _, stackData := range stacksMap {
		if stackMap, ok := stackData.(map[string]any); ok {
			if components, ok := stackMap["components"].(map[string]any); ok {
				if terraform, ok := components["terraform"].(map[string]any); ok {
					for componentName := range terraform {
						componentSet[componentName] = struct{}{}
					}
				}
			}
		}
	}

	components := make([]string, 0, len(componentSet))
	for name := range componentSet {
		components = append(components, name)
	}
	sort.Strings(components)
	return components, nil
}

// listStacksForComponent returns stacks that contain the specified component.
func listStacksForComponent(component string) ([]string, error) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return nil, err
	}

	stacksMap, err := e.ExecuteDescribeStacks(&atmosConfig, "", nil, nil, nil, false, false, false, false, nil, nil)
	if err != nil {
		return nil, err
	}

	// Filter stacks that contain the specified component.
	var stacks []string
	for stackName, stackData := range stacksMap {
		if stackContainsComponent(stackData, component) {
			stacks = append(stacks, stackName)
		}
	}
	sort.Strings(stacks)
	return stacks, nil
}

// stackContainsComponent checks if a stack contains the specified terraform component.
func stackContainsComponent(stackData any, component string) bool {
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
	_, hasComponent := terraform[component]
	return hasComponent
}

// listAllStacks returns all stacks.
func listAllStacks() ([]string, error) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return nil, err
	}

	stacksMap, err := e.ExecuteDescribeStacks(&atmosConfig, "", nil, nil, nil, false, false, false, false, nil, nil)
	if err != nil {
		return nil, err
	}

	stacks := make([]string, 0, len(stacksMap))
	for stackName := range stacksMap {
		stacks = append(stacks, stackName)
	}
	sort.Strings(stacks)
	return stacks, nil
}
