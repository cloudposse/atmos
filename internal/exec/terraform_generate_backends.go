package exec

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ExecuteTerraformGenerateBackendsCmd executes `terraform generate backends` command.
func ExecuteTerraformGenerateBackendsCmd(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "exec.ExecuteTerraformGenerateBackendsCmd")()

	info, err := ProcessCommandLineArgs("terraform", cmd, args, nil)
	if err != nil {
		return err
	}

	info.CliArgs = []string{"terraform", "generate", "backends"}

	atmosConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		return err
	}

	flags := cmd.Flags()

	fileTemplate, err := flags.GetString("file-template")
	if err != nil {
		return err
	}

	stacksCsv, err := flags.GetString("stacks")
	if err != nil {
		return err
	}
	var stacks []string
	if stacksCsv != "" {
		stacks = strings.Split(stacksCsv, ",")
	}

	componentsCsv, err := flags.GetString("components")
	if err != nil {
		return err
	}
	var components []string
	if componentsCsv != "" {
		components = strings.Split(componentsCsv, ",")
	}

	format, err := flags.GetString("format")
	if err != nil {
		return err
	}
	if format != "" && format != "json" && format != "hcl" && format != "backend-config" {
		return fmt.Errorf("invalid '--format' argument '%s'. Valid values are 'hcl', 'json', and 'backend-config'", format)
	}
	if format == "" {
		format = "hcl"
	}

	return ExecuteTerraformGenerateBackends(&atmosConfig, fileTemplate, format, stacks, components)
}

