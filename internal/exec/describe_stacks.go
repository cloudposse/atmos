package exec

import (
	"fmt"
	"github.com/spf13/cobra"
	"strings"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	s "github.com/cloudposse/atmos/pkg/stack"
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

	finalStacksMap, err := ExecuteDescribeStacks(cliConfig, filterByStack, components, componentTypes, sections, false)
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
) (map[string]any, error) {

	stacksMap, _, err := FindStacksMap(cliConfig, ignoreMissingFiles)
	if err != nil {
		return nil, err
	}

	finalStacksMap := make(map[string]any)
	var varsSection map[any]any
	var metadataSection map[any]any
	var stackName string
	context := schema.Context{}

	for stackFileName, stackSection := range stacksMap {
		// Delete the stack-wide imports
		delete(stackSection.(map[any]any), "imports")

		if componentsSection, ok := stackSection.(map[any]any)["components"].(map[string]any); ok {

			if len(componentTypes) == 0 || u.SliceContainsString(componentTypes, "terraform") {
				if terraformSection, ok := componentsSection["terraform"].(map[string]any); ok {
					for componentName, compSection := range terraformSection {
						componentSection, ok := compSection.(map[string]any)
						if !ok {
							return nil, fmt.Errorf("invalid 'components.terraform.%s' section in the file '%s'", componentName, stackFileName)
						}

						if comp, ok := componentSection["component"].(string); !ok || comp == "" {
							componentSection["component"] = componentName
						}

						// Find all derived components of the provided components and include them in the output
						derivedComponents, err := s.FindComponentsDerivedFromBaseComponents(stackFileName, terraformSection, components)
						if err != nil {
							return nil, err
						}

						// Component vars
						if varsSection, ok = componentSection["vars"].(map[any]any); ok {
							context = cfg.GetContextFromVars(varsSection)
							stackName, err = cfg.GetContextPrefix(stackFileName, context, cliConfig.Stacks.NamePattern, stackFileName)
							if err != nil {
								return nil, err
							}
						}

						// Component metadata
						if metadataSection, ok = componentSection["metadata"].(map[any]any); !ok {
							metadataSection = map[any]any{}
						}

						if filterByStack != "" && filterByStack != stackFileName && filterByStack != stackName {
							continue
						}

						if stackName == "" {
							stackName = stackFileName
						}

						if !u.MapKeyExists(finalStacksMap, stackName) {
							finalStacksMap[stackName] = make(map[string]any)
						}

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

							for sectionName, section := range componentSection {
								if len(sections) == 0 || u.SliceContainsString(sections, sectionName) {
									finalStacksMap[stackName].(map[string]any)["components"].(map[string]any)["terraform"].(map[string]any)[componentName].(map[string]any)[sectionName] = section
								}

								// Terraform workspace
								if len(sections) == 0 || u.SliceContainsString(sections, "workspace") {
									workspace, err := BuildTerraformWorkspace(
										stackName,
										cliConfig.Stacks.NamePattern,
										metadataSection,
										context,
									)
									if err != nil {
										return nil, err
									}

									finalStacksMap[stackName].(map[string]any)["components"].(map[string]any)["terraform"].(map[string]any)[componentName].(map[string]any)["workspace"] = workspace
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

						if comp, ok := componentSection["component"].(string); !ok || comp == "" {
							componentSection["component"] = componentName
						}

						// Find all derived components of the provided components and include them in the output
						derivedComponents, err := s.FindComponentsDerivedFromBaseComponents(stackFileName, helmfileSection, components)
						if err != nil {
							return nil, err
						}

						// Component vars
						if varsSection, ok = componentSection["vars"].(map[any]any); ok {
							context := cfg.GetContextFromVars(varsSection)
							stackName, err = cfg.GetContextPrefix(stackFileName, context, cliConfig.Stacks.NamePattern, stackFileName)
							if err != nil {
								return nil, err
							}
						}

						if filterByStack != "" && filterByStack != stackFileName && filterByStack != stackName {
							continue
						}

						if stackName == "" {
							stackName = stackFileName
						}

						if !u.MapKeyExists(finalStacksMap, stackName) {
							finalStacksMap[stackName] = make(map[string]any)
						}

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

		// Filter out empty stacks (stacks without any components)
		if st, ok := finalStacksMap[stackName].(map[string]any); ok {
			if len(st) == 0 {
				delete(finalStacksMap, stackName)
			}
		}
	}

	return finalStacksMap, nil
}
