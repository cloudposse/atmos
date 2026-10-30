package backend

import (
	"maps"
	"slices"

	"github.com/spf13/cobra"
)

func inspectionStackCompletion(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	if atmosConfigPtr == nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	stacks, err := inspectBackendStacks(atmosConfigPtr, "")
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	if len(args) == 0 {
		return slices.Sorted(maps.Keys(stacks)), cobra.ShellCompDirectiveNoFileComp
	}
	var names []string
	for stack, section := range stacks {
		if slices.Contains(inspectionComponentNames(section), args[0]) {
			names = append(names, stack)
		}
	}
	slices.Sort(names)
	return names, cobra.ShellCompDirectiveNoFileComp
}

func inspectionComponentCompletion(cmd *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 || atmosConfigPtr == nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	stacks, err := inspectBackendStacks(atmosConfigPtr, getCommandFlagStack(cmd))
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	names := make(map[string]bool)
	for _, section := range stacks {
		for _, name := range inspectionComponentNames(section) {
			names[name] = true
		}
	}
	return slices.Sorted(maps.Keys(names)), cobra.ShellCompDirectiveNoFileComp
}

func inspectionComponentNames(raw any) []string {
	section, _ := raw.(map[string]any)
	components, _ := section["components"].(map[string]any)
	terraform, _ := components["terraform"].(map[string]any)
	var names []string
	for name, rawComponent := range terraform {
		component, _ := rawComponent.(map[string]any)
		metadata, _ := component["metadata"].(map[string]any)
		if metadata["type"] != "abstract" {
			names = append(names, name)
		}
	}
	return names
}
