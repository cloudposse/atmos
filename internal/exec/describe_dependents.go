package exec

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/pager"
	u "github.com/cloudposse/atmos/pkg/utils"

	"github.com/mitchellh/mapstructure"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

type DescribeDependentsExecProps struct {
	File      string
	Format    string
	Query     string
	Stack     string
	Component string
}

//go:generate mockgen -source=$GOFILE -destination=mock_$GOFILE -package=$GOPACKAGE
type DescribeDependentsExec interface {
	Execute(describeDependentsExecProps *DescribeDependentsExecProps) error
}

type describeDependentsExec struct {
	atmosConfig               *schema.AtmosConfiguration
	executeDescribeDependents func(
		atmosConfig schema.AtmosConfiguration,
		component string,
		stack string,
		includeSettings bool,
	) ([]schema.Dependent, error)
	newPageCreator        pager.PageCreator
	isTTYSupportForStdout func() bool
	evaluateYqExpression  func(
		atmosConfig *schema.AtmosConfiguration,
		data any,
		yq string,
	) (any, error)
}

func NewDescribeDependentsExec(atmosConfig *schema.AtmosConfiguration) DescribeDependentsExec {
	return &describeDependentsExec{
		executeDescribeDependents: ExecuteDescribeDependents,
		newPageCreator:            pager.New(),
		isTTYSupportForStdout:     term.IsTTYSupportForStdout,
		atmosConfig:               atmosConfig,
		evaluateYqExpression:      u.EvaluateYqExpression,
	}
}

func (d *describeDependentsExec) Execute(describeDependentsExecProps *DescribeDependentsExecProps) error {
	dependents, err := d.executeDescribeDependents(
		*d.atmosConfig,
		describeDependentsExecProps.Component,
		describeDependentsExecProps.Stack,
		false,
	)
	if err != nil {
		return err
	}

	var res any

	if describeDependentsExecProps.Query != "" {
		res, err = d.evaluateYqExpression(d.atmosConfig, dependents, describeDependentsExecProps.Query)
		if err != nil {
			return err
		}
	} else {
		res = dependents
	}

	return viewWithScroll(&viewWithScrollProps{
		atmosConfig:           d.atmosConfig,
		format:                describeDependentsExecProps.Format,
		file:                  describeDependentsExecProps.File,
		res:                   res,
		pageCreator:           d.newPageCreator,
		isTTYSupportForStdout: d.isTTYSupportForStdout,
		displayName:           fmt.Sprintf("Dependents of '%s' in stack '%s'", describeDependentsExecProps.Component, describeDependentsExecProps.Stack),
		printOrWriteToFile:    printOrWriteToFile,
	})
}

// ExecuteDescribeDependents produces a list of Atmos components in Atmos stacks that depend on the provided Atmos component
func ExecuteDescribeDependents(
	atmosConfig schema.AtmosConfiguration,
	component string,
	stack string,
	includeSettings bool,
) ([]schema.Dependent, error) {
	dependents := []schema.Dependent{}
	var ok bool

	// Get all stacks with all components
	stacks, err := ExecuteDescribeStacks(atmosConfig, "", nil, nil, nil, false, true, true, false, nil)
	if err != nil {
		return nil, err
	}

	providedComponentSection, err := ExecuteDescribeComponent(component, stack, true, true, nil)
	if err != nil {
		return nil, err
	}

	// Get the provided component `vars`
	var providedComponentVarsSection map[string]any
	if providedComponentVarsSection, ok = providedComponentSection["vars"].(map[string]any); !ok {
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
				if metadataSection, ok := stackComponentMap["metadata"].(map[string]any); ok {
					if metadataType, ok := metadataSection["type"].(string); ok {
						if metadataType == "abstract" {
							continue
						}
					}
				}

				// Get the stack component `vars`
				var stackComponentVarsSection map[string]any
				if stackComponentVarsSection, ok = stackComponentMap["vars"].(map[string]any); !ok {
					return dependents, nil
				}

				// Convert the stack component `vars` section to the `Context` structure
				var stackComponentVars schema.Context
				err = mapstructure.Decode(stackComponentVarsSection, &stackComponentVars)
				if err != nil {
					return nil, err
				}

				// Get the stack component `settings`
				var stackComponentSettingsSection map[string]any
				if stackComponentSettingsSection, ok = stackComponentMap["settings"].(map[string]any); !ok {
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
						ComponentPath: BuildComponentPath(atmosConfig, stackComponentMap, stackComponentType),
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

						spaceliftStackName, err := BuildSpaceliftStackNameFromComponentConfig(atmosConfig, configAndStacksInfo)
						if err != nil {
							return nil, err
						}
						dependent.SpaceliftStack = spaceliftStackName

						// Atlantis project
						atlantisProjectName, err := BuildAtlantisProjectNameFromComponentConfig(atmosConfig, configAndStacksInfo)
						if err != nil {
							return nil, err
						}
						dependent.AtlantisProject = atlantisProjectName
					}

					if includeSettings {
						dependent.Settings = stackComponentSettingsSection
					}

					dependents = append(dependents, dependent)
				}
			}
		}
	}

	return dependents, nil
}
