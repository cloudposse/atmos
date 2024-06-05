package exec

import (
	"fmt"
	"github.com/spf13/cobra"
	"path"
	"path/filepath"
	"strings"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ExecuteTerraformGenerateBackendsCmd executes `terraform generate backends` command
func ExecuteTerraformGenerateBackendsCmd(cmd *cobra.Command, args []string) error {
	info, err := processCommandLineArgs("terraform", cmd, args, nil)
	if err != nil {
		return err
	}

	cliConfig, err := cfg.InitCliConfig(info, true)
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

	return ExecuteTerraformGenerateBackends(cliConfig, fileTemplate, format, stacks, components)
}

// ExecuteTerraformGenerateBackends generates backend configs for all terraform components
func ExecuteTerraformGenerateBackends(cliConfig schema.CliConfiguration, fileTemplate string, format string, stacks []string, components []string) error {
	stacksMap, _, err := FindStacksMap(cliConfig, false)
	if err != nil {
		return err
	}

	var ok bool
	var componentsSection map[string]any
	var terraformSection map[string]any
	var componentSection map[string]any
	var varsSection map[any]any
	var backendSection map[any]any
	var backendType string
	processedTerraformComponents := map[string]any{}
	fileTemplateProvided := fileTemplate != ""

	for stackFileName, stackSection := range stacksMap {
		if componentsSection, ok = stackSection.(map[any]any)["components"].(map[string]any); !ok {
			continue
		}

		if terraformSection, ok = componentsSection["terraform"].(map[string]any); !ok {
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
				metadataSection := map[any]any{}
				if metadataSection, ok = componentSection["metadata"].(map[any]any); ok {
					if componentType, ok := metadataSection["type"].(string); ok {
						// Don't include abstract components
						if componentType == "abstract" {
							continue
						}
					}
				}

				// Component backend
				if backendSection, ok = componentSection["backend"].(map[any]any); !ok {
					continue
				}

				// Backend type
				if backendType, ok = componentSection["backend_type"].(string); !ok {
					continue
				}

				// Find terraform component.
				// If `component` attribute is present, it's the terraform component.
				// Otherwise, the YAML component name is the terraform component.
				terraformComponent := componentName
				if componentAttribute, ok := componentSection[cfg.ComponentSectionName].(string); ok {
					terraformComponent = componentAttribute
				}

				// Path to the terraform component
				terraformComponentPath := path.Join(
					cliConfig.BasePath,
					cliConfig.Components.Terraform.BasePath,
					terraformComponent,
				)

				// Context
				if varsSection, ok = componentSection["vars"].(map[any]any); !ok {
					varsSection = map[any]any{}
				}
				context := cfg.GetContextFromVars(varsSection)
				context.Component = strings.Replace(componentName, "/", "-", -1)
				context.ComponentPath = terraformComponentPath
				contextPrefix, err := cfg.GetContextPrefix(stackFileName, context, GetStackNamePattern(cliConfig), stackFileName)
				if err != nil {
					return err
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
					u.SliceContainsString(stacks, contextPrefix) {

					// If '--file-template' is not specified, don't check if we've already processed the terraform component,
					// and write the backends to the terraform components folders
					if !fileTemplateProvided {
						// If the terraform component has been already processed, continue
						if u.MapKeyExists(processedTerraformComponents, terraformComponent) {
							continue
						}

						processedTerraformComponents[terraformComponent] = terraformComponent

						backendFilePath = path.Join(
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
					u.LogDebug(cliConfig, fmt.Sprintf("Writing backend config for the component '%s' to file '%s'", terraformComponent, backendFilePath))

					if format == "json" {
						componentBackendConfig, err := generateComponentBackendConfig(backendType, backendSection, "")
						if err != nil {
							return err
						}

						err = u.WriteToFileAsJSON(backendFileAbsolutePath, componentBackendConfig, 0644)
						if err != nil {
							return err
						}
					} else if format == "hcl" {
						err = u.WriteTerraformBackendConfigToFileAsHcl(cliConfig, backendFileAbsolutePath, backendType, backendSection)
						if err != nil {
							return err
						}
					} else if format == "backend-config" {
						err = u.WriteToFileAsHcl(cliConfig, backendFileAbsolutePath, backendSection, 0644)
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
