package exec

import (
	"errors"
	"fmt"
	"strings"

	"github.com/mitchellh/mapstructure"
	"github.com/spf13/cobra"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ExecuteDescribeStacksCmd executes `describe stacks` command
func ExecuteDescribeStacksCmd(cmd *cobra.Command, args []string) error {
	info, err := ProcessCommandLineArgs("", cmd, args, nil)
	if err != nil {
		return err
	}

	atmosConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		return err
	}

	err = ValidateStacks(atmosConfig)
	if err != nil {
		return err
	}

	flags := cmd.Flags()

	filterByStack, err := flags.GetString("stack")
	if err != nil {
		return err
	}

	format, err := flags.GetString("format")
	if err != nil {
		return err
	}

	if format != "" && format != "yaml" && format != "json" {
		return fmt.Errorf("invalid '--format' flag '%s'. Valid values are 'yaml' (default) and 'json'", format)
	}

	if format == "" {
		format = "yaml"
	}

	file, err := flags.GetString("file")
	if err != nil {
		return err
	}

	includeEmptyStacks, err := flags.GetBool("include-empty-stacks")
	if err != nil {
		return err
	}

	componentsCsv, err := flags.GetString("components")
	if err != nil {
		return err
	}

	var components []string
	if componentsCsv != "" {
		components = strings.Split(componentsCsv, ",")
	}

	componentTypesCsv, err := flags.GetString("component-types")
	if err != nil {
		return err
	}

	var componentTypes []string
	if componentTypesCsv != "" {
		componentTypes = strings.Split(componentTypesCsv, ",")
	}

	sectionsCsv, err := flags.GetString("sections")
	if err != nil {
		return err
	}
	var sections []string
	if sectionsCsv != "" {
		sections = strings.Split(sectionsCsv, ",")
	}

	processTemplates, err := flags.GetBool("process-templates")
	if err != nil {
		return err
	}

	processYamlFunctions, err := flags.GetBool("process-functions")
	if err != nil {
		return err
	}

	query, err := flags.GetString("query")
	if err != nil {
		return err
	}

	skip, err := flags.GetStringSlice("skip")
	if err != nil {
		return err
	}

	finalStacksMap, err := ExecuteDescribeStacks(
		atmosConfig,
		filterByStack,
		components,
		componentTypes,
		sections,
		false,
		processTemplates,
		processYamlFunctions,
		includeEmptyStacks,
		skip,
	)
	if err != nil {
		return err
	}

	var res any

	if query != "" {
		res, err = u.EvaluateYqExpression(&atmosConfig, finalStacksMap, query)
		if err != nil {
			return err
		}
	} else {
		res = finalStacksMap
	}

	err = printOrWriteToFile(format, file, res)
	if err != nil {
		return err
	}

	return nil
}