// ExecuteTerraformGenerateBackends generates backend configs for all terraform components.
func ExecuteTerraformGenerateBackends(
	atmosConfig *schema.AtmosConfiguration,
	fileTemplate string,
	format string,
	stacks []string,
	components []string,
) error {
	defer perf.Track(atmosConfig, "exec.ExecuteTerraformGenerateBackends")()

	stacksMap, _, err := FindStacksMap(atmosConfig, false)
	if err != nil {
		return err
	}

	var ok bool
	var componentsSection map[string]any
	var terraformSection map[string]any
	var componentSection map[string]any
	var metadataSection map[string]any
	var varsSection map[string]any
	var settingsSection map[string]any
	var envSection map[string]any
	var providersSection map[string]any
	var authSection map[string]any
	var hooksSection map[string]any
	var overridesSection map[string]any
	var backendSection map[string]any
	var backendTypeSection string
	processedTerraformComponents := map[string]any{}
	fileTemplateProvided := fileTemplate != ""

	for stackFileName, stackSection := range stacksMap {
		if componentsSection, ok = stackSection.(map[string]any)["components"].(map[string]any); !ok {
			continue
		}

		if terraformSection, ok = componentsSection[cfg.TerraformSectionName].(map[string]any); !ok {
			continue
		}

		for componentName, compSection := range terraformSection {
			if componentSection, ok = compSection.(map[string]any); !ok {
				continue
			}

			// Check if `components` filter is provided
			if len(components) == 0 ||
				u.SliceContainsString(components, componentName) {

				// Component metadata
				if metadataSection, ok = componentSection[cfg.MetadataSectionName].(map[string]any); ok {
					if componentType, ok := metadataSection["type"].(string); ok {
						// Don't include abstract components
						if componentType == "abstract" {
							continue
						}
					}
				}

				// Component backend
				if backendSection, ok = componentSection[cfg.BackendSectionName].(map[string]any); !ok {
					continue
				}

				// Backend type
				if backendTypeSection, ok = componentSection[cfg.BackendTypeSectionName].(string); !ok {
					continue
				}

				if varsSection, ok = componentSection[cfg.VarsSectionName].(map[string]any); !ok {
					varsSection = map[string]any{}
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

				if authSection, ok = componentSection[cfg.AuthSectionName].(map[string]any); !ok {
					authSection = map[string]any{}
				}

				if hooksSection, ok = componentSection[cfg.HooksSectionName].(map[string]any); !ok {
					hooksSection = map[string]any{}
				}

				if overridesSection, ok = componentSection[cfg.OverridesSectionName].(map[string]any); !ok {
					overridesSection = map[string]any{}
				}

				// Find terraform component.
				// If `component` attribute is present, it's the terraform component.
				// Otherwise, the YAML component name is the terraform component.
				terraformComponent := componentName
				if componentAttribute, ok := componentSection[cfg.ComponentSectionName].(string); ok {
					terraformComponent = componentAttribute
				}

				// Path to the terraform component
				terraformComponentPath := filepath.Join(
					atmosConfig.BasePath,
					atmosConfig.Components.Terraform.BasePath,
					terraformComponent,
				)

				configAndStacksInfo := schema.ConfigAndStacksInfo{
					ComponentFromArg:          componentName,
					ComponentMetadataSection:  metadataSection,
					ComponentVarsSection:      varsSection,
					ComponentSettingsSection:  settingsSection,
					ComponentEnvSection:       envSection,
					ComponentAuthSection:      authSection,
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

				// Context
				context := cfg.GetContextFromVars(varsSection)
				context.Component = strings.Replace(componentName, "/", "-", -1)
				context.ComponentPath = terraformComponentPath

				// Stack name
				var stackName string
				if atmosConfig.Stacks.NameTemplate != "" {
					stackName, err = ProcessTmpl(atmosConfig, "terraform-generate-backends-template", atmosConfig.Stacks.NameTemplate, configAndStacksInfo.ComponentSection, false)
					if err != nil {
						return err
					}
				} else {
					stackName, err = cfg.GetContextPrefix(stackFileName, context, GetStackNamePattern(atmosConfig), stackFileName)
					if err != nil {
						return err
					}
				}

				configAndStacksInfo.ComponentSection["atmos_component"] = componentName
				configAndStacksInfo.ComponentSection["atmos_stack"] = stackName
				configAndStacksInfo.ComponentSection["stack"] = stackName
				configAndStacksInfo.ComponentSection["atmos_stack_file"] = stackFileName
				configAndStacksInfo.ComponentSection["atmos_manifest"] = stackFileName

				// Process `Go` templates
				componentSectionStr, err := u.ConvertToYAML(componentSection)
				if err != nil {
					return err
				}

				var settingsSectionStruct schema.Settings
				err = mapstructure.Decode(settingsSection, &settingsSectionStruct)
				if err != nil {
					return err
				}

				componentSectionProcessed, err := ProcessTmplWithDatasources(
					atmosConfig,
					&configAndStacksInfo,
					settingsSectionStruct,
					"terraform-generate-backends",
					componentSectionStr,
					configAndStacksInfo.ComponentSection,
					true,
				)
				if err != nil {
					return err
				}

				componentSectionConverted, err := u.UnmarshalYAML[schema.AtmosSectionMapType](componentSectionProcessed)
				if err != nil {
					if !atmosConfig.Templates.Settings.Enabled {
						if strings.Contains(componentSectionStr, "{{") || strings.Contains(componentSectionStr, "}}") {
							errorMessage := "the stack manifests contain Go templates, but templating is disabled in atmos.yaml in 'templates.settings.enabled'\n" +
								"to enable templating, refer to https://atmos.tools/core-concepts/stacks/templating"
							err = errors.Join(err, errors.New(errorMessage))
						}
					}
					errUtils.CheckErrorPrintAndExit(err, "", "")
				}

				componentSectionFinal, err := ProcessCustomYamlTags(atmosConfig, componentSectionConverted, stackName, nil, &configAndStacksInfo)
				if err != nil {
					return err
				}

				componentSection = componentSectionFinal

				if i, ok := componentSection[cfg.BackendSectionName].(map[string]any); ok {
					backendSection = i
				}

				if i, ok := componentSection[cfg.BackendTypeSectionName].(string); ok {
					backendTypeSection = i
				}

				var backendFilePath string
				var backendFileAbsolutePath string

				// Check if `stacks` filter is provided
				if len(stacks) == 0 ||
					// `stacks` filter can contain the names of the top-level stack config files:
					// atmos terraform generate varfiles --stacks=orgs/cp/tenant1/staging/us-east-2,orgs/cp/tenant2/dev/us-east-2
					u.SliceContainsString(stacks, stackFileName) ||
					// `stacks` filter can also contain the logical stack names (derived from the context vars):
					// atmos terraform generate varfiles --stacks=tenant1-ue2-staging,tenant1-ue2-prod
					u.SliceContainsString(stacks, stackName) {

					// If '--file-template' is not specified, don't check if we've already processed the terraform component,
					// and write the backends to the terraform components folders
					if !fileTemplateProvided {
						// If the terraform component has been already processed, continue
						if u.MapKeyExists(processedTerraformComponents, terraformComponent) {
							continue
						}

						processedTerraformComponents[terraformComponent] = terraformComponent

						backendFilePath = filepath.Join(
							terraformComponentPath,
							"backend.tf",
						)

						if format == "json" {
							backendFilePath = backendFilePath + ".json"
						}

						backendFileAbsolutePath, err = filepath.Abs(backendFilePath)
						if err != nil {
							return err
						}
					} else {
						// Replace the tokens in the file template
						// Supported context tokens: {namespace}, {tenant}, {environment}, {region}, {stage}, {base-component}, {component}, {component-path}
						backendFilePath = cfg.ReplaceContextTokens(context, fileTemplate)
						backendFileAbsolutePath, err = filepath.Abs(backendFilePath)
						if err != nil {
							return err
						}

						// Create all the intermediate subdirectories
						err = u.EnsureDir(backendFileAbsolutePath)
						if err != nil {
							return err
						}
					}

					// Write the backend config to the file
					log.Debug("Writing backend config for the component to file", "component", terraformComponent, "file", backendFilePath)

					if format == "json" {
						componentBackendConfig, err := generateComponentBackendConfig(backendTypeSection, backendSection, "")
						if err != nil {
							return err
						}

						err = u.WriteToFileAsJSON(backendFileAbsolutePath, componentBackendConfig, 0o644)
						if err != nil {
							return err
						}
					} else if format == "hcl" {
						err = u.WriteTerraformBackendConfigToFileAsHcl(backendFileAbsolutePath, backendTypeSection, backendSection)
						if err != nil {
							return err
						}
					} else if format == "backend-config" {
						err = u.WriteToFileAsHcl(backendFileAbsolutePath, backendSection, 0o644)
						if err != nil {
							return err
						}
					} else {
						return fmt.Errorf("invalid '--format' argument '%s'. Valid values are 'hcl' (default), 'json' and 'backend-config'", format)
					}
				}
			}
		}
	}

	return nil
}
