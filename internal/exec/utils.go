package exec

import (
	"errors"
	"fmt"
	c "github.com/cloudposse/atmos/pkg/config"
	g "github.com/cloudposse/atmos/pkg/globals"
	s "github.com/cloudposse/atmos/pkg/stack"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
	"strings"
)

var (
	commonFlags = []string{
		"--stack",
		"-s",
		g.DryRunFlag,
		g.KubeConfigConfigFlag,
		g.TerraformDirFlag,
		g.HelmfileDirFlag,
		g.ConfigDirFlag,
		g.StackDirFlag,
		g.BasePathFlag,
		g.GlobalOptionsFlag,
		g.DeployRunInitFlag,
		g.InitRunReconfigure,
		g.AutoGenerateBackendFileFlag,
		g.FromPlanFlag,
		g.HelpFlag1,
		g.HelpFlag2,
	}
)

// FindComponentConfig finds component config sections
func FindComponentConfig(
	stack string,
	stacksMap map[string]interface{},
	componentType string,
	component string,
) (map[string]interface{},
	map[interface{}]interface{},
	map[interface{}]interface{},
	map[interface{}]interface{},
	string,
	string,
	string,
	[]string,
	bool,
	map[interface{}]interface{},
	error,
) {

	var stackSection map[interface{}]interface{}
	var componentsSection map[string]interface{}
	var componentTypeSection map[string]interface{}
	var componentSection map[string]interface{}
	var componentVarsSection map[interface{}]interface{}
	var componentEnvSection map[interface{}]interface{}
	var componentBackendSection map[interface{}]interface{}
	var componentBackendType string
	var baseComponentName string
	var command string
	var componentInheritanceChain []string
	var componentIsAbstract bool
	var componentMetadata map[interface{}]interface{}
	var ok bool

	if len(stack) == 0 {
		return nil, nil, nil, nil, "", "", "", nil, false, nil, errors.New("stack must be provided and must not be empty")
	}
	if len(component) == 0 {
		return nil, nil, nil, nil, "", "", "", nil, false, nil, errors.New("component must be provided and must not be empty")
	}
	if len(componentType) == 0 {
		return nil, nil, nil, nil, "", "", "", nil, false, nil, errors.New("component type must be provided and must not be empty")
	}
	if stackSection, ok = stacksMap[stack].(map[interface{}]interface{}); !ok {
		return nil, nil, nil, nil, "", "", "", nil, false, nil, errors.New(fmt.Sprintf("Could not find the stack '%s'", stack))
	}
	if componentsSection, ok = stackSection["components"].(map[string]interface{}); !ok {
		return nil, nil, nil, nil, "", "", "", nil, false, nil, errors.New(fmt.Sprintf("'components' section is missing in the stack '%s'", stack))
	}
	if componentTypeSection, ok = componentsSection[componentType].(map[string]interface{}); !ok {
		return nil, nil, nil, nil, "", "", "", nil, false, nil, errors.New(fmt.Sprintf("'components/%s' section is missing in the stack '%s'", componentType, stack))
	}
	if componentSection, ok = componentTypeSection[component].(map[string]interface{}); !ok {
		return nil, nil, nil, nil, "", "", "", nil, false, nil, errors.New(fmt.Sprintf("Invalid or missing configuration for the component '%s' in the stack '%s'", component, stack))
	}
	if componentVarsSection, ok = componentSection["vars"].(map[interface{}]interface{}); !ok {
		return nil, nil, nil, nil, "", "", "", nil, false, nil, errors.New(fmt.Sprintf("Missing 'vars' section for the component '%s' in the stack '%s'", component, stack))
	}
	if componentBackendSection, ok = componentSection["backend"].(map[interface{}]interface{}); !ok {
		componentBackendSection = nil
	}
	if componentBackendType, ok = componentSection["backend_type"].(string); !ok {
		componentBackendType = ""
	}
	if command, ok = componentSection["command"].(string); !ok {
		command = ""
	}
	if componentEnvSection, ok = componentSection["env"].(map[interface{}]interface{}); !ok {
		componentEnvSection = map[interface{}]interface{}{}
	}
	if componentInheritanceChain, ok = componentSection["inheritance"].([]string); !ok {
		componentInheritanceChain = []string{}
	}
	if baseComponentName, ok = componentSection["component"].(string); !ok {
		baseComponentName = ""
	}
	if componentMetadataSection, componentMetadataSectionExists := componentSection["metadata"]; componentMetadataSectionExists {
		componentMetadata = componentMetadataSection.(map[interface{}]interface{})
		if componentMetadataType, componentMetadataTypeAttributeExists := componentMetadata["type"].(string); componentMetadataTypeAttributeExists {
			if componentMetadataType == "abstract" {
				componentIsAbstract = true
			}
		}
		if componentMetadataComponent, componentMetadataComponentExists := componentMetadata["component"].(string); componentMetadataComponentExists {
			baseComponentName = componentMetadataComponent
		}
	}

	if component == baseComponentName {
		baseComponentName = ""
	}

	return componentSection,
		componentVarsSection,
		componentEnvSection,
		componentBackendSection,
		componentBackendType,
		baseComponentName,
		command,
		componentInheritanceChain,
		componentIsAbstract,
		componentMetadata,
		nil
}

