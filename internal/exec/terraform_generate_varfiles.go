package exec

import (
	"fmt"
	c "github.com/cloudposse/atmos/pkg/config"
	s "github.com/cloudposse/atmos/pkg/stack"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
	"path"
	"path/filepath"
	"strings"
)

// ExecuteTerraformGenerateVarfilesCmd executes `terraform generate varfiles` command
func ExecuteTerraformGenerateVarfilesCmd(cmd *cobra.Command, args []string) error {
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
	if format != "" && format != "yaml" && format != "json" {
		return fmt.Errorf("invalid '--format' flag '%s'. Valid values are 'json' (default) and 'yaml'", format)
	}
	if format == "" {
		format = "json"
	}

	return ExecuteTerraformGenerateVarfiles(fileTemplate, format, stacks, components)
}

// ExecuteTerraformGenerateVarfiles generates varfiles for all terraform components in all stacks
func ExecuteTerraformGenerateVarfiles(fileTemplate string, format string, stacks []string, components []string) error {
	var configAndStacksInfo c.ConfigAndStacksInfo
	stacksMap, err := FindStacksMap(configAndStacksInfo, false)
	if err != nil {
		return err
	}

	fmt.Println()

	for stackConfigFileName, stackSection := range stacksMap {
		if componentsSection, ok := stackSection.(map[any]any)["components"].(map[string]any); ok {
			if terraformSection, ok := componentsSection["terraform"].(map[string]any); ok {
				for componentName, compSection := range terraformSection {
					if componentSection, ok := compSection.(map[string]any); ok {
						// Find all derived components of the provided components
						derivedComponents, err := s.FindComponentsDerivedFromBaseComponents(stackConfigFileName, terraformSection, components)
						if err != nil {
							return err
						}

						if len(components) == 0 || u.SliceContainsString(components, componentName) || u.SliceContainsString(derivedComponents, componentName) {
							if varsSection, ok := componentSection["vars"].(map[any]any); ok {
								// Find terraform component.
								// If `component` attribute is present, it's the terraform component.
								// Otherwise, the YAML component name is the terraform component.
								terraformComponent := componentName
								if componentAttribute, ok := componentSection["component"].(string); ok {
									terraformComponent = componentAttribute
								}

								// Absolute path to the terraform component
								terraformComponentPath := path.Join(
									c.Config.BasePath,
									c.Config.Components.Terraform.BasePath,
									terraformComponent,
								)

								context := c.GetContextFromVars(varsSection)
								context.Component = strings.Replace(componentName, "/", "-", -1)
								context.ComponentPath = terraformComponentPath
								contextPrefix, err := c.GetContextPrefix(stackConfigFileName, context, c.Config.Stacks.NamePattern, stackConfigFileName)
								if err != nil {
									return err
								}

								// Check if `stacks` filter is provided
								if len(stacks) == 0 ||
									// `stacks` filter can contain the names of the top-level stack config files
									// atmos terraform generate varfiles --stacks=orgs/cp/tenant1/staging/us-east-2,orgs/cp/tenant2/dev/us-east-2
									u.SliceContainsString(stacks, stackConfigFileName) ||
									// `stacks` filter can also contain the stack names (derived from the context vars)
									// atmos terraform generate varfiles --stacks=tenant1-ue2-staging,tenant1-ue2-prod
									u.SliceContainsString(stacks, contextPrefix) {

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
									}

									u.PrintInfo(fmt.Sprintf("Varfile: %s", fileName))
									u.PrintMessage(fmt.Sprintf("Terraform component: %s", terraformComponent))
									u.PrintMessage(fmt.Sprintf("YAML component: %s", componentName))
									u.PrintMessage(fmt.Sprintf("Stack: %s", contextPrefix))
									u.PrintMessage(fmt.Sprintf("Stack config file: %s", stackConfigFileName))
									fmt.Println()
								}
							}
						}
					}
				}
			}
		}
	}

	return nil
}
