package exec

import (
	"fmt"
	"slices"
	"strings"

	"github.com/samber/lo"

	tui "github.com/cloudposse/atmos/internal/tui/atmos"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ExecuteAtmosCmd executes `atmos` command.
func ExecuteAtmosCmd() error {
	defer perf.Track(nil, "exec.ExecuteAtmosCmd")()

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	if err != nil {
		return err
	}
	return ExecuteAtmosCmdWithConfig(&atmosConfig)
}

// ExecuteAtmosCmdWithConfig opens the TUI using configuration with discovered
// stack manifests, preserving the caller's CLI configuration overrides.
func ExecuteAtmosCmdWithConfig(atmosConfig *schema.AtmosConfiguration) error {
	defer perf.Track(atmosConfig, "exec.ExecuteAtmosCmdWithConfig")()

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

	// Get a map of stacks and components in the stacks
	// Don't process `Go` templates and YAML functions in Atmos stack manifests since we just need to display the stack and component names in the TUI
	stacksMap, err := ExecuteDescribeStacks(atmosConfig, "", nil, nil, nil, false, false, false, false, nil, nil)
	if err != nil {
		return err
	}

	stacksComponentsMap, componentsStacksMap := stackComponentMapsForUI(stacksMap)

	// Start the UI
	app, err := tui.Execute(commands, stacksComponentsMap, componentsStacksMap)
	ui.Writeln("")
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
	return executeAtmosUISelection(atmosConfig, selectedCommand, selectedComponent, selectedStack)
}

func stackComponentMapsForUI(stacksMap map[string]any) (map[string][]string, map[string][]string) {
	// Create a map of stacks to lists of components in each stack.
	stacksComponentsMap := lo.MapEntries(stacksMap, func(k string, v any) (string, []string) {
		return k, terraformComponentsForUI(v)
	})

	// Get a set of all components.
	componentsSet := lo.Uniq(lo.Flatten(lo.Values(stacksComponentsMap)))

	// Create a map of components to lists of stacks for each component.
	componentsStacksMap := make(map[string][]string)
	lo.ForEach(componentsSet, func(c string, _ int) {
		var stacksForComponent []string
		for k, v := range stacksComponentsMap {
			if slices.Contains(v, c) {
				stacksForComponent = append(stacksForComponent, k)
			}
		}
		componentsStacksMap[c] = stacksForComponent
	})

	// Sort the maps by the keys, and sort the lists of values.
	return u.SortMapByKeysAndValuesUniq(stacksComponentsMap), u.SortMapByKeysAndValuesUniq(componentsStacksMap)
}

func terraformComponentsForUI(value any) []string {
	stack, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	components, ok := stack["components"].(map[string]any)
	if !ok {
		return nil
	}
	terraform, ok := components["terraform"].(map[string]any)
	if ok {
		return FilterAbstractComponents(terraform)
	}
	// TODO: process 'helmfile' components and stacks.
	// This will require checking the list of commands and filtering the stacks and components depending on the selected command.
	return nil
}

func executeAtmosUISelection(atmosConfig *schema.AtmosConfiguration, selectedCommand, selectedComponent, selectedStack string) error {
	// Process the selected command, stack and component
	c := fmt.Sprintf("atmos %s %s --stack %s", selectedCommand, selectedComponent, selectedStack)
	log.Info("Executing", "command", c)

	if selectedCommand == "describe component" {
		data, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
			Component:            selectedComponent,
			Stack:                selectedStack,
			ProcessTemplates:     true,
			ProcessYamlFunctions: true,
			Skip:                 nil,
			AuthManager:          nil,
		})
		if err != nil {
			return err
		}
		return u.PrintAsYAML(atmosConfig, data)
	}

	if selectedCommand == "describe dependents" {
		data, err := ExecuteDescribeDependents(atmosConfig, &DescribeDependentsArgs{
			Component:            selectedComponent,
			Stack:                selectedStack,
			IncludeSettings:      false,
			ProcessTemplates:     true,
			ProcessYamlFunctions: true,
			Skip:                 nil,
			OnlyInStack:          "",
		})
		if err != nil {
			return err
		}
		return u.PrintAsYAML(atmosConfig, data)
	}

	if selectedCommand == "validate component" {
		_, err := ExecuteValidateComponent(atmosConfig, schema.ConfigAndStacksInfo{}, selectedComponent, selectedStack, "", "", nil, 0)
		if err != nil {
			return err
		}

		log.Info("Validated successfully", "component", selectedComponent, "stack", selectedStack)
		return nil
	}

	// All Terraform commands.
	if strings.HasPrefix(selectedCommand, "terraform") {
		return executeTerraformUISelection(atmosConfig, selectedCommand, selectedComponent, selectedStack)
	}

	return nil
}

func executeTerraformUISelection(atmosConfig *schema.AtmosConfiguration, command, component, stack string) error {
	parts := strings.Split(command, " ")
	subcommand := parts[1]

	// "terraform shell" is an Atmos-only command (not a native terraform subcommand).
	// Route it directly to ExecuteTerraformShell to avoid ExecuteTerraform passing
	// it to the terraform executable.
	if subcommand == "shell" {
		return ExecuteTerraformShell(shellOptionsForUI(component, stack), atmosConfig)
	}

	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	configAndStacksInfo.ComponentType = "terraform"
	configAndStacksInfo.Component = component
	configAndStacksInfo.ComponentFromArg = component
	configAndStacksInfo.Stack = stack
	configAndStacksInfo.SubCommand = subcommand
	configAndStacksInfo.ProcessTemplates = true
	configAndStacksInfo.ProcessFunctions = true
	return ExecuteTerraform(configAndStacksInfo)
}

// shellOptionsForUI builds ShellOptions for the interactive UI dispatch path.
// The UI doesn't support DryRun or Identity selection, so those default to zero values.
func shellOptionsForUI(component, stack string) *ShellOptions {
	return &ShellOptions{
		Component:         component,
		Stack:             stack,
		ProcessingOptions: ProcessingOptions{ProcessTemplates: true, ProcessFunctions: true},
	}
}