// processArgsConfigAndStacks processes command-line args, CLI config and stacks
func processArgsConfigAndStacks(componentType string, cmd *cobra.Command, args []string) (c.ConfigAndStacksInfo, error) {
	var configAndStacksInfo c.ConfigAndStacksInfo

	if len(args) < 1 {
		return configAndStacksInfo, errors.New("invalid number of arguments")
	}

	cmd.DisableFlagParsing = false

	err := cmd.ParseFlags(args)
	if err != nil {
		return configAndStacksInfo, err
	}

	argsAndFlagsInfo, err := processArgsAndFlags(componentType, args)
	if err != nil {
		return configAndStacksInfo, err
	}

	configAndStacksInfo.AdditionalArgsAndFlags = argsAndFlagsInfo.AdditionalArgsAndFlags
	configAndStacksInfo.SubCommand = argsAndFlagsInfo.SubCommand
	configAndStacksInfo.SubCommand2 = argsAndFlagsInfo.SubCommand2
	configAndStacksInfo.ComponentType = componentType
	configAndStacksInfo.ComponentFromArg = argsAndFlagsInfo.ComponentFromArg
	configAndStacksInfo.GlobalOptions = argsAndFlagsInfo.GlobalOptions
	configAndStacksInfo.BasePath = argsAndFlagsInfo.BasePath
	configAndStacksInfo.TerraformDir = argsAndFlagsInfo.TerraformDir
	configAndStacksInfo.HelmfileDir = argsAndFlagsInfo.HelmfileDir
	configAndStacksInfo.StacksDir = argsAndFlagsInfo.StacksDir
	configAndStacksInfo.ConfigDir = argsAndFlagsInfo.ConfigDir
	configAndStacksInfo.WorkflowsDir = argsAndFlagsInfo.WorkflowsDir
	configAndStacksInfo.DeployRunInit = argsAndFlagsInfo.DeployRunInit
	configAndStacksInfo.InitRunReconfigure = argsAndFlagsInfo.InitRunReconfigure
	configAndStacksInfo.AutoGenerateBackendFile = argsAndFlagsInfo.AutoGenerateBackendFile
	configAndStacksInfo.UseTerraformPlan = argsAndFlagsInfo.UseTerraformPlan
	configAndStacksInfo.DryRun = argsAndFlagsInfo.DryRun
	configAndStacksInfo.NeedHelp = argsAndFlagsInfo.NeedHelp

	// Check if `-h` or `--help` flags are specified
	if argsAndFlagsInfo.NeedHelp == true {
		err = processHelp(componentType, argsAndFlagsInfo.SubCommand)
		if err != nil {
			return configAndStacksInfo, err
		}
		return configAndStacksInfo, nil
	}

	flags := cmd.Flags()

	configAndStacksInfo.Stack, err = flags.GetString("stack")
	if err != nil {
		return configAndStacksInfo, err
	}

	return ProcessStacks(configAndStacksInfo, true)
}

