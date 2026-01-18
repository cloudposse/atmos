// Package shared provides utilities shared between terraform and its subpackages.
package shared

import (
	"errors"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Package-level variables for dependency injection (enables testing).
var (
	initCliConfig         = cfg.InitCliConfig
	executeDescribeStacks = e.ExecuteDescribeStacks
)

// buildConfigAndStacksInfo creates a ConfigAndStacksInfo populated with global CLI flags.
// This ensures --base-path, --config, --config-path, and --profile flags are respected.
func buildConfigAndStacksInfo(cmd *cobra.Command) schema.ConfigAndStacksInfo {
	if cmd == nil {
		return schema.ConfigAndStacksInfo{}
	}
	v := viper.GetViper()
	globalFlags := flags.ParseGlobalFlags(cmd, v)
	return schema.ConfigAndStacksInfo{
		AtmosBasePath:           globalFlags.BasePath,
		AtmosConfigFilesFromArg: globalFlags.Config,
		AtmosConfigDirsFromArg:  globalFlags.ConfigPath,
		ProfilesFromArg:         globalFlags.Profile,
	}
}

// PromptForComponent shows an interactive selector for component selection.
// If stack is provided, filters components to only those in that stack.
func PromptForComponent(cmd *cobra.Command, stack string) (string, error) {
	// Create a completion function that respects the stack filter.
	completionFunc := func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return componentsArgCompletionWithStack(cmd, args, toComplete, stack)
	}

	return flags.PromptForPositionalArg(
		"component",
		"Choose a component",
		completionFunc,
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
// Checks for --stack flag and filters components accordingly.
func ComponentsArgCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) == 0 {
		// Check if --stack flag was provided.
		stack := ""
		if cmd != nil {
			if stackFlag := cmd.Flag("stack"); stackFlag != nil {
				stack = stackFlag.Value.String()
			}
		}
		return componentsArgCompletionWithStack(cmd, args, toComplete, stack)
	}
	return nil, cobra.ShellCompDirectiveNoFileComp
}

// componentsArgCompletionWithStack provides shell completion for component arguments with optional stack filtering.
func componentsArgCompletionWithStack(cmd *cobra.Command, args []string, toComplete string, stack string) ([]string, cobra.ShellCompDirective) {
	// toComplete kept for Cobra completion function signature compatibility.
	_ = toComplete

	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var output []string
	var err error

	if stack != "" {
		output, err = listTerraformComponentsForStack(cmd, stack)
	} else {
		output, err = listTerraformComponents(cmd)
	}

	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return output, cobra.ShellCompDirectiveNoFileComp
}

// StackFlagCompletion provides shell completion for the --stack flag.
// If a component was provided as the first positional argument, it filters stacks
// to only those containing that component.
func StackFlagCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// If a component was provided as the first argument, filter stacks by that component.
	if len(args) > 0 && args[0] != "" {
		output, err := listStacksForComponent(cmd, args[0])
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return output, cobra.ShellCompDirectiveNoFileComp
	}

	// Otherwise, list all stacks.
	output, err := listAllStacks(cmd)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return output, cobra.ShellCompDirectiveNoFileComp
}

// IsComponentDeployable checks if a component can be deployed (not abstract, not disabled).
// Returns false for components with metadata.type: abstract or metadata.enabled: false.
func IsComponentDeployable(componentConfig any) bool {
	// Handle nil or non-map configs - assume deployable.
	configMap, ok := componentConfig.(map[string]any)
	if !ok {
		return true
	}

	// Check metadata section.
	metadata, ok := configMap["metadata"].(map[string]any)
	if !ok {
		return true // No metadata means deployable.
	}

	// Check if component is abstract.
	if componentType, ok := metadata["type"].(string); ok && componentType == "abstract" {
		return false
	}

	// Check if component is disabled.
	if enabled, ok := metadata["enabled"].(bool); ok && !enabled {
		return false
	}

	return true
}

// FilterDeployableComponents returns only components that can be deployed.
// Filters out abstract and disabled components from the terraform components map.
// Returns a sorted slice of deployable component names.
func FilterDeployableComponents(terraformComponents map[string]any) []string {
	if len(terraformComponents) == 0 {
		return []string{}
	}

	var components []string
	for name, config := range terraformComponents {
		if IsComponentDeployable(config) {
			components = append(components, name)
		}
	}

	sort.Strings(components)
	return components
}

