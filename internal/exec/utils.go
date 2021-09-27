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

const (
	// Custom flag to specify helmfile `GLOBAL OPTIONS`
	// https://github.com/roboll/helmfile#cli-reference
	globalOptionsFlag = "--global-options"

	terraformDirFlag = "--terraform-dir"
	helmfileDirFlag  = "--helmfile-dir"
	configDirFlag    = "--config-dir"
	stackDirFlag     = "--stacks-dir"
)

var (
	commonFlags = []string{
		"--stack",
		"-s",
		"--dry-run",
		"--kubeconfig-path",
		terraformDirFlag,
		helmfileDirFlag,
		configDirFlag,
		stackDirFlag,
		globalOptionsFlag,
	}
)

// checkStackConfig checks stack schema and return component info
func checkStackConfig(
	stack string,
	stacksMap map[string]interface{},
	componentType string,
	component string,
) (map[string]interface{}, map[interface{}]interface{}, string, string, error) {

	var stackSection map[interface{}]interface{}
	var componentsSection map[string]interface{}
	var componentTypeSection map[string]interface{}
	var componentSection map[string]interface{}
	var componentVarsSection map[interface{}]interface{}
	var baseComponentPath string
	var command string
	var ok bool

	if stackSection, ok = stacksMap[stack].(map[interface{}]interface{}); !ok {
		return nil, nil, "", "", errors.New(fmt.Sprintf("Stack '%s' does not exist", stack))
	}
	if componentsSection, ok = stackSection["components"].(map[string]interface{}); !ok {
		return nil, nil, "", "", errors.New(fmt.Sprintf("'components' section is missing in stack '%s'", stack))
	}
	if componentTypeSection, ok = componentsSection[componentType].(map[string]interface{}); !ok {
		return nil, nil, "", "", errors.New(fmt.Sprintf("'components/%s' section is missing in stack '%s'", componentType, stack))
	}
	if componentSection, ok = componentTypeSection[component].(map[string]interface{}); !ok {
		return nil, nil, "", "", errors.New(fmt.Sprintf("Invalid or missing configuration for component '%s' in stack '%s'", component, stack))
	}
	if componentVarsSection, ok = componentSection["vars"].(map[interface{}]interface{}); !ok {
		return nil, nil, "", "", errors.New(fmt.Sprintf("Missing 'vars' section for component '%s' in stack '%s'", component, stack))
	}
	if baseComponentPath, ok = componentSection["component"].(string); !ok {
		baseComponentPath = ""
	}
	if command, ok = componentSection["command"].(string); !ok {
		command = ""
	}

	return componentSection, componentVarsSection, baseComponentPath, command, nil
}

// findComponentConfig finds component section in config
func findComponentConfig(
	stack string,
	stacksMap map[string]interface{},
	componentType string,
	component string,
) (map[string]interface{}, map[interface{}]interface{}, error) {

	var stackSection map[interface{}]interface{}
	var componentsSection map[string]interface{}
	var componentTypeSection map[string]interface{}
	var componentSection map[string]interface{}
	var componentVarsSection map[interface{}]interface{}
	var ok bool

	if stackSection, ok = stacksMap[stack].(map[interface{}]interface{}); !ok {
		return nil, nil, errors.New(fmt.Sprintf("Stack '%s' does not exist", stack))
	}
	if componentsSection, ok = stackSection["components"].(map[string]interface{}); !ok {
		return nil, nil, errors.New(fmt.Sprintf("'components' section is missing in stack '%s'", stack))
	}
	if componentTypeSection, ok = componentsSection[componentType].(map[string]interface{}); !ok {
		return nil, nil, errors.New(fmt.Sprintf("'components/%s' section is missing in stack '%s'", componentType, stack))
	}
	if componentSection, ok = componentTypeSection[component].(map[string]interface{}); !ok {
		return nil, nil, errors.New(fmt.Sprintf("Invalid or missing configuration for component '%s' in stack '%s'", component, stack))
	}
	if componentVarsSection, ok = componentSection["vars"].(map[interface{}]interface{}); !ok {
		return nil, nil, errors.New(fmt.Sprintf("Missing 'vars' section for component '%s' in stack '%s'", component, stack))
	}

	return componentSection, componentVarsSection, nil
}