// FindStacksMap processes stack config and returns a map of all stacks
func FindStacksMap(configAndStacksInfo c.ConfigAndStacksInfo, checkStack bool) (map[string]interface{}, error) {
	// Process and merge CLI configurations
	err := c.InitConfig()
	if err != nil {
		return nil, err
	}

	err = c.ProcessConfig(configAndStacksInfo, checkStack)
	if err != nil {
		return nil, err
	}

	// Process stack config file(s)
	_, stacksMap, err := s.ProcessYAMLConfigFiles(
		c.ProcessedConfig.StacksBaseAbsolutePath,
		c.ProcessedConfig.StackConfigFilesAbsolutePaths,
		false,
		true)

	if err != nil {
		return nil, err
	}

	return stacksMap, nil
}

// ProcessStacks processes stack config
func ProcessStacks(configAndStacksInfo c.ConfigAndStacksInfo, checkStack bool) (c.ConfigAndStacksInfo, error) {
	// Check if stack was provided
	if checkStack && len(configAndStacksInfo.Stack) < 1 {
		message := fmt.Sprintf("'stack' is required. Usage: atmos %s <command> <component> -s <stack>", configAndStacksInfo.ComponentType)
		return configAndStacksInfo, errors.New(message)
	}

	// Check if component was provided
	if len(configAndStacksInfo.ComponentFromArg) < 1 {
		message := fmt.Sprintf("'component' is required. Usage: atmos %s <command> <component> <arguments_and_flags>", configAndStacksInfo.ComponentType)
		return configAndStacksInfo, errors.New(message)
	}

	configAndStacksInfo.StackFromArg = configAndStacksInfo.Stack

	stacksMap, err := FindStacksMap(configAndStacksInfo, checkStack)
	if err != nil {
		return configAndStacksInfo, err
	}

	// Print the stack config files
	if g.LogVerbose {
		fmt.Println()
		var msg string
		if c.ProcessedConfig.StackType == "Directory" {
			msg = "Found the config file for the provided stack:"
		} else {
			msg = "Found config files:"
		}
		u.PrintInfo(msg)
		err = u.PrintAsYAML(c.ProcessedConfig.StackConfigFilesRelativePaths)
		if err != nil {
			return configAndStacksInfo, err
		}
	}

	// Check and process stacks
	if c.ProcessedConfig.StackType == "Directory" {
		configAndStacksInfo.ComponentSection,
			configAndStacksInfo.ComponentVarsSection,
			configAndStacksInfo.ComponentEnvSection,
			configAndStacksInfo.ComponentBackendSection,
			configAndStacksInfo.ComponentBackendType,
			configAndStacksInfo.BaseComponentPath,
			configAndStacksInfo.Command,
			configAndStacksInfo.ComponentInheritanceChain,
			configAndStacksInfo.ComponentIsAbstract,
			configAndStacksInfo.ComponentMetadataSection,
			err = FindComponentConfig(configAndStacksInfo.Stack, stacksMap, configAndStacksInfo.ComponentType, configAndStacksInfo.ComponentFromArg)
		if err != nil {
			return configAndStacksInfo, err
		}

		configAndStacksInfo.ComponentEnvList = convertEnvVars(configAndStacksInfo.ComponentEnvSection)

		// Process context
		configAndStacksInfo.Context = c.GetContextFromVars(configAndStacksInfo.ComponentVarsSection)
		configAndStacksInfo.Context.Component = configAndStacksInfo.ComponentFromArg
		configAndStacksInfo.Context.BaseComponent = configAndStacksInfo.BaseComponentPath
		configAndStacksInfo.ContextPrefix, err = c.GetContextPrefix(configAndStacksInfo.Stack, configAndStacksInfo.Context, c.Config.Stacks.NamePattern)
		if err != nil {
			return configAndStacksInfo, err
		}
	} else {
		u.PrintInfoVerbose(fmt.Sprintf("Searching for stack config where the component '%s' is defined\n", configAndStacksInfo.ComponentFromArg))

		stackFound := false

		for stackName := range stacksMap {
			configAndStacksInfo.ComponentSection,
				configAndStacksInfo.ComponentVarsSection,
				configAndStacksInfo.ComponentEnvSection,
				configAndStacksInfo.ComponentBackendSection,
				configAndStacksInfo.ComponentBackendType,
				configAndStacksInfo.BaseComponentPath,
				configAndStacksInfo.Command,
				configAndStacksInfo.ComponentInheritanceChain,
				configAndStacksInfo.ComponentIsAbstract,
				configAndStacksInfo.ComponentMetadataSection,
				err = FindComponentConfig(stackName, stacksMap, configAndStacksInfo.ComponentType, configAndStacksInfo.ComponentFromArg)
			if err != nil {
				continue
			}

			configAndStacksInfo.ComponentEnvList = convertEnvVars(configAndStacksInfo.ComponentEnvSection)

			// Process context
			configAndStacksInfo.Context = c.GetContextFromVars(configAndStacksInfo.ComponentVarsSection)
			configAndStacksInfo.Context.Component = configAndStacksInfo.ComponentFromArg
			configAndStacksInfo.Context.BaseComponent = configAndStacksInfo.BaseComponentPath
			configAndStacksInfo.ContextPrefix, err = c.GetContextPrefix(configAndStacksInfo.Stack, configAndStacksInfo.Context, c.Config.Stacks.NamePattern)
			if err != nil {
				return configAndStacksInfo, err
			}

			// Check if we've found the stack
			if configAndStacksInfo.Stack == configAndStacksInfo.ContextPrefix {
				stackFound = true
				u.PrintInfoVerbose(fmt.Sprintf("Found config for the component '%s' for the stack '%s' in the file '%s'\n",
					configAndStacksInfo.ComponentFromArg,
					configAndStacksInfo.Stack,
					stackName,
				))
				break
			}
		}

		if !stackFound {
			return configAndStacksInfo,
				errors.New(fmt.Sprintf("\nSearched all stack files, but could not find config for the component '%s' in the stack '%s'.\n"+
					"Check that all attributes in the stack name pattern '%s' are defined in the stack config files.\n"+
					"Are the component and stack names correct? Did you forget an import?",
					configAndStacksInfo.ComponentFromArg,
					configAndStacksInfo.Stack,
					c.Config.Stacks.NamePattern,
				))
		}
	}

	if len(configAndStacksInfo.Command) == 0 {
		configAndStacksInfo.Command = configAndStacksInfo.ComponentType
	}

	// Process component path and name
	configAndStacksInfo.ComponentFolderPrefix = ""
	componentPathParts := strings.Split(configAndStacksInfo.ComponentFromArg, "/")
	componentPathPartsLength := len(componentPathParts)
	if componentPathPartsLength > 1 {
		componentFromArgPartsWithoutLast := componentPathParts[:componentPathPartsLength-1]
		configAndStacksInfo.ComponentFolderPrefix = strings.Join(componentFromArgPartsWithoutLast, "/")
		configAndStacksInfo.Component = componentPathParts[componentPathPartsLength-1]
	} else {
		configAndStacksInfo.Component = configAndStacksInfo.ComponentFromArg
	}

	// Process base component path and name
	if len(configAndStacksInfo.BaseComponentPath) > 0 {
		baseComponentPathParts := strings.Split(configAndStacksInfo.BaseComponentPath, "/")
		baseComponentPathPartsLength := len(baseComponentPathParts)
		if baseComponentPathPartsLength > 1 {
			baseComponentFromArgPartsWithoutLast := baseComponentPathParts[:componentPathPartsLength-1]
			configAndStacksInfo.ComponentFolderPrefix = strings.Join(baseComponentFromArgPartsWithoutLast, "/")
			configAndStacksInfo.BaseComponent = baseComponentPathParts[baseComponentPathPartsLength-1]
		} else {
			configAndStacksInfo.ComponentFolderPrefix = ""
			configAndStacksInfo.BaseComponent = configAndStacksInfo.BaseComponentPath
		}
	}

	// Get the final component
	if len(configAndStacksInfo.BaseComponent) > 0 {
		configAndStacksInfo.FinalComponent = configAndStacksInfo.BaseComponent
	} else {
		configAndStacksInfo.FinalComponent = configAndStacksInfo.Component
	}

	// workspace
	workspace, err := BuildTerraformWorkspace(
		configAndStacksInfo.Stack,
		c.Config.Stacks.NamePattern,
		configAndStacksInfo.ComponentMetadataSection,
		configAndStacksInfo.Context,
	)
	if err != nil {
		return configAndStacksInfo, err
	}
	configAndStacksInfo.TerraformWorkspace = workspace
	configAndStacksInfo.ComponentSection["workspace"] = workspace

	return configAndStacksInfo, nil
}

