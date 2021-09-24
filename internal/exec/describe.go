package exec

import (
	c "atmos/internal/config"
	s "atmos/internal/stack"
	u "atmos/internal/utils"
	"fmt"
	"github.com/fatih/color"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"strings"
)

// ExecuteDescribeComponent executes `describe component` command
func ExecuteDescribeComponent(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return errors.New("invalid arguments. Command requires one argument `component`")
	}
	flags := cmd.Flags()

	stack, err := flags.GetString("stack")
	if err != nil {
		return err
	}

	err = c.InitConfig(stack)
	if err != nil {
		return err
	}

	component := args[0]

	// Print the stack config files
	fmt.Println()
	var msg string
	if c.ProcessedConfig.StackType == "Directory" {
		msg = "Found the config file for the provided stack:"
	} else {
		msg = "Found config files:"
	}
	color.Cyan(msg)

	err = u.PrintAsYAML(c.ProcessedConfig.StackConfigFilesRelativePaths)
	if err != nil {
		return err
	}

	_, stacksMap, err := s.ProcessYAMLConfigFiles(
		c.ProcessedConfig.StacksBaseAbsolutePath,
		c.ProcessedConfig.StackConfigFilesAbsolutePaths,
		false,
		true)

	if err != nil {
		return err
	}

	var componentSection map[string]interface{}
	var componentVarsSection map[interface{}]interface{}

	// Check and process stacks
	if c.ProcessedConfig.StackType == "Directory" {
		componentSection, componentVarsSection, err = findComponentConfig(stack, stacksMap, "terraform", component)
		if err != nil {
			componentSection, componentVarsSection, err = findComponentConfig(stack, stacksMap, "helmfile", component)
			if err != nil {
				return err
			}
		}
	} else {
		color.Cyan("Searching for stack config where the component '%s' is defined\n", component)

		if len(c.Config.Stacks.NamePattern) < 1 {
			return errors.New("stack name pattern must be provided in 'stacks.name_pattern' config or 'ATMOS_STACKS_NAME_PATTERN' ENV variable")
		}

		stackParts := strings.Split(stack, "-")
		stackNamePatternParts := strings.Split(c.Config.Stacks.NamePattern, "-")

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
			componentSection, componentVarsSection, err = findComponentConfig(stackName, stacksMap, "terraform", component)
			if err != nil {
				componentSection, componentVarsSection, err = findComponentConfig(stackName, stacksMap, "helmfile", component)
				if err != nil {
					continue
				}
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
				color.Green("Found stack config for component '%s' in stack '%s'\n\n", component, stackName)
				stack = stackName
				break
			}
		}

		if tenantFound == false || environmentFound == false || stageFound == false {
			return errors.New(fmt.Sprintf("\nCould not find config for component '%s' for stack '%s'.\n"+
				"Check that all attributes in the stack name pattern '%s' are defined in stack config files.\n"+
				"Are the component and stack names correct? Did you forget an import?",
				component,
				stack,
				c.Config.Stacks.NamePattern,
			))
		}
	}

	color.Cyan("\nComponent config:\n\n")
	err = u.PrintAsYAML(componentSection)
	if err != nil {
		return err
	}

	return nil
}
