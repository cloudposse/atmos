package exec

import (
	"fmt"
	"github.com/cloudposse/atmos/pkg/config"
	g "github.com/cloudposse/atmos/pkg/globals"
	s "github.com/cloudposse/atmos/pkg/stack"
	"github.com/cloudposse/atmos/pkg/utils"
	"github.com/fatih/color"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"path"
	"strings"
)

// ExecuteTerraformGenerateBackend executes `terraform generate backend` command
func ExecuteTerraformGenerateBackend(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return errors.New("invalid arguments. The command requires one argument `component`")
	}
	flags := cmd.Flags()

	stack, err := flags.GetString("stack")
	if err != nil {
		return err
	}

	var configAndStacksInfo config.ConfigAndStacksInfo
	configAndStacksInfo.Stack = stack

	err = config.InitConfig()
	if err != nil {
		return err
	}

	err = config.ProcessConfig(configAndStacksInfo)
	if err != nil {
		return err
	}

	component := args[0]

	// Print the stack config files
	if g.LogVerbose {
		fmt.Println()
		var msg string
		if config.ProcessedConfig.StackType == "Directory" {
			msg = "Found the config file for the provided stack:"
		} else {
			msg = "Found config files:"
		}
		color.Cyan(msg)

		err = utils.PrintAsYAML(config.ProcessedConfig.StackConfigFilesRelativePaths)
		if err != nil {
			return err
		}
	}

	_, stacksMap, err := s.ProcessYAMLConfigFiles(
		config.ProcessedConfig.StacksBaseAbsolutePath,
		config.ProcessedConfig.StackConfigFilesAbsolutePaths,
		false,
		false)

	if err != nil {
		return err
	}

	var componentSection map[string]interface{}
	var componentVarsSection map[interface{}]interface{}
	var componentBackendSection map[interface{}]interface{}
	var componentBackendType string

	// Check and process stacks
	if config.ProcessedConfig.StackType == "Directory" {
		componentSection,
			componentVarsSection,
			_,
			componentBackendSection,
			componentBackendType,
			_, _, _, _,
			err = findComponentConfig(stack, stacksMap, "terraform", component)
		if err != nil {
			return err
		}
	} else {
		if g.LogVerbose == true {
			color.Cyan("Searching for stack config where the component '%s' is defined\n", component)
		}

		if len(config.Config.Stacks.NamePattern) < 1 {
			return errors.New("stack name pattern must be provided in 'stacks.name_pattern' config or 'ATMOS_STACKS_NAME_PATTERN' ENV variable")
		}

		stackParts := strings.Split(stack, "-")
		stackNamePatternParts := strings.Split(config.Config.Stacks.NamePattern, "-")

		var tenant string
		var environment string
		var stage string
		var tenantFound bool
		var environmentFound bool
		var stageFound bool

		for i, part := range stackNamePatternParts {
			if part == "{tenant}" {
				tenant = stackParts[i]
			} else if part == "{environment}" {
				environment = stackParts[i]
			} else if part == "{stage}" {
				stage = stackParts[i]
			}
		}

		for stackName := range stacksMap {
			componentSection,
				componentVarsSection,
				_,
				componentBackendSection,
				componentBackendType,
				_, _, _, _,
				err = findComponentConfig(stackName, stacksMap, "terraform", component)
			if err != nil {
				continue
			}

			tenantFound = true
			environmentFound = true
			stageFound = true

			// Search for tenant in stack
			if len(tenant) > 0 {
				if tenantInStack, ok := componentVarsSection["tenant"].(string); !ok || tenantInStack != tenant {
					tenantFound = false
				}
			}

			// Search for environment in stack
			if len(environment) > 0 {
				if environmentInStack, ok := componentVarsSection["environment"].(string); !ok || environmentInStack != environment {
					environmentFound = false
				}
			}

			// Search for stage in stack
			if len(stage) > 0 {
				if stageInStack, ok := componentVarsSection["stage"].(string); !ok || stageInStack != stage {
					stageFound = false
				}
			}

			if tenantFound == true && environmentFound == true && stageFound == true {
				if g.LogVerbose == true {
					color.Green("Found stack config for the '%s' component in the '%s' stack\n\n", component, stackName)
				}
				stack = stackName
				break
			}
		}

		if tenantFound == false || environmentFound == false || stageFound == false {
			return errors.New(fmt.Sprintf("\nCould not find config for the '%s' component in the '%s' stack.\n"+
				"Check that all attributes in the stack name pattern '%s' are defined in stack config files.\n"+
				"Are the component and stack names correct? Did you forget an import?",
				component,
				stack,
				config.Config.Stacks.NamePattern,
			))
		}
	}

	if componentBackendType == "" {
		return errors.New(fmt.Sprintf("\n'backend_type' is missing for the '%s' component.\n", component))
	}

	if componentBackendSection == nil {
		return errors.New(fmt.Sprintf("\nCould not find 'backend' config for the '%s' component.\n", component))
	}

	var componentBackendConfig = generateComponentBackendConfig(componentBackendType, componentBackendSection)

	fmt.Println()
	color.Cyan("Component backend config:\n\n")
	err = utils.PrintAsJSON(componentBackendConfig)
	if err != nil {
		return err
	}

	// Check if the `backend` section has `workspace_key_prefix`
	if _, ok := componentBackendSection["workspace_key_prefix"].(string); !ok {
		return errors.New(fmt.Sprintf("\nBackend config for the '%s' component is missing 'workspace_key_prefix'\n", component))
	}

	// Find if the component has a base component
	var baseComponent string
	if baseComponentSection, ok := componentSection["component"].(string); ok {
		baseComponent = baseComponentSection
	}

	var finalComponent string
	if len(baseComponent) > 0 {
		finalComponent = baseComponent
	} else {
		finalComponent = component
	}

	// Write backend config to file
	var backendFileName = path.Join(
		config.Config.Components.Terraform.BasePath,
		finalComponent,
		"backend.tf.json",
	)

	fmt.Println()
	color.Cyan("Writing backend config to file:")
	fmt.Println(backendFileName)
	err = utils.WriteToFileAsJSON(backendFileName, componentBackendConfig, 0644)
	if err != nil {
		return err
	}

	fmt.Println()
	return nil
}

// ExecuteTerraformGenerateBackends executes `terraform generate backends` command
func ExecuteTerraformGenerateBackends(cmd *cobra.Command, args []string) error {
	return nil
}
