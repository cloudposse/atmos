package exec

import (
	tui "github.com/cloudposse/atmos/internal/tui/atmos"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/samber/lo"
)

// ExecuteAtmosCmd executes `atmos` command
func ExecuteAtmosCmd() error {
	commands := []string{
		"terraform plan",
		"terraform apply",
		"terraform destroy",
		"terraform init",
		"terraform output",
		"terraform clean",
		"terraform workspace",
		"terraform refresh",
		"terraform show",
		"terraform validate",
		"terraform shell",
		"validate component",
		"describe component",
		"describe dependents",
	}

	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	cliConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return err
	}

	// Get a map of stacks and components in the stacks
	stacksMap, err := ExecuteDescribeStacks(cliConfig, "", nil, nil, nil, false)
	if err != nil {
		return err
	}

	// Create a map of stacks to lists of components in each stack
	stacksComponentsMap := lo.MapEntries(stacksMap, func(k string, v any) (string, []string) {
		if v2, ok := v.(map[string]any); ok {
			if v3, ok := v2["components"].(map[string]any); ok {
				// TODO: process 'helmfile' components and stacks.
				// This will require checking the list of commands and filtering the stacks and components depending on the selected command.
				if v4, ok := v3["terraform"].(map[string]any); ok {
					return k, lo.Keys(v4)
				}
			}
		}
		return k, nil
	})

	// Get a set of all components
	componentsSet := lo.Uniq(lo.Flatten(lo.Values(stacksComponentsMap)))

	// Create a map of components to lists of stacks for each component
	componentsStacksMap := make(map[string][]string)
	lo.ForEach(componentsSet, func(c string, _ int) {
		var stacksForComponent []string
		componentsStacksMap[c] = stacksForComponent
	})

	// Sort the maps by the keys, and sort the lists of values
	stacksComponentsMap = u.SortMapByKeysAndValues(stacksComponentsMap)
	componentsStacksMap = u.SortMapByKeysAndValues(componentsStacksMap)

	// Start the UI
	app, err := tui.Execute(commands, stacksComponentsMap, componentsStacksMap)
	if err != nil {
		return err
	}

	// If the user quit the UI, exit
	if app.ExitStatusQuit() {
		return nil
	}

	// Process the selected command, stack and component
	selectedComponent := app.GetSelectedComponent()
	selectedStack := app.GetSelectedStack()

	data, err := ExecuteDescribeComponent(selectedComponent, selectedStack)
	if err != nil {
		return err
	}

	err = u.PrintAsYAML(data)
	if err != nil {
		return err
	}

	return nil
}
