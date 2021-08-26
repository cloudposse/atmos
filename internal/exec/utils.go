package exec

import (
	c "atmos/internal/config"
	s "atmos/internal/stack"
	u "atmos/internal/utils"
	"errors"
	"fmt"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"os"
	"os/exec"
	"strings"
)

var (
	commonFlags = []string{
		"--stack",
		"-s",
		"--dry-run",
		"--kubeconfig-path",
		"--terraform-dir",
		"--helmfile-dir",
		"--config-dir",
		"--stacks-dir",
	}
)

// Check stack schema and return component info
func checkStackConfig(
	stack string,
	stacksMap map[string]interface{},
	componentType string,
	component string,
) (map[interface{}]interface{}, string, string, error) {

	var stackSection map[interface{}]interface{}
	var componentsSection map[string]interface{}
	var terraformSection map[string]interface{}
	var componentSection map[string]interface{}
	var componentVarsSection map[interface{}]interface{}
	var baseComponent string
	var command string
	var ok bool

	if stackSection, ok = stacksMap[stack].(map[interface{}]interface{}); !ok {
		return nil, "", "", errors.New(fmt.Sprintf("Stack '%s' does not exist", stack))
	}
	if componentsSection, ok = stackSection["components"].(map[string]interface{}); !ok {
		return nil, "", "", errors.New(fmt.Sprintf("'components' section is missing in stack '%s'", stack))
	}
	if terraformSection, ok = componentsSection[componentType].(map[string]interface{}); !ok {
		return nil, "", "", errors.New(fmt.Sprintf("'components/terraform' section is missing in stack '%s'", stack))
	}
	if componentSection, ok = terraformSection[component].(map[string]interface{}); !ok {
		return nil, "", "", errors.New(fmt.Sprintf("Invalid or missing configuration for component '%s' in stack '%s'", component, stack))
	}
	if componentVarsSection, ok = componentSection["vars"].(map[interface{}]interface{}); !ok {
		return nil, "", "", errors.New(fmt.Sprintf("Missing 'vars' section for component '%s' in stack '%s'", component, stack))
	}
	if baseComponent, ok = componentSection["component"].(string); !ok {
		baseComponent = ""
	}
	if command, ok = componentSection["command"].(string); !ok {
		command = ""
	}

	return componentVarsSection, baseComponent, command, nil
}