// processArgsAndFlags removes common args and flags from the provided list of arguments/flags
func processArgsAndFlags(componentType string, inputArgsAndFlags []string) (c.ArgsAndFlagsInfo, error) {
	var info c.ArgsAndFlagsInfo
	var additionalArgsAndFlags []string
	var globalOptions []string

	var indexesToRemove []int

	// https://github.com/roboll/helmfile#cli-reference
	var globalOptionsFlagIndex int

	for i, arg := range inputArgsAndFlags {
		if arg == g.GlobalOptionsFlag {
			globalOptionsFlagIndex = i + 1
		} else if strings.HasPrefix(arg+"=", g.GlobalOptionsFlag) {
			globalOptionsFlagIndex = i
		}

		if arg == g.TerraformDirFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, errors.New(fmt.Sprintf("invalid flag: %s", arg))
			}
			info.TerraformDir = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", g.TerraformDirFlag) {
			var terraformDirFlagParts = strings.Split(arg, "=")
			if len(terraformDirFlagParts) != 2 {
				return info, errors.New(fmt.Sprintf("invalid flag: %s", arg))
			}
			info.TerraformDir = terraformDirFlagParts[1]
		}

		if arg == g.HelmfileDirFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, errors.New(fmt.Sprintf("invalid flag: %s", arg))
			}
			info.HelmfileDir = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", g.HelmfileDirFlag) {
			var helmfileDirFlagParts = strings.Split(arg, "=")
			if len(helmfileDirFlagParts) != 2 {
				return info, errors.New(fmt.Sprintf("invalid flag: %s", arg))
			}
			info.HelmfileDir = helmfileDirFlagParts[1]
		}

		if arg == g.ConfigDirFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, errors.New(fmt.Sprintf("invalid flag: %s", arg))
			}
			info.StacksDir = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", g.ConfigDirFlag) {
			var configDirFlagParts = strings.Split(arg, "=")
			if len(configDirFlagParts) != 2 {
				return info, errors.New(fmt.Sprintf("invalid flag: %s", arg))
			}
			info.StacksDir = configDirFlagParts[1]
		}

		if arg == g.StackDirFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, errors.New(fmt.Sprintf("invalid flag: %s", arg))
			}
			info.ConfigDir = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", g.StackDirFlag) {
			var stacksDirFlagParts = strings.Split(arg, "=")
			if len(stacksDirFlagParts) != 2 {
				return info, errors.New(fmt.Sprintf("invalid flag: %s", arg))
			}
			info.ConfigDir = stacksDirFlagParts[1]
		}

		if arg == g.BasePathFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, errors.New(fmt.Sprintf("invalid flag: %s", arg))
			}
			info.BasePath = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", g.BasePathFlag) {
			var stacksDirFlagParts = strings.Split(arg, "=")
			if len(stacksDirFlagParts) != 2 {
				return info, errors.New(fmt.Sprintf("invalid flag: %s", arg))
			}
			info.BasePath = stacksDirFlagParts[1]
		}

		if arg == g.DeployRunInitFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, errors.New(fmt.Sprintf("invalid flag: %s", arg))
			}
			info.DeployRunInit = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", g.DeployRunInitFlag) {
			var deployRunInitFlagParts = strings.Split(arg, "=")
			if len(deployRunInitFlagParts) != 2 {
				return info, errors.New(fmt.Sprintf("invalid flag: %s", arg))
			}
			info.DeployRunInit = deployRunInitFlagParts[1]
		}

		if arg == g.AutoGenerateBackendFileFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, errors.New(fmt.Sprintf("invalid flag: %s", arg))
			}
			info.AutoGenerateBackendFile = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", g.AutoGenerateBackendFileFlag) {
			var autoGenerateBackendFileFlagParts = strings.Split(arg, "=")
			if len(autoGenerateBackendFileFlagParts) != 2 {
				return info, errors.New(fmt.Sprintf("invalid flag: %s", arg))
			}
			info.AutoGenerateBackendFile = autoGenerateBackendFileFlagParts[1]
		}

		if arg == g.WorkflowDirFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, errors.New(fmt.Sprintf("invalid flag: %s", arg))
			}
			info.WorkflowsDir = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", g.WorkflowDirFlag) {
			var workflowDirFlagParts = strings.Split(arg, "=")
			if len(workflowDirFlagParts) != 2 {
				return info, errors.New(fmt.Sprintf("invalid flag: %s", arg))
			}
			info.WorkflowsDir = workflowDirFlagParts[1]
		}

		if arg == g.InitRunReconfigure {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, errors.New(fmt.Sprintf("invalid flag: %s", arg))
			}
			info.InitRunReconfigure = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", g.InitRunReconfigure) {
			var initRunReconfigureParts = strings.Split(arg, "=")
			if len(initRunReconfigureParts) != 2 {
				return info, errors.New(fmt.Sprintf("invalid flag: %s", arg))
			}
			info.InitRunReconfigure = initRunReconfigureParts[1]
		}

		if arg == g.FromPlanFlag {
			info.UseTerraformPlan = true
		}

		if arg == g.DryRunFlag {
			info.DryRun = true
		}

		if arg == g.HelpFlag1 || arg == g.HelpFlag2 {
			info.NeedHelp = true
		}

		for _, f := range commonFlags {
			if arg == f {
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
			if strings.HasPrefix(arg, g.GlobalOptionsFlag+"=") {
				parts := strings.SplitN(arg, "=", 2)
				globalOptions = strings.Split(parts[1], " ")
			} else {
				globalOptions = strings.Split(arg, " ")
			}
		}
	}

	info.GlobalOptions = globalOptions

	if info.NeedHelp == true {
		if len(additionalArgsAndFlags) > 0 {
			info.SubCommand = additionalArgsAndFlags[0]
		}
		return info, nil
	}

	if len(additionalArgsAndFlags) > 1 {
		twoWordsCommand := false

		// Handle terraform two-words commands
		// https://www.terraform.io/cli/commands
		if componentType == "terraform" {
			// Handle the custom legacy command `terraform write varfile` (NOTE: use `terraform generate varfile` instead)
			if additionalArgsAndFlags[0] == "write" && additionalArgsAndFlags[1] == "varfile" {
				info.SubCommand = "write"
				info.SubCommand2 = "varfile"
				twoWordsCommand = true
			}
			// `terraform workspace` commands
			if additionalArgsAndFlags[0] == "workspace" &&
				u.SliceContainsString([]string{"list", "select", "new", "delete", "show"}, additionalArgsAndFlags[1]) {
				info.SubCommand = "workspace"
				info.SubCommand2 = additionalArgsAndFlags[1]
				twoWordsCommand = true
			}
			// `terraform state` commands
			if additionalArgsAndFlags[0] == "state" &&
				u.SliceContainsString([]string{"list", "mv", "pull", "push", "replace-provider", "rm", "show"}, additionalArgsAndFlags[1]) {
				info.SubCommand = "state"
				info.SubCommand2 = additionalArgsAndFlags[1]
				twoWordsCommand = true
			}
		}

		if twoWordsCommand {
			info.ComponentFromArg = additionalArgsAndFlags[2]
			info.AdditionalArgsAndFlags = additionalArgsAndFlags[3:]
		} else {
			info.SubCommand = additionalArgsAndFlags[0]
			info.ComponentFromArg = additionalArgsAndFlags[1]
			info.AdditionalArgsAndFlags = additionalArgsAndFlags[2:]
		}
	}

	return info, nil
}

// generateComponentBackendConfig generates backend config for components
func generateComponentBackendConfig(backendType string, backendConfig map[interface{}]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"terraform": map[string]interface{}{
			"backend": map[string]interface{}{
				backendType: backendConfig,
			},
		},
	}
}

// convertEnvVars convert ENV vars from a map to a list of strings in the format ["key1=val1", "key2=val2", "key3=val3" ...]
func convertEnvVars(envVarsMap map[interface{}]interface{}) []string {
	res := []string{}
	if envVarsMap != nil {
		for k, v := range envVarsMap {
			res = append(res, fmt.Sprintf("%s=%s", k, v))
		}
	}
	return res
}
