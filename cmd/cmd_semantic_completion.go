package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	semanticTypeStack     = "stack"
	semanticTypeComponent = "component"
)

// loadStacksMapForCompletion loads the stacks map for shell completion.
// This is used by completion functions to get available stacks and components.
func loadStacksMapForCompletion() (map[string]any, error) {
	defer perf.Track(nil, "cmd.loadStacksMapForCompletion")()

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	if err != nil {
		return nil, err
	}
	return e.ExecuteDescribeStacks(&atmosConfig, "", nil, nil, nil, false, false, false, false, nil, nil)
}

// customComponentCompletion returns a completion function for a custom component type.
// This enables tab completion for custom commands with semantic-typed arguments.
func customComponentCompletion(componentType string) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		stacksMap, err := loadStacksMapForCompletion()
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		components, err := component.ListAllComponents(context.Background(), componentType, stacksMap)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return components, cobra.ShellCompDirectiveNoFileComp
	}
}

// registerSemanticFlagCompletions registers completion for flags with semantic types.
// This adds tab completion for flags like --stack (semantic_type: stack) and
// --component (semantic_type: component) in custom commands.
func registerSemanticFlagCompletions(cmd *cobra.Command, commandConfig *schema.Command) {
	defer perf.Track(nil, "cmd.registerSemanticFlagCompletions")()

	if commandConfig.Component == nil {
		return
	}

	for _, flag := range commandConfig.Flags {
		var err error
		switch flag.SemanticType {
		case semanticTypeStack:
			err = cmd.RegisterFlagCompletionFunc(flag.Name, StackFlagCompletion)
		case semanticTypeComponent:
			err = cmd.RegisterFlagCompletionFunc(flag.Name, customComponentCompletion(commandConfig.Component.Type))
		default:
			continue
		}
		if err != nil {
			log.Trace("Failed to register flag completion", "flag", flag.Name, "error", err)
		}
	}
}

// setSemanticArgCompletion sets ValidArgsFunction for the first semantic-typed argument.
// This enables tab completion for positional arguments with type: component or type: stack.
func setSemanticArgCompletion(cmd *cobra.Command, commandConfig *schema.Command) {
	defer perf.Track(nil, "cmd.setSemanticArgCompletion")()

	if commandConfig.Component == nil {
		return
	}

	if len(commandConfig.Arguments) > 0 {
		arg := commandConfig.Arguments[0]
		switch arg.Type {
		case semanticTypeComponent:
			cmd.ValidArgsFunction = customComponentCompletion(commandConfig.Component.Type)
		case semanticTypeStack:
			cmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
				stacks, err := listStacks(cmd)
				if err != nil {
					return nil, cobra.ShellCompDirectiveNoFileComp
				}
				return stacks, cobra.ShellCompDirectiveNoFileComp
			}
		}
	}
}

// PromptConfig holds dependencies for interactive prompting.
// This enables testability by allowing mock implementations.
type PromptConfig struct {
	ListComponents func(ctx context.Context, componentType string, stacksMap map[string]any) ([]string, error)
	ListStacks     func(cmd *cobra.Command) ([]string, error)
	PromptArg      func(name, title string, completionFunc flags.CompletionFunc, cmd *cobra.Command, args []string) (string, error)
	PromptFlag     func(name, title string, completionFunc flags.CompletionFunc, cmd *cobra.Command, args []string) (string, error)
}

// DefaultPromptConfig returns production implementations.
func DefaultPromptConfig() *PromptConfig {
	defer perf.Track(nil, "cmd.DefaultPromptConfig")()

	return &PromptConfig{
		ListComponents: component.ListAllComponents,
		ListStacks:     listStacks,
		PromptArg:      flags.PromptForPositionalArg,
		PromptFlag:     flags.PromptForMissingRequired,
	}
}

