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

	includeEmptyStacks, err := cmd.Flags().GetBool("include-empty-stacks")
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

	finalStacksMap, err := ExecuteDescribeStacks(
		cliConfig,
		filterByStack,
		components,
		componentTypes,
		sections,
		false,
		processTemplates,
		includeEmptyStacks,
	)
	if err != nil {
		return err
	}

	err = printOrWriteToFile(format, file, finalStacksMap)
	if err != nil {
		return err
	}

	return nil
}

// ExecuteDescribeStacks processes stack manifests and returns the final map of stacks and components
func ExecuteDescribeStacks(
	cliConfig schema.CliConfiguration,
	filterByStack string,
	components []string,
	componentTypes []string,
	sections []string,
	ignoreMissingFiles bool,
	processTemplates bool,
	includeEmptyStacks bool,
) (map[string]any, error) {

	stacksMap, _, err := FindStacksMap(cliConfig, ignoreMissingFiles)
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
	var overridesSection map[string]any
	var backendSection map[string]any
	var backendTypeSection string
	var stackName string
	context := schema.Context{}

	for stackFileName, stackSection := range stacksMap {
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
							ComponentOverridesSection: overridesSection,
							ComponentBackendSection:   backendSection,
							ComponentBackendType:      backendTypeSection,
							ComponentSection: map[string]any{
								cfg.VarsSectionName:        varsSection,
								cfg.MetadataSectionName:    metadataSection,
								cfg.SettingsSectionName:    settingsSection,
								cfg.EnvSectionName:         envSection,
								cfg.ProvidersSectionName:   providersSection,
								cfg.OverridesSectionName:   overridesSection,
								cfg.BackendSectionName:     backendSection,
								cfg.BackendTypeSectionName: backendTypeSection,
							},
						}

						if comp, ok := configAndStacksInfo.ComponentSection[cfg.ComponentSectionName].(string); !ok || comp == "" {
							configAndStacksInfo.ComponentSection[cfg.ComponentSectionName] = componentName
						}

						// Stack name
						if cliConfig.Stacks.NameTemplate != "" {
							stackName, err = ProcessTmpl("describe-stacks-name-template", cliConfig.Stacks.NameTemplate, configAndStacksInfo.ComponentSection, false)
							if err != nil {
								return nil, err
							}
						} else {
							context = cfg.GetContextFromVars(varsSection)
							configAndStacksInfo.Context = context
							stackName, err = cfg.GetContextPrefix(stackFileName, context, GetStackNamePattern(cliConfig), stackFileName)
							if err != nil {
								return nil, err
							}
						}

						if filterByStack != "" && filterByStack != stackFileName && filterByStack != stackName {
							continue
						}

						if stackName == "" {
							stackName = stackFileName
						} else if strings.HasPrefix(stackFileName, "deploy/") {
							// If we have a deploy/ prefixed version, use that as the canonical name
							stackName = stackFileName
						}

						// Only create the stack entry if it doesn't exist or if we're using the canonical name
						if !u.MapKeyExists(finalStacksMap, stackName) || strings.HasPrefix(stackName, "deploy/") {
							finalStacksMap[stackName] = make(map[string]any)
						}

						configAndStacksInfo.Stack = stackName
						configAndStacksInfo.ComponentSection["atmos_component"] = componentName
						configAndStacksInfo.ComponentSection["atmos_stack"] = stackName
						configAndStacksInfo.ComponentSection["stack"] = stackName
						configAndStacksInfo.ComponentSection["atmos_stack_file"] = stackFileName
						configAndStacksInfo.ComponentSection["atmos_manifest"] = stackFileName

						if len(components) == 0 || u.SliceContainsString(components, componentName) || u.SliceContainsString(derivedComponents, componentName) {
							if !u.MapKeyExists(finalStacksMap[stackName].(map[string]any), "components") {
								finalStacksMap[stackName].(map[string]any)["components"] = make(map[string]any)
							}
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
							workspace, err := BuildTerraformWorkspace(cliConfig, configAndStacksInfo)
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
									cliConfig,
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
									if !cliConfig.Templates.Settings.Enabled {
										if strings.Contains(componentSectionStr, "{{") || strings.Contains(componentSectionStr, "}}") {
											errorMessage := "the stack manifests contain Go templates, but templating is disabled in atmos.yaml in 'templates.settings.enabled'\n" +
												"to enable templating, refer to https://atmos.tools/core-concepts/stacks/templates"
											err = errors.Join(err, errors.New(errorMessage))
										}
									}
									u.LogErrorAndExit(cliConfig, err)
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
							ComponentOverridesSection: overridesSection,
							ComponentBackendSection:   backendSection,
							ComponentBackendType:      backendTypeSection,
							ComponentSection: map[string]any{
								cfg.VarsSectionName:        varsSection,
								cfg.MetadataSectionName:    metadataSection,
								cfg.SettingsSectionName:    settingsSection,
								cfg.EnvSectionName:         envSection,
								cfg.ProvidersSectionName:   providersSection,
								cfg.OverridesSectionName:   overridesSection,
								cfg.BackendSectionName:     backendSection,
								cfg.BackendTypeSectionName: backendTypeSection,
							},
						}

						if comp, ok := configAndStacksInfo.ComponentSection[cfg.ComponentSectionName].(string); !ok || comp == "" {
							configAndStacksInfo.ComponentSection[cfg.ComponentSectionName] = componentName
						}

						// Stack name
						if cliConfig.Stacks.NameTemplate != "" {
							stackName, err = ProcessTmpl("describe-stacks-name-template", cliConfig.Stacks.NameTemplate, configAndStacksInfo.ComponentSection, false)
							if err != nil {
								return nil, err
							}
						} else {
							context = cfg.GetContextFromVars(varsSection)
							configAndStacksInfo.Context = context
							stackName, err = cfg.GetContextPrefix(stackFileName, context, GetStackNamePattern(cliConfig), stackFileName)
							if err != nil {
								return nil, err
							}
						}

						if filterByStack != "" && filterByStack != stackFileName && filterByStack != stackName {
							continue
						}

						if stackName == "" {
							stackName = stackFileName
						} else if strings.HasPrefix(stackFileName, "deploy/") {
							// If we have a deploy/ prefixed version, use that as the canonical name
							stackName = stackFileName
						}

						// Only create the stack entry if it doesn't exist or if we're using the canonical name
						if !u.MapKeyExists(finalStacksMap, stackName) || strings.HasPrefix(stackName, "deploy/") {
							finalStacksMap[stackName] = make(map[string]any)
						}

						configAndStacksInfo.ComponentSection["atmos_component"] = componentName
						configAndStacksInfo.ComponentSection["atmos_stack"] = stackName
						configAndStacksInfo.ComponentSection["stack"] = stackName
						configAndStacksInfo.ComponentSection["atmos_stack_file"] = stackFileName
						configAndStacksInfo.ComponentSection["atmos_manifest"] = stackFileName

						if len(components) == 0 || u.SliceContainsString(components, componentName) || u.SliceContainsString(derivedComponents, componentName) {
							if !u.MapKeyExists(finalStacksMap[stackName].(map[string]any), "components") {
								finalStacksMap[stackName].(map[string]any)["components"] = make(map[string]any)
							}
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
									cliConfig,
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
									if !cliConfig.Templates.Settings.Enabled {
										if strings.Contains(componentSectionStr, "{{") || strings.Contains(componentSectionStr, "}}") {
											errorMessage := "the stack manifests contain Go templates, but templating is disabled in atmos.yaml in 'templates.settings.enabled'\n" +
												"to enable templating, refer to https://atmos.tools/core-concepts/stacks/templates"
											err = errors.Join(err, errors.New(errorMessage))
										}
									}
									u.LogErrorAndExit(cliConfig, err)
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

		fmt.Printf("DEBUG: Final stack map for %s: %+v\n", stackName, finalStacksMap[stackName])
	}

	fmt.Printf("DEBUG: Before final filtering - Stack count: %d\n", len(finalStacksMap))

	// Filter out empty stacks after processing all stack files
	if !includeEmptyStacks {
		fmt.Printf("DEBUG: Starting empty stack filtering\n")
		for stackName := range finalStacksMap {
			fmt.Printf("DEBUG: Checking final stack: %s\n", stackName)

			if stackName == "" {
				fmt.Printf("DEBUG: Removing empty stack name\n")
				delete(finalStacksMap, stackName)
				continue
			}

			stackEntry := finalStacksMap[stackName].(map[string]any)
			componentsSection, hasComponents := stackEntry["components"].(map[string]any)
			fmt.Printf("DEBUG: Stack %s has components section: %v\n", stackName, hasComponents)

			if !hasComponents {
				fmt.Printf("DEBUG: Removing stack %s - no components section\n", stackName)
				delete(finalStacksMap, stackName)
				continue
			}

			// Check if any component type (terraform/helmfile) has components
			hasNonEmptyComponents := false
			for componentType, components := range componentsSection {
				fmt.Printf("DEBUG: Checking component type: %s\n", componentType)
				if compTypeMap, ok := components.(map[string]any); ok {
					for compName, comp := range compTypeMap {
						fmt.Printf("DEBUG: Checking component: %s\n", compName)
						if compContent, ok := comp.(map[string]any); ok {
							// Check for any meaningful content
							relevantSections := []string{"vars", "metadata", "settings", "env"}
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
				fmt.Printf("DEBUG: Removing stack %s - no non-empty components\n", stackName)
				delete(finalStacksMap, stackName)
				continue
			}

			// Check for duplicate stacks (deploy/ prefix)
			if strings.HasPrefix(stackName, "deploy/") {
				baseStackName := strings.TrimPrefix(stackName, "deploy/")
				if _, exists := finalStacksMap[baseStackName]; exists {
					fmt.Printf("DEBUG: Found duplicate stack %s (base: %s)\n", stackName, baseStackName)
					// Use the deploy/ prefixed version as canonical and remove the base name
					delete(finalStacksMap, baseStackName)
				}
			}
		}
	} else {
		fmt.Printf("DEBUG: Skipping empty stack filtering because includeEmptyStacks is true\n")
	}

	fmt.Printf("DEBUG: After final filtering - Stack count: %d\n", len(finalStacksMap))

	return finalStacksMap, nil
}