// listTerraformComponents lists all deployable terraform components across all stacks.
// Filters out abstract and disabled components.
// The cmd parameter is used to respect global CLI flags (--base-path, --config, --config-path, --profile).
func listTerraformComponents(cmd *cobra.Command) ([]string, error) {
	configAndStacksInfo := buildConfigAndStacksInfo(cmd)
	atmosConfig, err := initCliConfig(configAndStacksInfo, true)
	if err != nil {
		return nil, err
	}

	stacksMap, err := executeDescribeStacks(&atmosConfig, "", nil, nil, nil, false, false, false, false, nil, nil)
	if err != nil {
		return nil, err
	}

	// Collect unique deployable component names from all stacks.
	componentSet := make(map[string]struct{})
	for _, stackData := range stacksMap {
		if stackMap, ok := stackData.(map[string]any); ok {
			if components, ok := stackMap["components"].(map[string]any); ok {
				if terraform, ok := components["terraform"].(map[string]any); ok {
					// Filter to only deployable components.
					deployable := FilterDeployableComponents(terraform)
					for _, name := range deployable {
						componentSet[name] = struct{}{}
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

// listTerraformComponentsForStack lists deployable terraform components for a specific stack.
// Filters out abstract and disabled components.
// If stack is empty, returns components from all stacks.
// The cmd parameter is used to respect global CLI flags (--base-path, --config, --config-path, --profile).
func listTerraformComponentsForStack(cmd *cobra.Command, stack string) ([]string, error) {
	if stack == "" {
		return listTerraformComponents(cmd)
	}

	configAndStacksInfo := buildConfigAndStacksInfo(cmd)
	atmosConfig, err := initCliConfig(configAndStacksInfo, true)
	if err != nil {
		return nil, err
	}

	stacksMap, err := executeDescribeStacks(&atmosConfig, stack, nil, nil, nil, false, false, false, false, nil, nil)
	if err != nil {
		return nil, err
	}

	// Get components from the specified stack only.
	stackData, exists := stacksMap[stack]
	if !exists {
		return []string{}, nil
	}

	stackMap, ok := stackData.(map[string]any)
	if !ok {
		return []string{}, nil
	}

	components, ok := stackMap["components"].(map[string]any)
	if !ok {
		return []string{}, nil
	}

	terraform, ok := components["terraform"].(map[string]any)
	if !ok {
		return []string{}, nil
	}

	// Filter to only deployable components and return sorted.
	return FilterDeployableComponents(terraform), nil
}

// listStacksForComponent returns stacks that contain the specified component.
// The cmd parameter is used to respect global CLI flags (--base-path, --config, --config-path, --profile).
func listStacksForComponent(cmd *cobra.Command, component string) ([]string, error) {
	configAndStacksInfo := buildConfigAndStacksInfo(cmd)
	atmosConfig, err := initCliConfig(configAndStacksInfo, true)
	if err != nil {
		return nil, err
	}

	stacksMap, err := executeDescribeStacks(&atmosConfig, "", nil, nil, nil, false, false, false, false, nil, nil)
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
// The cmd parameter is used to respect global CLI flags (--base-path, --config, --config-path, --profile).
func listAllStacks(cmd *cobra.Command) ([]string, error) {
	configAndStacksInfo := buildConfigAndStacksInfo(cmd)
	atmosConfig, err := initCliConfig(configAndStacksInfo, true)
	if err != nil {
		return nil, err
	}

	stacksMap, err := executeDescribeStacks(&atmosConfig, "", nil, nil, nil, false, false, false, false, nil, nil)
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

// ValidateStackExists checks if the provided stack name exists and returns
// an error with suggestions if it doesn't.
// The cmd parameter is used to respect global CLI flags (--base-path, --config, --config-path, --profile).
func ValidateStackExists(cmd *cobra.Command, stack string) error {
	stacks, err := listAllStacks(cmd)
	if err != nil {
		return err
	}

	for _, s := range stacks {
		if s == stack {
			return nil // Stack exists.
		}
	}

	// Stack not found - use ErrorBuilder pattern with sentinel error.
	return errUtils.Build(errUtils.ErrInvalidStack).
		WithCausef("stack `%s` does not exist", stack).
		WithExplanation("The specified stack was not found in the configuration").
		WithHintf("Available stacks: %s", strings.Join(stacks, ", ")).
		WithContext("stack", stack).
		Err()
}
