package composition

import (
	"sort"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Seams keep stack completion credential-free and independently testable.
var (
	initCliConfigForCompletion  = cfg.InitCliConfig
	describeStacksForCompletion = func(
		atmosConfig *schema.AtmosConfiguration,
		filterByStack string,
		components, componentTypes, sections []string,
		ignoreMissingFiles, processTemplates, processYamlFunctions, includeEmptyStacks bool,
		skip []string,
		authManager auth.AuthManager,
	) (map[string]any, error) {
		return e.ExecuteDescribeStacksWithAuthDisabled(
			atmosConfig, filterByStack, components, componentTypes, sections,
			ignoreMissingFiles, processTemplates, processYamlFunctions, includeEmptyStacks,
			skip, authManager, true,
		)
	}
)

// globalInfoForCompletion preserves config-selection flags while populating a
// stack picker. Authentication is intentionally omitted from this read-only path.
func globalInfoForCompletion(cmd *cobra.Command) schema.ConfigAndStacksInfo {
	globalFlags := flags.ParseGlobalFlags(cmd, viper.GetViper())
	return schema.ConfigAndStacksInfo{
		AtmosBasePath:           globalFlags.BasePath,
		AtmosConfigFilesFromArg: globalFlags.Config,
		AtmosConfigDirsFromArg:  globalFlags.ConfigPath,
		ProfilesFromArg:         globalFlags.Profile,
	}
}

// compositionStackFlagCompletion offers only stacks that have composition
// members. A supplied composition name narrows the choices to its members.
func compositionStackFlagCompletion(cmd *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	atmosConfig, err := initCliConfigForCompletion(globalInfoForCompletion(cmd), true)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	stacksMap, err := describeStacksForCompletion(
		&atmosConfig, "", nil, nil, nil,
		false, false, false, false, nil, nil,
	)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return compositionStackNames(stacksMap, optionalCompositionArg(args)), cobra.ShellCompDirectiveNoFileComp
}

func compositionStackNames(stacksMap map[string]any, compositionName string) []string {
	stacks := make([]string, 0, len(stacksMap))
	for stackName, stackData := range stacksMap {
		if stackHasComposition(stackData, compositionName) {
			stacks = append(stacks, stackName)
		}
	}
	sort.Strings(stacks)
	return stacks
}

func stackHasComposition(stackData any, compositionName string) bool {
	stackMap, ok := stackData.(map[string]any)
	if !ok {
		return false
	}
	components, ok := stackMap[cfg.ComponentsSectionName].(map[string]any)
	if !ok {
		return false
	}
	for _, typeSection := range components {
		typeMap, ok := typeSection.(map[string]any)
		if !ok {
			continue
		}
		for _, componentData := range typeMap {
			componentMap, ok := componentData.(map[string]any)
			if !ok {
				continue
			}
			memberOf, _ := componentMap["composition"].(string)
			if memberOf != "" && (compositionName == "" || memberOf == compositionName) {
				return true
			}
		}
	}
	return false
}
