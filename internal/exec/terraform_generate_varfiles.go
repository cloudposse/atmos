package exec

import (
	"fmt"
	c "github.com/cloudposse/atmos/pkg/config"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
	"path"
	"path/filepath"
	"strings"
)

// ExecuteTerraformGenerateVarfilesCmd executes `terraform generate varfiles` command
func ExecuteTerraformGenerateVarfilesCmd(cmd *cobra.Command, args []string) error {
	Config, err := c.InitCliConfig(c.ConfigAndStacksInfo{})
	if err != nil {
		u.PrintErrorToStdError(err)
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

	return ExecuteTerraformGenerateVarfiles(Config, fileTemplate, format, stacks, components)
}

// ExecuteTerraformGenerateVarfiles generates varfiles for all terraform components in all stacks
func ExecuteTerraformGenerateVarfiles(cliConfig c.CliConfiguration, fileTemplate string, format string, stacks []string, components []string) error {
	stacksMap, err := FindStacksMap(cliConfig)
	if err != nil {
		return err
	}

	fmt.Println()

	var ok bool
	var componentsSection map[string]any
	var terraformSection map[string]any
	var componentSection map[string]any
	var varsSection map[any]any

	for stackConfigFileName, stackSection := range stacksMap {
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
				if componentAttribute, ok := componentSection["component"].(string); ok {
					terraformComponent = componentAttribute
				}

				// Absolute path to the terraform component
				terraformComponentPath := path.Join(
					cliConfig.BasePath,
					cliConfig.Components.Terraform.BasePath,
					terraformComponent,
				)

				// Context
				context := c.GetContextFromVars(varsSection)
				context.Component = strings.Replace(componentName, "/", "-", -1)
				context.ComponentPath = terraformComponentPath
				contextPrefix, err := c.GetContextPrefix(stackConfigFileName, context, cliConfig.Stacks.NamePattern, stackConfigFileName)
				if err != nil {
					return err
				}

				// Check if `stacks` filter is provided
				if len(stacks) == 0 ||
					// `stacks` filter can contain the names of the top-level stack config files:
					// atmos terraform generate varfiles --stacks=orgs/cp/tenant1/staging/us-east-2,orgs/cp/tenant2/dev/us-east-2
					u.SliceContainsString(stacks, stackConfigFileName) ||
					// `stacks` filter can also contain the logical stack names (derived from the context vars):
					// atmos terraform generate varfiles --stacks=tenant1-ue2-staging,tenant1-ue2-prod
					u.SliceContainsString(stacks, contextPrefix) {

					// Replace the tokens in the file template
					// Supported context tokens: {namespace}, {tenant}, {environment}, {region}, {stage}, {component}, {component-path}
					fileName := c.ReplaceContextTokens(context, fileTemplate)
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
						err = u.WriteToFileAsHcl(fileAbsolutePath, varsSection, 0644)
						if err != nil {
							return err
						}
					} else {
						return fmt.Errorf("invalid '--format' argument '%s'. Valid values are 'json' (default), 'yaml' and 'hcl", format)
					}

					u.PrintInfo(fmt.Sprintf("varfile: %s", fileName))
					u.PrintMessage(fmt.Sprintf("terraform component: %s", terraformComponent))
					u.PrintMessage(fmt.Sprintf("atmos component: %s", componentName))
					u.PrintMessage(fmt.Sprintf("atmos stack: %s", contextPrefix))
					u.PrintMessage(fmt.Sprintf("stack config file: %s", stackConfigFileName))
					fmt.Println()
				}
			}
		}
	}

	return nil
}
