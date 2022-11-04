package exec

import (
	"fmt"
	"github.com/spf13/cobra"
	"path"
	"path/filepath"

	cfg "github.com/cloudposse/atmos/pkg/config"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ExecuteTerraformGenerateBackendsCmd executes `terraform generate backends` command
func ExecuteTerraformGenerateBackendsCmd(cmd *cobra.Command, args []string) error {
	info, err := processCommandLineArgs("terraform", cmd, args)
	if err != nil {
		return err
	}

	cliConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		u.PrintErrorToStdError(err)
		return err
	}

	flags := cmd.Flags()

	format, err := flags.GetString("format")
	if err != nil {
		return err
	}
	if format != "" && format != "json" && format != "hcl" {
		return fmt.Errorf("invalid '--format' argument '%s'. Valid values are 'json' and 'hcl", format)
	}
	if format == "" {
		format = "hcl"
	}

	return ExecuteTerraformGenerateBackends(cliConfig, format)
}

// ExecuteTerraformGenerateBackends generates backend configs for all terraform components
func ExecuteTerraformGenerateBackends(cliConfig cfg.CliConfiguration, format string) error {
	stacksMap, err := FindStacksMap(cliConfig)
	if err != nil {
		return err
	}

	fmt.Println()

	var ok bool
	var componentsSection map[string]any
	var terraformSection map[string]any
	var componentSection map[string]any
	var backendSection map[any]any
	var backendType string
	processedTerraformComponents := map[string]any{}

	for _, stackSection := range stacksMap {
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

			// Find terraform component.
			// If `component` attribute is present, it's the terraform component.
			// Otherwise, the YAML component name is the terraform component.
			terraformComponent := componentName
			if componentAttribute, ok := componentSection["component"].(string); ok {
				terraformComponent = componentAttribute
			}

			// If the terraform component has been already processed, continue
			if u.MapKeyExists(processedTerraformComponents, terraformComponent) {
				continue
			}

			processedTerraformComponents[terraformComponent] = terraformComponent

			// Component backend
			if backendSection, ok = componentSection["backend"].(map[any]any); !ok {
				continue
			}

			// Backend type
			if backendType, ok = componentSection["backend_type"].(string); !ok {
				continue
			}

			// Component metadata
			metadataSection := map[any]any{}
			if metadataSection, ok = componentSection["metadata"].(map[any]any); ok {
				if componentType, ok := metadataSection["type"].(string); ok {
					// Don't process abstract components
					if componentType == "abstract" {
						continue
					}
				}
			}

			// Absolute path to the terraform component
			backendFilePath := path.Join(
				cliConfig.BasePath,
				cliConfig.Components.Terraform.BasePath,
				terraformComponent,
				"backend.tf",
			)

			if format == "json" {
				backendFilePath = backendFilePath + ".json"
			}

			backendFileAbsolutePath, err := filepath.Abs(backendFilePath)
			if err != nil {
				return err
			}

			// Write the backend config to the file
			u.PrintMessage(fmt.Sprintf("Writing backend config for the terraform component '%s' to file '%s'", terraformComponent, backendFilePath))

			if format == "json" {
				componentBackendConfig := generateComponentBackendConfig(backendType, backendSection)
				err = u.WriteToFileAsJSON(backendFileAbsolutePath, componentBackendConfig, 0644)
				if err != nil {
					return err
				}
			} else if format == "hcl" {
				err = u.WriteTerraformBackendConfigToFileAsHcl(backendFileAbsolutePath, backendType, backendSection)
				if err != nil {
					return err
				}
			} else {
				return fmt.Errorf("invalid '--format' argument '%s'. Valid values are 'hcl' (default) and 'json", format)
			}
		}
	}

	fmt.Println()

	return nil
}
