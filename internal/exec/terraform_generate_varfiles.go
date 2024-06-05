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

// ExecuteTerraformGenerateVarfilesCmd executes `terraform generate varfiles` command
func ExecuteTerraformGenerateVarfilesCmd(cmd *cobra.Command, args []string) error {
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
	if format != "" && format != "yaml" && format != "json" && format != "hcl" {
		return fmt.Errorf("invalid '--format' argument '%s'. Valid values are 'json' (default), 'yaml' and 'hcl", format)
	}
	if format == "" {
		format = "json"
	}

	return ExecuteTerraformGenerateVarfiles(cliConfig, fileTemplate, format, stacks, components)
}

// ExecuteTerraformGenerateVarfiles generates varfiles for all terraform components in all stacks
func ExecuteTerraformGenerateVarfiles(cliConfig schema.CliConfiguration, fileTemplate string, format string, stacks []string, components []string) error {
	stacksMap, _, err := FindStacksMap(cliConfig, false)
	if err != nil {
		return err
	}

	var ok bool
	var componentsSection map[string]any
	var terraformSection map[string]any
	var componentSection map[string]any
	var varsSection map[any]any

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

				// Component vars
				if varsSection, ok = componentSection["vars"].(map[any]any); !ok {
					continue
				}

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

				// Find terraform component.
				// If `component` attribute is present, it's the terraform component.
				// Otherwise, the YAML component name is the terraform component.
				terraformComponent := componentName
				if componentAttribute, ok := componentSection[cfg.ComponentSectionName].(string); ok {
					terraformComponent = componentAttribute
				}

				// Absolute path to the terraform component
				terraformComponentPath := path.Join(
					cliConfig.BasePath,
					cliConfig.Components.Terraform.BasePath,
					terraformComponent,
				)

				// Context
				context := cfg.GetContextFromVars(varsSection)
				context.Component = strings.Replace(componentName, "/", "-", -1)
				context.ComponentPath = terraformComponentPath
				contextPrefix, err := cfg.GetContextPrefix(stackFileName, context, GetStackNamePattern(cliConfig), stackFileName)
				if err != nil {
					return err
				}

				// Check if `stacks` filter is provided
				if len(stacks) == 0 ||
					// `stacks` filter can contain the names of the top-level stack config files:
					// atmos terraform generate varfiles --stacks=orgs/cp/tenant1/staging/us-east-2,orgs/cp/tenant2/dev/us-east-2
					u.SliceContainsString(stacks, stackFileName) ||
					// `stacks` filter can also contain the logical stack names (derived from the context vars):
					// atmos terraform generate varfiles --stacks=tenant1-ue2-staging,tenant1-ue2-prod
					u.SliceContainsString(stacks, contextPrefix) {

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
						err = u.WriteToFileAsYAML(fileAbsolutePath, varsSection, 0644)
						if err != nil {
							return err
						}
					} else if format == "json" {
						err = u.WriteToFileAsJSON(fileAbsolutePath, varsSection, 0644)
						if err != nil {
							return err
						}
					} else if format == "hcl" {
						err = u.WriteToFileAsHcl(cliConfig, fileAbsolutePath, varsSection, 0644)
						if err != nil {
							return err
						}
					} else {
						return fmt.Errorf("invalid '--format' argument '%s'. Valid values are 'json' (default), 'yaml' and 'hcl", format)
					}

					u.LogDebug(cliConfig, fmt.Sprintf("varfile: %s", fileName))
					u.LogDebug(cliConfig, fmt.Sprintf("terraform component: %s", terraformComponent))
					u.LogDebug(cliConfig, fmt.Sprintf("atmos component: %s", componentName))
					u.LogDebug(cliConfig, fmt.Sprintf("atmos stack: %s", contextPrefix))
					u.LogDebug(cliConfig, fmt.Sprintf("stack config file: %s", stackFileName))
				}
			}
		}
	}

	return nil
}
