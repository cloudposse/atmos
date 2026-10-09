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

// sourceComponentTypeAnnotation binds selection to the command builder's component type.
const sourceComponentTypeAnnotation = "source-component-type"

// Function variables for dependency injection in tests.
var (
	initCliConfigForPrompt    = cfg.InitCliConfig
	executeDescribeStacksFunc = e.ExecuteDescribeStacks
)

// PromptForComponent shows an interactive selector for component selection.
// Lists components of the command's type that have source configured.
func PromptForComponent(cmd *cobra.Command) (string, error) {
	defer perf.Track(nil, "source.cmd.PromptForComponent")()

	return flags.PromptForPositionalArg(
		"component",
		"Choose a component",
		ComponentArgCompletion,
		cmd,
		nil,
	)
}

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

// ComponentArgCompletion provides shell completion for the component positional argument.
// Lists components of the command's type that have source configured.
func ComponentArgCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	componentType := cmd.Annotations[sourceComponentTypeAnnotation]
	if componentType == "" {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	if len(args) == 0 {
		output, err := listComponentsWithSource(componentType)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return output, cobra.ShellCompDirectiveNoFileComp
	}
	return nil, cobra.ShellCompDirectiveNoFileComp
}

// StackFlagCompletion provides shell completion for the --stack flag.
// If a component was provided as the first argument, filters stacks to only those
// containing that component with source configured.
func StackFlagCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	componentType := cmd.Annotations[sourceComponentTypeAnnotation]
	if componentType == "" {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	// If a component was provided as the first argument, filter stacks by that component.
	if len(args) > 0 && args[0] != "" {
		output, err := listStacksWithSourceForComponent(componentType, args[0])
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return output, cobra.ShellCompDirectiveNoFileComp
	}

	// Otherwise, list all stacks with any source-configured components.
	output, err := listStacksWithSource(componentType)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return output, cobra.ShellCompDirectiveNoFileComp
}

// listStacksWithSourceForComponent returns stacks that contain the specified component with source configured.
// TODO: Consider caching ExecuteDescribeStacks results for shell completion performance.
// Each list function initializes config and calls ExecuteDescribeStacks independently,
// which can be slow when called repeatedly during tab completion.
func listStacksWithSourceForComponent(componentType, component string) ([]string, error) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := initCliConfigForPrompt(configAndStacksInfo, true)
	if err != nil {
		return nil, err
	}

	stacksMap, err := executeDescribeStacksFunc(&atmosConfig, "", nil, nil, nil, false, false, false, false, nil, nil)
	if err != nil {
		return nil, err
	}

	// Filter stacks that contain the specified component with source.
	var stacks []string
	for stackName, stackData := range stacksMap {
		if stackContainsComponentWithSource(stackData, componentType, component) {
			stacks = append(stacks, stackName)
		}
	}
	sort.Strings(stacks)
	return stacks, nil
}

// stackContainsComponentWithSource checks if a stack contains the specified component type and name with source.
func stackContainsComponentWithSource(stackData any, componentType, component string) bool {
	stackMap, ok := stackData.(map[string]any)
	if !ok {
		return false
	}
	components, ok := stackMap["components"].(map[string]any)
	if !ok {
		return false
	}
	typedComponents, ok := components[componentType].(map[string]any)
	if !ok {
		return false
	}
	componentData, hasComponent := typedComponents[component]
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
func listStacksWithSource(componentType string) ([]string, error) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := initCliConfigForPrompt(configAndStacksInfo, true)
	if err != nil {
		return nil, err
	}

	stacksMap, err := executeDescribeStacksFunc(&atmosConfig, "", nil, nil, nil, false, false, false, false, nil, nil)
	if err != nil {
		return nil, err
	}

	// Filter stacks that have any component with source.
	var stacks []string
	for stackName, stackData := range stacksMap {
		if stackHasAnySource(stackData, componentType) {
			stacks = append(stacks, stackName)
		}
	}
	sort.Strings(stacks)
	return stacks, nil
}

// stackHasAnySource checks if a stack has any component of the specified type with source configured.
func stackHasAnySource(stackData any, componentType string) bool {
	stackMap, ok := stackData.(map[string]any)
	if !ok {
		return false
	}
	components, ok := stackMap["components"].(map[string]any)
	if !ok {
		return false
	}
	typedComponents, ok := components[componentType].(map[string]any)
	if !ok {
		return false
	}
	for _, componentData := range typedComponents {
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

// listComponentsWithSource returns all components of the specified type that have source configured in any stack.
func listComponentsWithSource(componentType string) ([]string, error) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := initCliConfigForPrompt(configAndStacksInfo, true)
	if err != nil {
		return nil, err
	}

	stacksMap, err := executeDescribeStacksFunc(&atmosConfig, "", nil, nil, nil, false, false, false, false, nil, nil)
	if err != nil {
		return nil, err
	}

	// Collect unique component names with source from all stacks.
	componentSet := make(map[string]struct{})
	for _, stackData := range stacksMap {
		collectComponentsWithSource(stackData, componentType, componentSet)
	}

	componentsList := make([]string, 0, len(componentSet))
	for name := range componentSet {
		componentsList = append(componentsList, name)
	}
	sort.Strings(componentsList)
	return componentsList, nil
}

// collectComponentsWithSource extracts components of the specified type with source from a stack.
func collectComponentsWithSource(stackData any, componentType string, componentSet map[string]struct{}) {
	stackMap, ok := stackData.(map[string]any)
	if !ok {
		return
	}
	components, ok := stackMap["components"].(map[string]any)
	if !ok {
		return
	}
	typedComponents, ok := components[componentType].(map[string]any)
	if !ok {
		return
	}
	for componentName, componentData := range typedComponents {
		componentMap, ok := componentData.(map[string]any)
		if !ok {
			continue
		}
		if source.HasSource(componentMap) {
			componentSet[componentName] = struct{}{}
		}
	}
}