// processConfigAndStacks processes CLI config and stacks
func processConfigAndStacks(componentType string, cmd *cobra.Command, args []string) (
	stack string,
	componentFromArg string,
	component string,
	baseComponent string,
	command string,
	subCommand string,
	componentVarsSection map[interface{}]interface{},
	additionalArgsAndFlags []string,
	err error,
) {

	if len(args) < 3 {
		return "", "", "", "", "", "", nil, nil,
			errors.New("invalid number of arguments")
	}

	cmd.DisableFlagParsing = false

	err = cmd.ParseFlags(args)
	if err != nil {
		return "",
			"",
			"",
			"",
			"",
			"",
			nil,
			nil,
			err
	}
	flags := cmd.Flags()

	// Get stack
	stack, err = flags.GetString("stack")
	if err != nil {
		return "",
			"",
			"",
			"",
			"",
			"",
			nil,
			nil,
			err
	}

	additionalArgsAndFlags, subCommand, componentFromArg, err = processArgsAndFlags(args)
	if err != nil {
		return "",
			"",
			"",
			"",
			"",
			"",
			nil,
			nil,
			err
	}

	// Process and merge CLI configurations
	err = c.InitConfig(stack)
	if err != nil {
		return "",
			"",
			"",
			"",
			"",
			"",
			nil,
			nil,
			err
	}

	// Process stack config file(s)
	_, stacksMap, err := s.ProcessYAMLConfigFiles(
		c.ProcessedConfig.StacksBaseAbsolutePath,
		c.ProcessedConfig.StackConfigFilesAbsolutePaths,
		false,
		true)

	if err != nil {
		return "",
			"",
			"",
			"",
			"",
			"",
			nil,
			nil,
			err
	}

	// Check if component was provided
	componentFromArg = args[1]
	if len(componentFromArg) < 1 {
		return "",
			"",
			"",
			"",
			"",
			"",
			nil,
			nil,
			errors.New("'component' is required")
	}

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
		return "",
			"",
			"",
			"",
			"",
			"",
			nil,
			nil,
			err
	}

	// Check and process stacks
	if c.ProcessedConfig.StackType == "Directory" {
		componentVarsSection, baseComponent, command, err = checkStackConfig(stack, stacksMap, componentType, componentFromArg)
		if err != nil {
			return "",
				"",
				"",
				"",
				"",
				"",
				nil,
				nil,
				err
		}
	} else {
		color.Cyan("Searching for stack config where the component '%s' is defined\n", componentFromArg)

		if len(c.Config.Stacks.NamePattern) < 1 {
			return "",
				"",
				"",
				"",
				"",
				"",
				nil,
				nil,
				errors.New("stack name pattern must be provided in 'stacks.name_pattern' config or 'ATMOS_STACKS_NAME_PATTERN' ENV variable")
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
			componentVarsSection, baseComponent, command, err = checkStackConfig(stackName, stacksMap, componentType, componentFromArg)
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
				color.Green("Found stack config for component '%s' in stack '%s'\n\n", componentFromArg, stackName)
				stack = stackName
				break
			}
		}

		if tenantFound == false || environmentFound == false || stageFound == false {
			return "",
				"",
				"",
				"",
				"",
				"",
				nil,
				nil,
				errors.New(fmt.Sprintf("\ncould not find config for component '%s' for stack '%s'.\n"+
					"Check that all attributes in the stack name pattern '%s' are defined in stack config files.\n"+
					"Did you forget an import?",
					componentFromArg,
					stack,
					c.Config.Stacks.NamePattern,
				))
		}
	}

	if len(command) > 0 {
		color.Cyan("Found 'command: %s' for component '%s' in stack '%s'\n\n", command, componentFromArg, stack)
	} else {
		command = componentType
	}

	color.Cyan("Variables for component '%s' in stack '%s':", componentFromArg, stack)
	err = u.PrintAsYAML(componentVarsSection)
	if err != nil {
		return "",
			"",
			"",
			"",
			"",
			"",
			nil,
			nil,
			err
	}

	component = componentFromArg
	if len(baseComponent) > 0 {
		component = baseComponent
	}

	return stack,
		componentFromArg,
		component,
		baseComponent,
		command,
		subCommand,
		componentVarsSection,
		additionalArgsAndFlags,
		nil
}

// processArgsAndFlags removes common args and flags from the provided list of arguments/flags
func processArgsAndFlags(argsAndFlags []string) (additionalArgsAndFlags []string, subCommand string, componentFromArg string, err error) {
	// First arg is a terraform/helmfile subcommand
	// Second arg is component
	commonArgsIndexes := []int{0, 1}

	indexesToRemove := []int{}

	for i, arg := range argsAndFlags {
		for _, f := range commonFlags {
			if u.SliceContainsInt(commonArgsIndexes, i) {
				indexesToRemove = append(indexesToRemove, i)
			} else if arg == f {
				indexesToRemove = append(indexesToRemove, i)
				indexesToRemove = append(indexesToRemove, i+1)
			} else if strings.HasPrefix(arg, f+"=") {
				indexesToRemove = append(indexesToRemove, i)
			}
		}
	}

	for i, arg := range argsAndFlags {
		if !u.SliceContainsInt(indexesToRemove, i) {
			additionalArgsAndFlags = append(additionalArgsAndFlags, arg)
		}
	}

	subCommand = argsAndFlags[0]
	componentFromArg = argsAndFlags[1]
	return additionalArgsAndFlags, subCommand, componentFromArg, nil
}

// https://medium.com/rungo/executing-shell-commands-script-files-and-executables-in-go-894814f1c0f7
func execCommand(command string, args []string, dir string) error {
	cmd := exec.Command(command, args...)
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stdout
	return cmd.Run()
}