// ExecuteDescribeStacks processes stack manifests and returns the final map of stacks and components
func ExecuteDescribeStacks(
	atmosConfig schema.AtmosConfiguration,
	filterByStack string,
	components []string,
	componentTypes []string,
	sections []string,
	ignoreMissingFiles bool,
	processTemplates bool,
	processYamlFunctions bool,
	includeEmptyStacks bool,
	skip []string,
) (map[string]any, error) {
	stacksMap, _, err := FindStacksMap(atmosConfig, ignoreMissingFiles)
	if err != nil {
		return nil, err
	}

	finalStacksMap := make(map[string]any)
	processedStacks := make(map[string]bool)
	var varsSection map[string]any
	var metadataSection map[string]any
	var settingsSection map[string]any
	var envSection map[string]any
	var providersSection map[string]any
	var hooksSection map[string]any
	var overridesSection map[string]any
	var backendSection map[string]any
	var backendTypeSection string
	var stackName string

	for stackFileName, stackSection := range stacksMap {
		var context schema.Context

		// Delete the stack-wide imports
		delete(stackSection.(map[string]any), "imports")

		// Check if components section exists and has explicit components
		hasExplicitComponents := false
		if componentsSection, ok := stackSection.(map[string]any)["components"]; ok {
			if componentsSection != nil {
				if terraformSection, ok := componentsSection.(map[string]any)["terraform"].(map[string]any); ok {
					hasExplicitComponents = len(terraformSection) > 0
				}
				if helmfileSection, ok := componentsSection.(map[string]any)["helmfile"].(map[string]any); ok {
					hasExplicitComponents = hasExplicitComponents || len(helmfileSection) > 0
				}
			}
		}

		// Also check for imports
		hasImports := false
		if importsSection, ok := stackSection.(map[string]any)["import"].([]any); ok {
			hasImports = len(importsSection) > 0
		}

		// Skip stacks without components or imports when includeEmptyStacks is false
		if !includeEmptyStacks && !hasExplicitComponents && !hasImports {
			continue
		}

		stackName = stackFileName
		if processedStacks[stackName] {
			continue
		}
		processedStacks[stackName] = true

		if !u.MapKeyExists(finalStacksMap, stackName) {
			finalStacksMap[stackName] = make(map[string]any)
			finalStacksMap[stackName].(map[string]any)["components"] = make(map[string]any)
			finalStacksMap[stackName].(map[string]any)["atmos_stack_file"] = stackFileName
		}

		if componentsSection, ok := stackSection.(map[string]any)["components"].(map[string]any); ok {

			if len(componentTypes) == 0 || u.SliceContainsString(componentTypes, "terraform") {
				if terraformSection, ok := componentsSection["terraform"].(map[string]any); ok {
					for componentName, compSection := range terraformSection {
						componentSection, ok := compSection.(map[string]any)
						if !ok {
							return nil, fmt.Errorf("invalid 'components.terraform.%s' section in the file '%s'", componentName, stackFileName)
						}

						if comp, ok := componentSection[cfg.ComponentSectionName].(string); !ok || comp == "" {
							componentSection[cfg.ComponentSectionName] = componentName
						}

						// Find all derived components of the provided components and include them in the output
						derivedComponents, err := FindComponentsDerivedFromBaseComponents(stackFileName, terraformSection, components)
						if err != nil {
							return nil, err
						}

						if varsSection, ok = componentSection[cfg.VarsSectionName].(map[string]any); !ok {
							varsSection = map[string]any{}
						}

						if metadataSection, ok = componentSection[cfg.MetadataSectionName].(map[string]any); !ok {
							metadataSection = map[string]any{}
						}

						if settingsSection, ok = componentSection[cfg.SettingsSectionName].(map[string]any); !ok {
							settingsSection = map[string]any{}
						}

						if envSection, ok = componentSection[cfg.EnvSectionName].(map[string]any); !ok {
							envSection = map[string]any{}
						}

						if providersSection, ok = componentSection[cfg.ProvidersSectionName].(map[string]any); !ok {
							providersSection = map[string]any{}
						}

						if hooksSection, ok = componentSection[cfg.HooksSectionName].(map[string]any); !ok {
							hooksSection = map[string]any{}
						}

						if overridesSection, ok = componentSection[cfg.OverridesSectionName].(map[string]any); !ok {
							overridesSection = map[string]any{}
						}

						if backendSection, ok = componentSection[cfg.BackendSectionName].(map[string]any); !ok {
							backendSection = map[string]any{}
						}

						if backendTypeSection, ok = componentSection[cfg.BackendTypeSectionName].(string); !ok {
							backendTypeSection = ""
						}

						configAndStacksInfo := schema.ConfigAndStacksInfo{
							ComponentFromArg:          componentName,
							Stack:                     stackName,
							ComponentMetadataSection:  metadataSection,
							ComponentVarsSection:      varsSection,
							ComponentSettingsSection:  settingsSection,
							ComponentEnvSection:       envSection,
							ComponentProvidersSection: providersSection,
							ComponentHooksSection:     hooksSection,
							ComponentOverridesSection: overridesSection,
							ComponentBackendSection:   backendSection,
							ComponentBackendType:      backendTypeSection,
							ComponentSection: map[string]any{
								cfg.VarsSectionName:        varsSection,
								cfg.MetadataSectionName:    metadataSection,
								cfg.SettingsSectionName:    settingsSection,
								cfg.EnvSectionName:         envSection,
								cfg.ProvidersSectionName:   providersSection,
								cfg.HooksSectionName:       hooksSection,
								cfg.OverridesSectionName:   overridesSection,
								cfg.BackendSectionName:     backendSection,
								cfg.BackendTypeSectionName: backendTypeSection,
							},
						}

						if comp, ok := configAndStacksInfo.ComponentSection[cfg.ComponentSectionName].(string); !ok || comp == "" {
							configAndStacksInfo.ComponentSection[cfg.ComponentSectionName] = componentName
						}

						// Stack name
						if atmosConfig.Stacks.NameTemplate != "" {
							stackName, err = ProcessTmpl("describe-stacks-name-template", atmosConfig.Stacks.NameTemplate, configAndStacksInfo.ComponentSection, false)
							if err != nil {
								return nil, err
							}
						} else if atmosConfig.Stacks.NamePattern != "" {
							context = cfg.GetContextFromVars(varsSection)
							configAndStacksInfo.Context = context
							stackName, err = cfg.GetContextPrefix(stackFileName, context, GetStackNamePattern(atmosConfig), stackFileName)
							if err != nil {
								return nil, err
							}
						} else {
							// If no name pattern or template is configured, use the stack file name
							stackName = stackFileName
						}

						// Update the component section with the final stack name
						configAndStacksInfo.ComponentSection["atmos_stack"] = stackName
						configAndStacksInfo.ComponentSection["stack"] = stackName

						if filterByStack != "" && filterByStack != stackFileName && filterByStack != stackName {
							continue
						}

						if stackName == "" {
							stackName = stackFileName
						}

						// Only create the stack entry if it doesn't exist
						if !u.MapKeyExists(finalStacksMap, stackName) {
							finalStacksMap[stackName] = make(map[string]any)
							finalStacksMap[stackName].(map[string]any)["components"] = make(map[string]any)
							finalStacksMap[stackName].(map[string]any)["atmos_stack_file"] = stackFileName
						}

						configAndStacksInfo.ComponentSection["atmos_component"] = componentName
						configAndStacksInfo.ComponentSection["atmos_stack_file"] = stackFileName
						configAndStacksInfo.ComponentSection["atmos_manifest"] = stackFileName

						if len(components) == 0 || u.SliceContainsString(components, componentName) || u.SliceContainsString(derivedComponents, componentName) {
							if !u.MapKeyExists(finalStacksMap[stackName].(map[string]any)["components"].(map[string]any), "terraform") {
								finalStacksMap[stackName].(map[string]any)["components"].(map[string]any)["terraform"] = make(map[string]any)
							}
							if !u.MapKeyExists(finalStacksMap[stackName].(map[string]any)["components"].(map[string]any)["terraform"].(map[string]any), componentName) {
								finalStacksMap[stackName].(map[string]any)["components"].(map[string]any)["terraform"].(map[string]any)[componentName] = make(map[string]any)
							}

							// Atmos component, stack, and stack manifest file
							componentSection["atmos_component"] = componentName
							componentSection["atmos_stack"] = stackName
							componentSection["stack"] = stackName
							componentSection["atmos_stack_file"] = stackFileName
							componentSection["atmos_manifest"] = stackFileName

							// Terraform workspace
							workspace, err := BuildTerraformWorkspace(atmosConfig, configAndStacksInfo)
							if err != nil {
								return nil, err
							}
							componentSection["workspace"] = workspace
							configAndStacksInfo.ComponentSection["workspace"] = workspace

							// Process `Go` templates
							if processTemplates {
								componentSectionStr, err := u.ConvertToYAML(componentSection)
								if err != nil {
									return nil, err
								}

								var settingsSectionStruct schema.Settings
								err = mapstructure.Decode(settingsSection, &settingsSectionStruct)
								if err != nil {
									return nil, err
								}

								componentSectionProcessed, err := ProcessTmplWithDatasources(
									atmosConfig,
									settingsSectionStruct,
									"describe-stacks-all-sections",
									componentSectionStr,
									configAndStacksInfo.ComponentSection,
									true,
								)
								if err != nil {
									return nil, err
								}

								componentSectionConverted, err := u.UnmarshalYAML[schema.AtmosSectionMapType](componentSectionProcessed)
								if err != nil {
									if !atmosConfig.Templates.Settings.Enabled {
										if strings.Contains(componentSectionStr, "{{") || strings.Contains(componentSectionStr, "}}") {
											errorMessage := "the stack manifests contain Go templates, but templating is disabled in atmos.yaml in 'templates.settings.enabled'\n" +
												"to enable templating, refer to https://atmos.tools/core-concepts/stacks/templates"
											err = errors.Join(err, errors.New(errorMessage))
										}
									}
									u.LogErrorAndExit(err)
								}

								componentSection = componentSectionConverted
							}

							// Process YAML functions
							if processYamlFunctions {
								componentSectionConverted, err := ProcessCustomYamlTags(
									atmosConfig,
									componentSection,
									configAndStacksInfo.Stack,
									skip,
								)
								if err != nil {
									return nil, err
								}

								componentSection = componentSectionConverted
							}

							// Add sections
							for sectionName, section := range componentSection {
								if len(sections) == 0 || u.SliceContainsString(sections, sectionName) {
									finalStacksMap[stackName].(map[string]any)["components"].(map[string]any)["terraform"].(map[string]any)[componentName].(map[string]any)[sectionName] = section
								}
							}
						}
					}
				}
			}

			if len(componentTypes) == 0 || u.SliceContainsString(componentTypes, "helmfile") {
				if helmfileSection, ok := componentsSection["helmfile"].(map[string]any); ok {
					for componentName, compSection := range helmfileSection {
						componentSection, ok := compSection.(map[string]any)
						if !ok {
							return nil, fmt.Errorf("invalid 'components.helmfile.%s' section in the file '%s'", componentName, stackFileName)
						}

						if comp, ok := componentSection[cfg.ComponentSectionName].(string); !ok || comp == "" {
							componentSection[cfg.ComponentSectionName] = componentName
						}

						// Find all derived components of the provided components and include them in the output
						derivedComponents, err := FindComponentsDerivedFromBaseComponents(stackFileName, helmfileSection, components)
						if err != nil {
							return nil, err
						}

						if varsSection, ok = componentSection[cfg.VarsSectionName].(map[string]any); !ok {
							varsSection = map[string]any{}
						}

						if metadataSection, ok = componentSection[cfg.MetadataSectionName].(map[string]any); !ok {
							metadataSection = map[string]any{}
						}

						if settingsSection, ok = componentSection[cfg.SettingsSectionName].(map[string]any); !ok {
							settingsSection = map[string]any{}
						}

						if envSection, ok = componentSection[cfg.EnvSectionName].(map[string]any); !ok {
							envSection = map[string]any{}
						}

						if providersSection, ok = componentSection[cfg.ProvidersSectionName].(map[string]any); !ok {
							providersSection = map[string]any{}
						}

						if hooksSection, ok = componentSection[cfg.HooksSectionName].(map[string]any); !ok {
							hooksSection = map[string]any{}
						}

						if overridesSection, ok = componentSection[cfg.OverridesSectionName].(map[string]any); !ok {
							overridesSection = map[string]any{}
						}

						if backendSection, ok = componentSection[cfg.BackendSectionName].(map[string]any); !ok {
							backendSection = map[string]any{}
						}

						if backendTypeSection, ok = componentSection[cfg.BackendTypeSectionName].(string); !ok {
							backendTypeSection = ""
						}

						configAndStacksInfo := schema.ConfigAndStacksInfo{
							ComponentFromArg:          componentName,
							Stack:                     stackName,
							ComponentMetadataSection:  metadataSection,
							ComponentVarsSection:      varsSection,
							ComponentSettingsSection:  settingsSection,
							ComponentEnvSection:       envSection,
							ComponentProvidersSection: providersSection,
							ComponentHooksSection:     hooksSection,
							ComponentOverridesSection: overridesSection,
							ComponentBackendSection:   backendSection,
							ComponentBackendType:      backendTypeSection,
							ComponentSection: map[string]any{
								cfg.VarsSectionName:        varsSection,
								cfg.MetadataSectionName:    metadataSection,
								cfg.SettingsSectionName:    settingsSection,
								cfg.EnvSectionName:         envSection,
								cfg.ProvidersSectionName:   providersSection,
								cfg.HooksSectionName:       hooksSection,
								cfg.OverridesSectionName:   overridesSection,
								cfg.BackendSectionName:     backendSection,
								cfg.BackendTypeSectionName: backendTypeSection,
							},
						}

						if comp, ok := configAndStacksInfo.ComponentSection[cfg.ComponentSectionName].(string); !ok || comp == "" {
							configAndStacksInfo.ComponentSection[cfg.ComponentSectionName] = componentName
						}

						// Stack name
						if atmosConfig.Stacks.NameTemplate != "" {
							stackName, err = ProcessTmpl("describe-stacks-name-template", atmosConfig.Stacks.NameTemplate, configAndStacksInfo.ComponentSection, false)
							if err != nil {
								return nil, err
							}
						} else if atmosConfig.Stacks.NamePattern != "" {
							context = cfg.GetContextFromVars(varsSection)
							configAndStacksInfo.Context = context
							stackName, err = cfg.GetContextPrefix(stackFileName, context, GetStackNamePattern(atmosConfig), stackFileName)
							if err != nil {
								return nil, err
							}
						} else {
							// If no name pattern or template is configured, use the stack file name
							stackName = stackFileName
						}

						// Update the component section with the final stack name
						configAndStacksInfo.ComponentSection["atmos_stack"] = stackName
						configAndStacksInfo.ComponentSection["stack"] = stackName

						if filterByStack != "" && filterByStack != stackFileName && filterByStack != stackName {
							continue
						}

						if stackName == "" {
							stackName = stackFileName
						}

						// Only create the stack entry if it doesn't exist
						if !u.MapKeyExists(finalStacksMap, stackName) {
							finalStacksMap[stackName] = make(map[string]any)
							finalStacksMap[stackName].(map[string]any)["components"] = make(map[string]any)
							finalStacksMap[stackName].(map[string]any)["atmos_stack_file"] = stackFileName
						}

						configAndStacksInfo.ComponentSection["atmos_component"] = componentName
						configAndStacksInfo.ComponentSection["atmos_stack_file"] = stackFileName
						configAndStacksInfo.ComponentSection["atmos_manifest"] = stackFileName

						if len(components) == 0 || u.SliceContainsString(components, componentName) || u.SliceContainsString(derivedComponents, componentName) {
							if !u.MapKeyExists(finalStacksMap[stackName].(map[string]any)["components"].(map[string]any), "helmfile") {
								finalStacksMap[stackName].(map[string]any)["components"].(map[string]any)["helmfile"] = make(map[string]any)
							}
							if !u.MapKeyExists(finalStacksMap[stackName].(map[string]any)["components"].(map[string]any)["helmfile"].(map[string]any), componentName) {
								finalStacksMap[stackName].(map[string]any)["components"].(map[string]any)["helmfile"].(map[string]any)[componentName] = make(map[string]any)
							}

							// Atmos component, stack, and stack manifest file
							componentSection["atmos_component"] = componentName
							componentSection["atmos_stack"] = stackName
							componentSection["stack"] = stackName
							componentSection["atmos_stack_file"] = stackFileName
							componentSection["atmos_manifest"] = stackFileName

							// Process `Go` templates
							if processTemplates {
								componentSectionStr, err := u.ConvertToYAML(componentSection)
								if err != nil {
									return nil, err
								}

								var settingsSectionStruct schema.Settings
								err = mapstructure.Decode(settingsSection, &settingsSectionStruct)
								if err != nil {
									return nil, err
								}

								componentSectionProcessed, err := ProcessTmplWithDatasources(
									atmosConfig,
									settingsSectionStruct,
									"templates-describe-stacks-all-atmos-sections",
									componentSectionStr,
									configAndStacksInfo.ComponentSection,
									true,
								)
								if err != nil {
									return nil, err
								}

								componentSectionConverted, err := u.UnmarshalYAML[schema.AtmosSectionMapType](componentSectionProcessed)
								if err != nil {
									if !atmosConfig.Templates.Settings.Enabled {
										if strings.Contains(componentSectionStr, "{{") || strings.Contains(componentSectionStr, "}}") {
											errorMessage := "the stack manifests contain Go templates, but templating is disabled in atmos.yaml in 'templates.settings.enabled'\n" +
												"to enable templating, refer to https://atmos.tools/core-concepts/stacks/templates"
											err = errors.Join(err, errors.New(errorMessage))
										}
									}
									u.LogErrorAndExit(err)
								}

								componentSection = componentSectionConverted
							}

							// Process YAML functions
							if processYamlFunctions {
								componentSectionConverted, err := ProcessCustomYamlTags(
									atmosConfig,
									componentSection,
									configAndStacksInfo.Stack,
									skip,
								)
								if err != nil {
									return nil, err
								}

								componentSection = componentSectionConverted
							}

							// Add sections
							for sectionName, section := range componentSection {
								if len(sections) == 0 || u.SliceContainsString(sections, sectionName) {
									finalStacksMap[stackName].(map[string]any)["components"].(map[string]any)["helmfile"].(map[string]any)[componentName].(map[string]any)[sectionName] = section
								}
							}
						}
					}
				}
			}
		}
	}

	// Filter out empty stacks after processing all stack files
	if !includeEmptyStacks {
		for stackName := range finalStacksMap {
			if stackName == "" {
				delete(finalStacksMap, stackName)
				continue
			}

			stackEntry, ok := finalStacksMap[stackName].(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid stack entry type for stack %s", stackName)
			}
			componentsSection, hasComponents := stackEntry["components"].(map[string]any)

			if !hasComponents {
				delete(finalStacksMap, stackName)
				continue
			}

			// Check if any component type (terraform/helmfile) has components
			hasNonEmptyComponents := false
			for _, components := range componentsSection {
				if compTypeMap, ok := components.(map[string]any); ok {
					for _, comp := range compTypeMap {
						if compContent, ok := comp.(map[string]any); ok {
							// Check for any meaningful content
							relevantSections := []string{"vars", "metadata", "settings", "env", "workspace"}
							for _, section := range relevantSections {
								if _, hasSection := compContent[section]; hasSection {
									hasNonEmptyComponents = true
									break
								}
							}
						}
					}
				}
				if hasNonEmptyComponents {
					break
				}
			}

			if !hasNonEmptyComponents {
				delete(finalStacksMap, stackName)
				continue
			}
		}
	} else {
		// Process stacks normally without special handling for any prefixes
		for stackName, stackConfig := range finalStacksMap {
			finalStacksMap[stackName] = stackConfig
		}
	}

	return finalStacksMap, nil
}
