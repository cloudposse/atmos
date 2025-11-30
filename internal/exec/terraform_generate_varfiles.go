package exec

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/go-viper/mapstructure/v2"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ExecuteTerraformGenerateVarfilesCmd executes `terraform generate varfiles` command.
// Deprecated: Use ExecuteTerraformGenerateVarfiles with typed parameters instead.
func ExecuteTerraformGenerateVarfilesCmd(cmd interface{}, args []string) error {
	defer perf.Track(nil, "exec.ExecuteTerraformGenerateVarfilesCmd")()

	return errUtils.ErrDeprecatedCmdNotCallable
}

// ExecuteTerraformGenerateVarfiles generates varfiles for all terraform components in all stacks.
func ExecuteTerraformGenerateVarfiles(
	atmosConfig *schema.AtmosConfiguration,
	fileTemplate string,
	format string,
	stacks []string,
	components []string,
) error {
	defer perf.Track(atmosConfig, "exec.ExecuteTerraformGenerateVarfiles")()

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
	var overridesSection map[string]any
	var backendSection map[string]any
	var backendTypeSection string

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

				// Component vars
				if varsSection, ok = componentSection[cfg.VarsSectionName].(map[string]any); !ok {
					continue
				}

				// Component metadata
				if metadataSection, ok = componentSection[cfg.MetadataSectionName].(map[string]any); ok {
					if componentType, ok := metadataSection["type"].(string); ok {
						// Don't include abstract components
						if componentType == "abstract" {
							continue
						}
					}
				}

				if settingsSection, ok = componentSection[cfg.SettingsSectionName].(map[string]any); !ok {
					settingsSection = map[string]any{}
				}

				if envSection, ok = componentSection[cfg.EnvSectionName].(map[string]any); !ok {
					envSection = map[string]any{}
				}

				if authSection, ok = componentSection[cfg.AuthSectionName].(map[string]any); !ok {
					authSection = map[string]any{}
				}

				if providersSection, ok = componentSection[cfg.ProvidersSectionName].(map[string]any); !ok {
					providersSection = map[string]any{}
				}

				if overridesSection, ok = componentSection[cfg.OverridesSectionName].(map[string]any); !ok {
					overridesSection = map[string]any{}
				}

				// Component backend
				if backendSection, ok = componentSection[cfg.BackendSectionName].(map[string]any); !ok {
					backendSection = map[string]any{}
				}

				// Backend type
				if backendTypeSection, ok = componentSection[cfg.BackendTypeSectionName].(string); !ok {
					backendTypeSection = ""
				}

				// Find terraform component.
				// If `component` attribute is present, it's the terraform component.
				// Otherwise, the YAML component name is the terraform component.
				terraformComponent := componentName
				if componentAttribute, ok := componentSection[cfg.ComponentSectionName].(string); ok {
					terraformComponent = componentAttribute
				}

				// Absolute path to the terraform component
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
					ComponentOverridesSection: overridesSection,
					ComponentBackendSection:   backendSection,
					ComponentBackendType:      backendTypeSection,
					ComponentSection: map[string]any{
						cfg.VarsSectionName:        varsSection,
						cfg.MetadataSectionName:    metadataSection,
						cfg.SettingsSectionName:    settingsSection,
						cfg.EnvSectionName:         envSection,
						cfg.AuthSectionName:        authSection,
						cfg.ProvidersSectionName:   providersSection,
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
					stackName, err = ProcessTmpl(atmosConfig, "terraform-generate-varfiles-template", atmosConfig.Stacks.NameTemplate, configAndStacksInfo.ComponentSection, false)
					if err != nil {
						return err
					}
				} else {
					stackName, err = cfg.GetContextPrefix(stackFileName, context, GetStackNamePattern(atmosConfig), stackFileName)
					if err != nil {
						return err
					}
				}

				configAndStacksInfo.Context = context
				configAndStacksInfo.Stack = stackName
				configAndStacksInfo.ComponentSection["atmos_component"] = componentName
				configAndStacksInfo.ComponentSection["atmos_stack"] = stackName
				configAndStacksInfo.ComponentSection["stack"] = stackName
				configAndStacksInfo.ComponentSection["atmos_stack_file"] = stackFileName
				configAndStacksInfo.ComponentSection["atmos_manifest"] = stackFileName

				// Terraform workspace
				workspace, err := BuildTerraformWorkspace(atmosConfig, configAndStacksInfo)
				if err != nil {
					return err
				}
				componentSection["workspace"] = workspace
				configAndStacksInfo.ComponentSection["workspace"] = workspace

				// Atmos component, stack, and stack manifest file
				componentSection["atmos_component"] = componentName
				componentSection["atmos_stack"] = stackName
				componentSection["stack"] = stackName
				componentSection["atmos_stack_file"] = stackFileName
				componentSection["atmos_manifest"] = stackFileName

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
					"terraform-generate-varfiles",
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

				if i, ok := componentSection[cfg.VarsSectionName].(map[string]any); ok {
					varsSection = i
				}

				// Check if `stacks` filter is provided
				if len(stacks) == 0 ||
					// `stacks` filter can contain the names of the top-level stack config files:
					// atmos terraform generate varfiles --stacks=orgs/cp/tenant1/staging/us-east-2,orgs/cp/tenant2/dev/us-east-2
					u.SliceContainsString(stacks, stackFileName) ||
					// `stacks` filter can also contain the logical stack names (derived from the context vars):
					// atmos terraform generate varfiles --stacks=tenant1-ue2-staging,tenant1-ue2-prod
					u.SliceContainsString(stacks, stackName) {

					// Replace the tokens in the file template
					// Supported context tokens: {namespace}, {tenant}, {environment}, {region}, {stage}, {base-component}, {component}, {component-path}
					fileName := cfg.ReplaceContextTokens(context, fileTemplate)
					fileAbsolutePath, err := filepath.Abs(fileName)
					if err != nil {
						return err
					}

					// Create all the intermediate subdirectories
					err = u.EnsureDir(fileAbsolutePath)
					if err != nil {
						return err
					}

					// Write the varfile
					if format == "yaml" {
						err = u.WriteToFileAsYAML(fileAbsolutePath, varsSection, 0o644)
						if err != nil {
							return err
						}
					} else if format == "json" {
						err = u.WriteToFileAsJSON(fileAbsolutePath, varsSection, 0o644)
						if err != nil {
							return err
						}
					} else if format == "hcl" {
						err = u.WriteToFileAsHcl(fileAbsolutePath, varsSection, 0o644)
						if err != nil {
							return err
						}
					} else {
						return fmt.Errorf("%w: invalid '--format' argument '%s'. Valid values are 'json' (default), 'yaml' and 'hcl'", errUtils.ErrInvalidFlag, format)
					}

					log.Debug("varfile", fileName)
					log.Debug("terraform component", terraformComponent)
					log.Debug("atmos component", componentName)
					log.Debug("atmos stack", stackName)
					log.Debug("stack config file", stackFileName)
				}
			}
		}
	}

	return nil
}