// processConfigAndStacks processes CLI config and stacks
func processConfigAndStacks(componentType string, cmd *cobra.Command, args []string) (c.ConfigAndStacksInfo, error) {
	var configAndStacksInfo c.ConfigAndStacksInfo

	if len(args) < 3 {
		return configAndStacksInfo, errors.New("invalid number of arguments")
	}

	cmd.DisableFlagParsing = false

	err := cmd.ParseFlags(args)
	if err != nil {
		return configAndStacksInfo, err
	}
	flags := cmd.Flags()

	// Get stack
	configAndStacksInfo.Stack, err = flags.GetString("stack")
	if err != nil {
		return configAndStacksInfo, err
	}

	argsAndFlagsInfo, err := processArgsAndFlags(args)
	if err != nil {
		return configAndStacksInfo, err
	}

	configAndStacksInfo.AdditionalArgsAndFlags = argsAndFlagsInfo.AdditionalArgsAndFlags
	configAndStacksInfo.SubCommand = argsAndFlagsInfo.SubCommand
	configAndStacksInfo.ComponentFromArg = argsAndFlagsInfo.ComponentFromArg
	configAndStacksInfo.GlobalOptions = argsAndFlagsInfo.GlobalOptions
	configAndStacksInfo.TerraformDir = argsAndFlagsInfo.TerraformDir
	configAndStacksInfo.HelmfileDir = argsAndFlagsInfo.HelmfileDir
	configAndStacksInfo.StacksDir = argsAndFlagsInfo.StacksDir
	configAndStacksInfo.ConfigDir = argsAndFlagsInfo.ConfigDir

	// Process and merge CLI configurations
	err = c.InitConfig(configAndStacksInfo)
	if err != nil {
		return configAndStacksInfo, err
	}

	// Process stack config file(s)
	_, stacksMap, err := s.ProcessYAMLConfigFiles(
		c.ProcessedConfig.StacksBaseAbsolutePath,
		c.ProcessedConfig.StackConfigFilesAbsolutePaths,
		false,
		true)

	if err != nil {
		return configAndStacksInfo, err
	}

	// Check if component was provided
	configAndStacksInfo.ComponentFromArg = args[1]
	if len(configAndStacksInfo.ComponentFromArg) < 1 {
		return configAndStacksInfo, errors.New("'component' is required")
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
		return configAndStacksInfo, err
	}

	// Check and process stacks
	if c.ProcessedConfig.StackType == "Directory" {
		_, configAndStacksInfo.ComponentVarsSection, configAndStacksInfo.BaseComponentPath, configAndStacksInfo.Command, err = checkStackConfig(configAndStacksInfo.Stack,
			stacksMap,
			componentType,
			configAndStacksInfo.ComponentFromArg)
		if err != nil {
			return configAndStacksInfo, err
		}
	} else {
		color.Cyan("Searching for stack config where the component '%s' is defined\n", configAndStacksInfo.ComponentFromArg)

		if len(c.Config.Stacks.NamePattern) < 1 {
			return configAndStacksInfo,
				errors.New("stack name pattern must be provided in 'stacks.name_pattern' config or 'ATMOS_STACKS_NAME_PATTERN' ENV variable")
		}

		stackParts := strings.Split(configAndStacksInfo.Stack, "-")
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
			_, configAndStacksInfo.ComponentVarsSection, configAndStacksInfo.BaseComponentPath, configAndStacksInfo.Command, err = checkStackConfig(stackName,
				stacksMap,
				componentType,
				configAndStacksInfo.ComponentFromArg)
			if err != nil {
				continue
			}

			tenantFound = true
			environmentFound = true
			stageFound = true

			// Search for tenant in stack
			if len(tenant) > 0 {
				if tenantInStack, ok := configAndStacksInfo.ComponentVarsSection["tenant"].(string); !ok || tenantInStack != tenant {
					tenantFound = false
				}
			}

			// Search for environment in stack
			if len(environment) > 0 {
				if environmentInStack, ok := configAndStacksInfo.ComponentVarsSection["environment"].(string); !ok || environmentInStack != environment {
					environmentFound = false
				}
			}

			// Search for stage in stack
			if len(stage) > 0 {
				if stageInStack, ok := configAndStacksInfo.ComponentVarsSection["stage"].(string); !ok || stageInStack != stage {
					stageFound = false
				}
			}

			if tenantFound == true && environmentFound == true && stageFound == true {
				color.Green("Found stack config for component '%s' in stack '%s'\n\n", configAndStacksInfo.ComponentFromArg, stackName)
				configAndStacksInfo.Stack = stackName
				break
			}
		}

		if tenantFound == false || environmentFound == false || stageFound == false {
			return configAndStacksInfo,
				errors.New(fmt.Sprintf("\nCould not find config for component '%s' for stack '%s'.\n"+
					"Check that all attributes in the stack name pattern '%s' are defined in stack config files.\n"+
					"Are the component and stack names correct? Did you forget an import?",
					configAndStacksInfo.ComponentFromArg,
					configAndStacksInfo.Stack,
					c.Config.Stacks.NamePattern,
				))
		}
	}

	if len(configAndStacksInfo.Command) < 1 {
		configAndStacksInfo.Command = componentType
	}

	color.Cyan("Variables for component '%s' in stack '%s':", configAndStacksInfo.ComponentFromArg, configAndStacksInfo.Stack)
	err = u.PrintAsYAML(configAndStacksInfo.ComponentVarsSection)
	if err != nil {
		return configAndStacksInfo, err
	}

	configAndStacksInfo.ComponentFolderPrefix = ""
	configAndStacksInfo.ComponentNamePrefix = ""

	finalComponentPathParts := strings.Split(configAndStacksInfo.ComponentFromArg, "/")
	finalComponentPathPartsLength := len(finalComponentPathParts)

	if finalComponentPathPartsLength > 1 {
		componentFromArgPartsWithoutLast := finalComponentPathParts[:finalComponentPathPartsLength-1]
		configAndStacksInfo.ComponentFolderPrefix = strings.Join(componentFromArgPartsWithoutLast, "/")
		configAndStacksInfo.ComponentFolderPrefix = strings.Join(componentFromArgPartsWithoutLast, "-")
		configAndStacksInfo.Component = finalComponentPathParts[finalComponentPathPartsLength-1]
	} else {
		configAndStacksInfo.Component = configAndStacksInfo.ComponentFromArg
	}

	if len(configAndStacksInfo.BaseComponentPath) > 0 {
		baseComponentPathParts := strings.Split(configAndStacksInfo.BaseComponentPath, "/")
		baseComponentPathPartsLength := len(baseComponentPathParts)
		if baseComponentPathPartsLength > 1 {
			configAndStacksInfo.BaseComponent = baseComponentPathParts[baseComponentPathPartsLength-1]
		} else {
			configAndStacksInfo.BaseComponent = configAndStacksInfo.BaseComponentPath
		}
	}

	return configAndStacksInfo, nil
}

