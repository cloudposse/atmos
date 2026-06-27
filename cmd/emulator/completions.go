package emulator

import (
	"sort"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	e "github.com/cloudposse/atmos/internal/exec"
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
	defer perf.Track(nil, "emulator.stackFlagCompletion")()

	atmosConfig, err := cfg.InitCliConfig(globalInfoForCompletion(cmd), true)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	stacks, err := stackNamesForCompletion(&atmosConfig)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return stacks, cobra.ShellCompDirectiveNoFileComp
}

func stackNamesForCompletion(atmosConfig *schema.AtmosConfiguration) ([]string, error) {
	stacksMap, _, err := e.FindStacksMap(atmosConfig, false)
	if err != nil {
		return nil, err
	}

	names := make(map[string]struct{}, len(stacksMap))
	for stackFileName, stackSection := range stacksMap {
		stackMap, ok := stackSection.(map[string]any)
		if !ok {
			continue
		}
		name, ok := stackNameForCompletion(atmosConfig, stackFileName, stackMap)
		if ok {
			names[name] = struct{}{}
		}
	}

	stacks := make([]string, 0, len(names))
	for name := range names {
		stacks = append(stacks, name)
	}
	sort.Strings(stacks)
	return stacks, nil
}

func stackNameForCompletion(atmosConfig *schema.AtmosConfiguration, stackFileName string, stackMap map[string]any) (string, bool) {
	if name, ok := stackMap["name"].(string); ok && name != "" {
		return name, true
	}

	if atmosConfig.Stacks.NameTemplate != "" {
		name, err := e.ProcessTmpl(atmosConfig, "emulator-stack-completion-name-template", atmosConfig.Stacks.NameTemplate, stackMap, atmosConfig.Templates.Settings.IgnoreMissingTemplateValues)
		if err == nil && name != "" {
			return name, true
		}
		return stackNameFromComponentTemplates(atmosConfig, stackMap)
	}

	if pattern := e.GetStackNamePattern(atmosConfig); pattern != "" {
		vars, _ := stackMap[cfg.VarsSectionName].(map[string]any)
		context := cfg.GetContextFromVars(vars)
		name, err := cfg.GetContextPrefix(stackFileName, context, pattern, stackFileName)
		if err == nil && name != "" {
			return name, true
		}
	}

	return stackFileName, stackFileName != ""
}

func stackNameFromComponentTemplates(atmosConfig *schema.AtmosConfiguration, stackMap map[string]any) (string, bool) {
	componentsSection, ok := stackMap[cfg.ComponentsSectionName].(map[string]any)
	if !ok {
		return "", false
	}
	for _, componentsByType := range componentsSection {
		componentMap, ok := componentsByType.(map[string]any)
		if !ok {
			continue
		}
		for _, componentSection := range componentMap {
			componentData, ok := componentSection.(map[string]any)
			if !ok {
				continue
			}
			name, err := e.ProcessTmpl(atmosConfig, "emulator-stack-completion-component-name-template", atmosConfig.Stacks.NameTemplate, componentData, atmosConfig.Templates.Settings.IgnoreMissingTemplateValues)
			if err == nil && name != "" {
				return name, true
			}
		}
	}
	return "", false
}

// componentArgCompletion provides completion values for the component argument,
// restricted to emulator components in the selected stack.
func componentArgCompletion(cmd *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	defer perf.Track(nil, "emulator.componentArgCompletion")()

	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	stack := ""
	if stackFlag := cmd.Flag("stack"); stackFlag != nil {
		stack = stackFlag.Value.String()
	}
	atmosConfig, err := cfg.InitCliConfig(globalInfoForCompletion(cmd), true)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	stacksMap, err := e.ExecuteDescribeStacksWithAuthDisabled(&atmosConfig, stack, nil, []string{cfg.EmulatorComponentType}, nil, false, false, false, false, nil, nil, true)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	components, err := l.FilterAndListComponents(stack, stacksMap)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return components, cobra.ShellCompDirectiveNoFileComp
}

// RegisterEmulatorCompletions registers completion functions for the emulator
// command. Every subcommand takes a component argument.
func RegisterEmulatorCompletions(cmd *cobra.Command) {
	defer perf.Track(nil, "emulator.RegisterEmulatorCompletions")()

	for _, subCmd := range cmd.Commands() {
		subCmd.ValidArgsFunction = componentArgCompletion
	}
}
