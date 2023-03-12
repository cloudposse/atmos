package exec

import (
	"fmt"
	"reflect"

	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	cfg "github.com/cloudposse/atmos/pkg/config"
)

// ExecuteDescribeDependantsCmd executes `describe dependants` command
func ExecuteDescribeDependantsCmd(cmd *cobra.Command, args []string) error {
	info, err := processCommandLineArgs("", cmd, args)
	if err != nil {
		return err
	}

	cliConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		return err
	}

	if len(args) != 1 {
		return errors.New("invalid arguments. The command requires one argument `component`")
	}

	flags := cmd.Flags()

	stack, err := flags.GetString("stack")
	if err != nil {
		return err
	}

	format, err := flags.GetString("format")
	if err != nil {
		return err
	}

	file, err := flags.GetString("file")
	if err != nil {
		return err
	}

	component := args[0]

	dependants, err := ExecuteDescribeDependants(cliConfig, component, stack)
	if err != nil {
		return err
	}

	fmt.Println()
	err = printOrWriteToFile(format, file, dependants)
	if err != nil {
		return err
	}

	return nil
}

// ExecuteDescribeDependants produces a list of Atmos components in Atmos stacks that depend on the provided Atmos component
func ExecuteDescribeDependants(
	cliConfig cfg.CliConfiguration,
	component string,
	stack string,
) ([]cfg.Dependant, error) {

	dependants := []cfg.Dependant{}
	var currentComponentVarsSection map[any]any
	var currentComponentVars cfg.Context
	var stackComponentsSection map[string]any
	var stackComponentTypeSectionMap map[string]any
	var stackComponentSettingsSection map[any]any
	var stackComponentSettings cfg.Settings
	var stackSectionMap map[string]any
	var stackComponentMap map[string]any
	var ok bool

	// Get all stacks with all components
	stacks, err := ExecuteDescribeStacks(cliConfig, "", nil, nil, nil, false)
	if err != nil {
		return nil, err
	}

	currentComponentSection, err := ExecuteDescribeComponent(component, stack)
	if err != nil {
		return nil, err
	}

	// Get the current component `vars`
	if currentComponentVarsSection, ok = currentComponentSection["vars"].(map[any]any); !ok {
		return dependants, nil
	}

	// Convert the current component `vars` section to the `Context` structure
	err = mapstructure.Decode(currentComponentVarsSection, &currentComponentVars)
	if err != nil {
		return nil, err
	}

	// Iterate over all stacks and all components in the stacks
	for stackName, stackSection := range stacks {
		if stackSectionMap, ok = stackSection.(map[string]any); !ok {
			continue
		}

		// Get the stack `components` section
		if stackComponentsSection, ok = stackSectionMap["components"].(map[string]any); !ok {
			continue
		}

		for stackComponentType, stackComponentTypeSection := range stackComponentsSection {
			if stackComponentTypeSectionMap, ok = stackComponentTypeSection.(map[string]any); !ok {
				continue
			}

			for stackComponentName, stackComponent := range stackComponentTypeSectionMap {
				// Skip the current component
				if stackComponentName == component {
					continue
				}

				if stackComponentMap, ok = stackComponent.(map[string]any); !ok {
					continue
				}

				// Get the stack component `settings`
				if stackComponentSettingsSection, ok = stackComponentMap["settings"].(map[any]any); !ok {
					continue
				}

				// Convert the `settings` section to the `Settings` structure
				err = mapstructure.Decode(stackComponentSettingsSection, &stackComponentSettings)
				if err != nil {
					return nil, err
				}

				// Skip if the stack component has an empty `settings.dependencies.depends_on` section
				if reflect.ValueOf(stackComponentSettings).IsZero() ||
					reflect.ValueOf(stackComponentSettings.Dependencies).IsZero() ||
					reflect.ValueOf(stackComponentSettings.Dependencies.DependsOn).IsZero() {
					continue
				}

				// Check if the stack component is a dependant of the current component
				for _, context := range stackComponentSettings.Dependencies.DependsOn {
					if context.Component == stackComponentName {
						dependant := cfg.Dependant{
							Component:     stackComponentName,
							ComponentType: stackComponentType,
							Stack:         stackName,
							Namespace:     currentComponentVars.Namespace,
							Tenant:        currentComponentVars.Tenant,
							Environment:   currentComponentVars.Environment,
							Stage:         currentComponentVars.Stage,
						}
						dependants = append(dependants, dependant)
					}
				}
			}
		}
	}

	return dependants, nil
}
