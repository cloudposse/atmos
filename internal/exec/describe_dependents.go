package exec

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ExecuteDescribeDependentsCmd executes `describe dependents` command
func ExecuteDescribeDependentsCmd(cmd *cobra.Command, args []string) error {
	info, err := processCommandLineArgs("", cmd, args, nil)
	if err != nil {
		return err
	}

	cliConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		return err
	}

	err = ValidateStacks(cliConfig)
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

	dependents, err := ExecuteDescribeDependents(cliConfig, component, stack)
	if err != nil {
		return err
	}

	err = printOrWriteToFile(format, file, dependents)
	if err != nil {
		return err
	}

	return nil
}

// ExecuteDescribeDependents produces a list of Atmos components in Atmos stacks that depend on the provided Atmos component
func ExecuteDescribeDependents(
	cliConfig schema.CliConfiguration,
	component string,
	stack string,
) ([]schema.Dependent, error) {

	dependents := []schema.Dependent{}
	var ok bool

	// Get all stacks with all components
	stacks, err := ExecuteDescribeStacks(cliConfig, "", nil, nil, nil, false)
	if err != nil {
		return nil, err
	}

	providedComponentSection, err := ExecuteDescribeComponent(component, stack)
	if err != nil {
		return nil, err
	}

	// Get the provided component `vars`
	var providedComponentVarsSection map[any]any
	if providedComponentVarsSection, ok = providedComponentSection["vars"].(map[any]any); !ok {
		return dependents, nil
	}

	// Convert the provided component `vars` section to the `Context` structure
	var providedComponentVars schema.Context
	err = mapstructure.Decode(providedComponentVarsSection, &providedComponentVars)
	if err != nil {
		return nil, err
	}

	// Iterate over all stacks and all components in the stacks
	for stackName, stackSection := range stacks {
		var stackSectionMap map[string]any
		if stackSectionMap, ok = stackSection.(map[string]any); !ok {
			continue
		}

		// Get the stack `components` section
		var stackComponentsSection map[string]any
		if stackComponentsSection, ok = stackSectionMap["components"].(map[string]any); !ok {
			continue
		}

		for stackComponentType, stackComponentTypeSection := range stackComponentsSection {
			var stackComponentTypeSectionMap map[string]any
			if stackComponentTypeSectionMap, ok = stackComponentTypeSection.(map[string]any); !ok {
				continue
			}

			for stackComponentName, stackComponent := range stackComponentTypeSectionMap {
				var stackComponentMap map[string]any
				if stackComponentMap, ok = stackComponent.(map[string]any); !ok {
					continue
				}

				// Skip the stack component if it's the same as the provided component
				if stackComponentName == component {
					continue
				}

				// Skip abstract components
				if metadataSection, ok := stackComponentMap["metadata"].(map[any]any); ok {
					if metadataType, ok := metadataSection["type"].(string); ok {
						if metadataType == "abstract" {
							continue
						}
					}
				}

				// Get the stack component `vars`
				var stackComponentVarsSection map[any]any
				if stackComponentVarsSection, ok = stackComponentMap["vars"].(map[any]any); !ok {
					return dependents, nil
				}

				// Convert the stack component `vars` section to the `Context` structure
				var stackComponentVars schema.Context
				err = mapstructure.Decode(stackComponentVarsSection, &stackComponentVars)
				if err != nil {
					return nil, err
				}

				// Get the stack component `settings`
				var stackComponentSettingsSection map[any]any
				if stackComponentSettingsSection, ok = stackComponentMap["settings"].(map[any]any); !ok {
					continue
				}

				// Convert the `settings` section to the `Settings` structure
				var stackComponentSettings schema.Settings
				err = mapstructure.Decode(stackComponentSettingsSection, &stackComponentSettings)
				if err != nil {
					return nil, err
				}

				// Skip if the stack component has an empty `settings.depends_on` section
				if reflect.ValueOf(stackComponentSettings).IsZero() ||
					reflect.ValueOf(stackComponentSettings.DependsOn).IsZero() {
					continue
				}

				// Check if the stack component is a dependent of the provided component
				for _, dependsOn := range stackComponentSettings.DependsOn {
					if dependsOn.Component != component {
						continue
					}

					// Include the component from the stack if any of the following is true:
					// - `namespace` is specified in `depends_on` and the provided component's namespace is equal to the namespace in `depends_on`
					// - `namespace` is not specified in `depends_on` and the provided component is from the same namespace as the component in `depends_on`
					if dependsOn.Namespace != "" {
						if providedComponentVars.Namespace != dependsOn.Namespace {
							continue
						}
					} else if providedComponentVars.Namespace != stackComponentVars.Namespace {
						continue
					}

					// Include the component from the stack if any of the following is true:
					// - `tenant` is specified in `depends_on` and the provided component's tenant is equal to the tenant in `depends_on`
					// - `tenant` is not specified in `depends_on` and the provided component is from the same tenant as the component in `depends_on`
					if dependsOn.Tenant != "" {
						if providedComponentVars.Tenant != dependsOn.Tenant {
							continue
						}
					} else if providedComponentVars.Tenant != stackComponentVars.Tenant {
						continue
					}

					// Include the component from the stack if any of the following is true:
					// - `environment` is specified in `depends_on` and the component's environment is equal to the environment in `depends_on`
					// - `environment` is not specified in `depends_on` and the provided component is from the same environment as the component in `depends_on`
					if dependsOn.Environment != "" {
						if providedComponentVars.Environment != dependsOn.Environment {
							continue
						}
					} else if providedComponentVars.Environment != stackComponentVars.Environment {
						continue
					}

					// Include the component from the stack if any of the following is true:
					// - `stage` is specified in `depends_on` and the provided component's stage is equal to the stage in `depends_on`
					// - `stage` is not specified in `depends_on` and the provided component is from the same stage as the component in `depends_on`
					if dependsOn.Stage != "" {
						if providedComponentVars.Stage != dependsOn.Stage {
							continue
						}
					} else if providedComponentVars.Stage != stackComponentVars.Stage {
						continue
					}

					dependent := schema.Dependent{
						Component:     stackComponentName,
						ComponentPath: BuildComponentPath(cliConfig, stackComponentMap, stackComponentType),
						ComponentType: stackComponentType,
						Stack:         stackName,
						StackSlug:     fmt.Sprintf("%s-%s", stackName, strings.Replace(stackComponentName, "/", "-", -1)),
						Namespace:     stackComponentVars.Namespace,
						Tenant:        stackComponentVars.Tenant,
						Environment:   stackComponentVars.Environment,
						Stage:         stackComponentVars.Stage,
					}

					// Add Spacelift stack and Atlantis project if they are configured for the dependent stack component
					if stackComponentType == "terraform" {

						// Spacelift stack
						configAndStacksInfo := schema.ConfigAndStacksInfo{
							ComponentFromArg:         stackComponentName,
							Stack:                    stackName,
							ComponentVarsSection:     stackComponentVarsSection,
							ComponentSettingsSection: stackComponentSettingsSection,
							ComponentSection: map[string]any{
								cfg.VarsSectionName:     stackComponentVarsSection,
								cfg.SettingsSectionName: stackComponentSettingsSection,
							},
						}

						spaceliftStackName, err := BuildSpaceliftStackNameFromComponentConfig(cliConfig, configAndStacksInfo)
						if err != nil {
							return nil, err
						}
						dependent.SpaceliftStack = spaceliftStackName

						// Atlantis project
						atlantisProjectName, err := BuildAtlantisProjectNameFromComponentConfig(cliConfig, configAndStacksInfo)
						if err != nil {
							return nil, err
						}
						dependent.AtlantisProject = atlantisProjectName
					}

					dependents = append(dependents, dependent)
				}
			}
		}
	}

	return dependents, nil
}