// processArgsAndFlags removes common args and flags from the provided list of arguments/flags
func processArgsAndFlags(inputArgsAndFlags []string) (
	c.ArgsAndFlagsInfo,
	error,
) {
	var info c.ArgsAndFlagsInfo
	subCommand := inputArgsAndFlags[0]
	componentFromArg := inputArgsAndFlags[1]
	var additionalArgsAndFlags []string
	var globalOptions []string

	// First arg is a terraform/helmfile subcommand
	// Second arg is component
	commonArgsIndexes := []int{0, 1}

	var indexesToRemove []int

	// https://github.com/roboll/helmfile#cli-reference
	var globalOptionsFlagIndex int

	for i, arg := range inputArgsAndFlags {
		if arg == globalOptionsFlag {
			globalOptionsFlagIndex = i + 1
		} else if strings.HasPrefix(arg+"=", globalOptionsFlag) {
			globalOptionsFlagIndex = i
		}

		if arg == terraformDirFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, errors.New(fmt.Sprintf("invalid flag: %s", arg))
			}
			info.TerraformDir = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", terraformDirFlag) {
			var terraformDirFlagParts = strings.Split(arg, "=")
			if len(terraformDirFlagParts) != 2 {
				return info, errors.New(fmt.Sprintf("invalid flag: %s", arg))
			}
			info.TerraformDir = terraformDirFlagParts[1]
		}

		if arg == helmfileDirFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, errors.New(fmt.Sprintf("invalid flag: %s", arg))
			}
			info.HelmfileDir = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", helmfileDirFlag) {
			var helmfileDirFlagParts = strings.Split(arg, "=")
			if len(helmfileDirFlagParts) != 2 {
				return info, errors.New(fmt.Sprintf("invalid flag: %s", arg))
			}
			info.HelmfileDir = helmfileDirFlagParts[1]
		}

		if arg == configDirFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, errors.New(fmt.Sprintf("invalid flag: %s", arg))
			}
			info.StacksDir = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", configDirFlag) {
			var configDirFlagParts = strings.Split(arg, "=")
			if len(configDirFlagParts) != 2 {
				return info, errors.New(fmt.Sprintf("invalid flag: %s", arg))
			}
			info.StacksDir = configDirFlagParts[1]
		}

		if arg == stackDirFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, errors.New(fmt.Sprintf("invalid flag: %s", arg))
			}
			info.ConfigDir = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", stackDirFlag) {
			var stacksDirFlagParts = strings.Split(arg, "=")
			if len(stacksDirFlagParts) != 2 {
				return info, errors.New(fmt.Sprintf("invalid flag: %s", arg))
			}
			info.ConfigDir = stacksDirFlagParts[1]
		}

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

	for i, arg := range inputArgsAndFlags {
		if !u.SliceContainsInt(indexesToRemove, i) {
			additionalArgsAndFlags = append(additionalArgsAndFlags, arg)
		}

		if globalOptionsFlagIndex > 0 && i == globalOptionsFlagIndex {
			if strings.HasPrefix(arg, globalOptionsFlag+"=") {
				parts := strings.SplitN(arg, "=", 2)
				globalOptions = strings.Split(parts[1], " ")
			} else {
				globalOptions = strings.Split(arg, " ")
			}
		}
	}

	info.AdditionalArgsAndFlags = additionalArgsAndFlags
	info.SubCommand = subCommand
	info.ComponentFromArg = componentFromArg
	info.GlobalOptions = globalOptions

	return info, nil
}

// execCommand prints and executes the provided command with args and flags
func execCommand(command string, args []string, dir string, env []string) error {
	cmd := exec.Command(command, args...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stdout
	color.Cyan("\nExecuting command:\n%s\n\n", cmd.String())
	return cmd.Run()
}

func getContextFromVars(vars map[interface{}]interface{}) c.Context {
	var context c.Context

	if namespace, ok := vars["namespace"].(string); ok {
		context.Namespace = namespace
	}

	if tenant, ok := vars["tenant"].(string); ok {
		context.Tenant = tenant
	}

	if environment, ok := vars["environment"].(string); ok {
		context.Environment = environment
	}

	if stage, ok := vars["stage"].(string); ok {
		context.Stage = stage
	}

	if region, ok := vars["region"].(string); ok {
		context.Region = region
	}

	return context
}

func replaceContextTokens(context c.Context, pattern string) string {
	return strings.Replace(
		strings.Replace(
			strings.Replace(
				strings.Replace(pattern,
					"{namespace}", context.Namespace, 1),
				"{environment}", context.Environment, 1),
			"{tenant}", context.Tenant, 1),
		"{stage}", context.Stage, 1)
}