// promptForSemanticValues prompts for missing semantic-typed values.
// This enables interactive selection for custom commands with component/stack arguments.
func promptForSemanticValues(
	cmd *cobra.Command,
	commandConfig *schema.Command,
	argumentsData map[string]string,
	flagsData map[string]any,
	promptCfg *PromptConfig,
) {
	defer perf.Track(nil, "cmd.promptForSemanticValues")()

	if commandConfig.Component == nil {
		return
	}
	if promptCfg == nil {
		promptCfg = DefaultPromptConfig()
	}

	stacksMap, err := loadStacksMapForCompletion()
	if err != nil {
		return // Graceful degradation.
	}

	promptSemanticArguments(cmd, commandConfig, argumentsData, stacksMap, promptCfg)
	promptSemanticFlags(cmd, commandConfig, flagsData, stacksMap, promptCfg)
}

// promptSemanticArguments prompts for missing semantic-typed arguments.
func promptSemanticArguments(
	cmd *cobra.Command,
	commandConfig *schema.Command,
	argumentsData map[string]string,
	stacksMap map[string]any,
	promptCfg *PromptConfig,
) {
	defer perf.Track(nil, "cmd.promptSemanticArguments")()

	for _, arg := range commandConfig.Arguments {
		if !arg.Required || argumentsData[arg.Name] != "" {
			continue
		}

		var selected string
		var err error

		switch arg.Type {
		case semanticTypeComponent:
			selected, err = promptForComponentValue(cmd, arg.Name, commandConfig.Component.Type, stacksMap, promptCfg, true)
		case semanticTypeStack:
			selected, err = promptForStackValue(cmd, arg.Name, promptCfg, true)
		default:
			continue
		}

		if err == nil && selected != "" {
			argumentsData[arg.Name] = selected
		}
	}
}

// promptSemanticFlags prompts for missing semantic-typed flags.
func promptSemanticFlags(
	cmd *cobra.Command,
	commandConfig *schema.Command,
	flagsData map[string]any,
	stacksMap map[string]any,
	promptCfg *PromptConfig,
) {
	defer perf.Track(nil, "cmd.promptSemanticFlags")()

	for _, flag := range commandConfig.Flags {
		if !flag.Required {
			continue
		}
		if val, ok := flagsData[flag.Name].(string); ok && val != "" {
			continue
		}

		var selected string
		var err error

		switch flag.SemanticType {
		case semanticTypeComponent:
			selected, err = promptForComponentValue(cmd, flag.Name, commandConfig.Component.Type, stacksMap, promptCfg, false)
		case semanticTypeStack:
			selected, err = promptForStackValue(cmd, flag.Name, promptCfg, false)
		default:
			continue
		}

		if err == nil && selected != "" {
			flagsData[flag.Name] = selected
		}
	}
}

// promptForComponentValue prompts for a component value.
// ComponentType and stacksMap are passed as parameters to avoid exceeding function argument limits.
// IsArg indicates whether this is an argument (true) or flag (false) for appropriate prompting.
//
//nolint:revive // argument-limit: parameters are necessary for semantic completion
func promptForComponentValue(
	cmd *cobra.Command,
	name string,
	componentType string,
	stacksMap map[string]any,
	promptCfg *PromptConfig,
	isArg bool,
) (string, error) {
	defer perf.Track(nil, "cmd.promptForComponentValue")()

	components, err := promptCfg.ListComponents(context.Background(), componentType, stacksMap)
	if err != nil || len(components) == 0 {
		return "", nil // Graceful degradation.
	}

	completionFunc := func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return components, cobra.ShellCompDirectiveNoFileComp
	}

	if isArg {
		return promptCfg.PromptArg(name, fmt.Sprintf("Choose %s", name), completionFunc, cmd, nil)
	}
	return promptCfg.PromptFlag(name, fmt.Sprintf("Choose %s", name), completionFunc, cmd, nil)
}

// promptForStackValue prompts for a stack value (arg or flag).
func promptForStackValue(cmd *cobra.Command, name string, promptCfg *PromptConfig, isArg bool) (string, error) {
	defer perf.Track(nil, "cmd.promptForStackValue")()

	stacks, err := promptCfg.ListStacks(cmd)
	if err != nil || len(stacks) == 0 {
		return "", nil // Graceful degradation.
	}

	completionFunc := func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return stacks, cobra.ShellCompDirectiveNoFileComp
	}

	if isArg {
		return promptCfg.PromptArg(name, "Choose stack", completionFunc, cmd, nil)
	}
	return promptCfg.PromptFlag(name, "Choose stack", completionFunc, cmd, nil)
}
