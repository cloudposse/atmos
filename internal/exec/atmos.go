package exec

import (
	"fmt"
	"strings"

	log "github.com/charmbracelet/log"
	"github.com/samber/lo"

	tui "github.com/cloudposse/atmos/internal/tui/atmos"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
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
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return err
	}

	// Get a map of stacks and components in the stacks
	// Don't process `Go` templates and YAML functions in Atmos stack manifests since we just need to display the stack and component names in the TUI
	stacksMap, err := ExecuteDescribeStacks(atmosConfig, "", nil, nil, nil, false, false, false, false, nil)
	if err != nil {
		return err
	}

	// Create a map of stacks to lists of components in each stack
	stacksComponentsMap := lo.MapEntries(stacksMap, func(k string, v any) (string, []string) {
		if v2, ok := v.(map[string]any); ok {
			if v3, ok := v2["components"].(map[string]any); ok {
				if v4, ok := v3["terraform"].(map[string]any); ok {
					return k, FilterAbstractComponents(v4)
				}
				// TODO: process 'helmfile' components and stacks.
				// This will require checking the list of commands and filtering the stacks and components depending on the selected command.
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
		for k, v := range stacksComponentsMap {
			if u.SliceContainsString(v, c) {
				stacksForComponent = append(stacksForComponent, k)
			}
		}
		componentsStacksMap[c] = stacksForComponent
	})

	// Sort the maps by the keys, and sort the lists of values
	stacksComponentsMap = u.SortMapByKeysAndValuesUniq(stacksComponentsMap)
	componentsStacksMap = u.SortMapByKeysAndValuesUniq(componentsStacksMap)

	// Start the UI
	app, err := tui.Execute(commands, stacksComponentsMap, componentsStacksMap)
	fmt.Println()
	if err != nil {
		return err
	}

	selectedCommand := app.GetSelectedCommand()
	selectedComponent := app.GetSelectedComponent()
	selectedStack := app.GetSelectedStack()

	// If the user quit the UI, exit
	if app.ExitStatusQuit() || selectedCommand == "" || selectedComponent == "" || selectedStack == "" {
		return nil
	}

	// Process the selected command, stack and component
	c := fmt.Sprintf("atmos %s %s --stack %s", selectedCommand, selectedComponent, selectedStack)
	log.Info("Executing", "command", c)

	if selectedCommand == "describe component" {
		data, err := ExecuteDescribeComponent(selectedComponent, selectedStack, true, true, nil)
		if err != nil {
			return err
		}
		err = u.PrintAsYAML(&atmosConfig, data)
		if err != nil {
			return err
		}
		return nil
	}

	if selectedCommand == "describe dependents" {
		data, err := ExecuteDescribeDependents(atmosConfig, selectedComponent, selectedStack, false)
		if err != nil {
			return err
		}
		err = u.PrintAsYAML(&atmosConfig, data)
		if err != nil {
			return err
		}
		return nil
	}

	if selectedCommand == "validate component" {
		_, err = ExecuteValidateComponent(atmosConfig, schema.ConfigAndStacksInfo{}, selectedComponent, selectedStack, "", "", nil, 0)
		if err != nil {
			return err
		}

		log.Info("Validated successfully", "component", selectedComponent, "stack", selectedStack)
		return nil
	}

	// All Terraform commands
	if strings.HasPrefix(selectedCommand, "terraform") {
		parts := strings.Split(selectedCommand, " ")
		subcommand := parts[1]
		configAndStacksInfo.ComponentType = "terraform"
		configAndStacksInfo.Component = selectedComponent
		configAndStacksInfo.ComponentFromArg = selectedComponent
		configAndStacksInfo.Stack = selectedStack
		configAndStacksInfo.SubCommand = subcommand
		configAndStacksInfo.ProcessTemplates = true
		configAndStacksInfo.ProcessFunctions = true
		err = ExecuteTerraform(configAndStacksInfo)
		if err != nil {
			return err
		}
	}

	return nil
}
