package exec

import (
	"errors"
	"fmt"
	"os"
	"strings"

	cfg "github.com/cloudposse/atmos/pkg/config"
	s "github.com/cloudposse/atmos/pkg/stack"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
)

var (
	// `commonFlags` are a list of flags that atmos understands but the underlying tools do not (e.g. terraform, helmfile, etc.).
	// These flags get removed from the arg list after atmos uses them so the underlying tool does not get passed a flag it doesn't accept.
	commonFlags = []string{
		"--stack",
		"-s",
		cfg.DryRunFlag,
		cfg.SkipInitFlag,
		cfg.KubeConfigConfigFlag,
		cfg.TerraformDirFlag,
		cfg.HelmfileDirFlag,
		cfg.CliConfigDirFlag,
		cfg.StackDirFlag,
		cfg.BasePathFlag,
		cfg.GlobalOptionsFlag,
		cfg.DeployRunInitFlag,
		cfg.InitRunReconfigure,
		cfg.AutoGenerateBackendFileFlag,
		cfg.FromPlanFlag,
		cfg.HelpFlag1,
		cfg.HelpFlag2,
		cfg.WorkflowDirFlag,
		cfg.JsonSchemaDirFlag,
		cfg.OpaDirFlag,
		cfg.CueDirFlag,
	}
)

// FindComponentConfig finds component config sections
func FindComponentConfig(
	stack string,
	stacksMap map[string]any,
	componentType string,
	component string,
) (map[string]any,
	map[any]any,
	map[any]any,
	map[any]any,
	string,
	string,
	string,
	[]string,
	bool,
	map[any]any,
	error,
) {

	var stackSection map[any]any
	var componentsSection map[string]any
	var componentTypeSection map[string]any
	var componentSection map[string]any
	var componentVarsSection map[any]any
	var componentEnvSection map[any]any
	var componentBackendSection map[any]any
	var componentBackendType string
	var command string
	var componentInheritanceChain []string
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
	if stackSection, ok = stacksMap[stack].(map[any]any); !ok {
		return nil, nil, nil, nil, "", "", "", nil, false, nil, fmt.Errorf("could not find the stack '%s'", stack)
	}
	if componentsSection, ok = stackSection["components"].(map[string]any); !ok {
		return nil, nil, nil, nil, "", "", "", nil, false, nil, fmt.Errorf("'components' section is missing in the stack file '%s'", stack)
	}
	if componentTypeSection, ok = componentsSection[componentType].(map[string]any); !ok {
		return nil, nil, nil, nil, "", "", "", nil, false, nil, fmt.Errorf("'components/%s' section is missing in the stack file '%s'", componentType, stack)
	}
	if componentSection, ok = componentTypeSection[component].(map[string]any); !ok {
		return nil, nil, nil, nil, "", "", "", nil, false, nil, fmt.Errorf("no config found for the component '%s' in the stack file '%s'", component, stack)
	}
	if componentVarsSection, ok = componentSection["vars"].(map[any]any); !ok {
		return nil, nil, nil, nil, "", "", "", nil, false, nil, fmt.Errorf("missing 'vars' section for the component '%s' in the stack file '%s'", component, stack)
	}
	if componentBackendSection, ok = componentSection["backend"].(map[any]any); !ok {
		componentBackendSection = nil
	}
	if componentBackendType, ok = componentSection["backend_type"].(string); !ok {
		componentBackendType = ""
	}
	if command, ok = componentSection["command"].(string); !ok {
		command = ""
	}
	if componentEnvSection, ok = componentSection["env"].(map[any]any); !ok {
		componentEnvSection = map[any]any{}
	}
	if componentInheritanceChain, ok = componentSection["inheritance"].([]string); !ok {
		componentInheritanceChain = []string{}
	}

	// Process component metadata and find a base component (if any) and whether the component is real or abstract
	componentMetadata, baseComponentName, componentIsAbstract := ProcessComponentMetadata(component, componentSection)

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

// processCommandLineArgs processes command-line args
func processCommandLineArgs(componentType string, cmd *cobra.Command, args []string) (cfg.ConfigAndStacksInfo, error) {
	var configAndStacksInfo cfg.ConfigAndStacksInfo

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
	configAndStacksInfo.SkipInit = argsAndFlagsInfo.SkipInit
	configAndStacksInfo.NeedHelp = argsAndFlagsInfo.NeedHelp
	configAndStacksInfo.JsonSchemaDir = argsAndFlagsInfo.JsonSchemaDir
	configAndStacksInfo.OpaDir = argsAndFlagsInfo.OpaDir
	configAndStacksInfo.CueDir = argsAndFlagsInfo.CueDir

	// Check if `-h` or `--help` flags are specified
	if argsAndFlagsInfo.NeedHelp {
		err = processHelp(componentType, argsAndFlagsInfo.SubCommand)
		if err != nil {
			return configAndStacksInfo, err
		}
		return configAndStacksInfo, nil
	}

	flags := cmd.Flags()

	stack, err := flags.GetString("stack")
	if err == nil && stack != "" {
		configAndStacksInfo.Stack = stack
	}

	return configAndStacksInfo, nil
}

// FindStacksMap processes stack config and returns a map of all stacks
func FindStacksMap(cliConfig cfg.CliConfiguration) (
	map[string]any,
	map[string]map[string]any,
	error,
) {
	// Process stack config file(s)
	_, stacksMap, rawStackConfigs, err := s.ProcessYAMLConfigFiles(
		cliConfig.StacksBaseAbsolutePath,
		cliConfig.TerraformDirAbsolutePath,
		cliConfig.HelmfileDirAbsolutePath,
		cliConfig.StackConfigFilesAbsolutePaths,
		false,
		true)

	if err != nil {
		return nil, nil, err
	}

	return stacksMap, rawStackConfigs, nil
}

// ProcessStacks processes stack config
func ProcessStacks(
	cliConfig cfg.CliConfiguration,
	configAndStacksInfo cfg.ConfigAndStacksInfo,
	checkStack bool,
) (
	cfg.ConfigAndStacksInfo,
	error,
) {

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

	stacksMap, rawStackConfigs, err := FindStacksMap(cliConfig)
	if err != nil {
		return configAndStacksInfo, err
	}

	// Print the stack config files
	if cliConfig.Logs.Verbose {
		fmt.Println()
		var msg string
		if cliConfig.StackType == "Directory" {
			msg = "Found the config file for the provided stack:"
		} else {
			msg = "Found stack config files:"
		}
		u.PrintInfo(msg)
		err = u.PrintAsYAML(cliConfig.StackConfigFilesRelativePaths)
		if err != nil {
			return configAndStacksInfo, err
		}
	}

	// Check and process stacks
	if cliConfig.StackType == "Directory" {
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

		configAndStacksInfo.ComponentEnvList = u.ConvertEnvVars(configAndStacksInfo.ComponentEnvSection)
		configAndStacksInfo.StackFile = configAndStacksInfo.Stack

		// Process context
		configAndStacksInfo.Context = cfg.GetContextFromVars(configAndStacksInfo.ComponentVarsSection)
		configAndStacksInfo.Context.Component = configAndStacksInfo.ComponentFromArg
		configAndStacksInfo.Context.BaseComponent = configAndStacksInfo.BaseComponentPath
		configAndStacksInfo.ContextPrefix, err = cfg.GetContextPrefix(configAndStacksInfo.Stack,
			configAndStacksInfo.Context,
			cliConfig.Stacks.NamePattern,
			configAndStacksInfo.Stack,
		)
		if err != nil {
			return configAndStacksInfo, err
		}
	} else {
		u.PrintInfoVerbose(cliConfig.Logs.Verbose, fmt.Sprintf("Searching for stack config where the component '%s' is defined", configAndStacksInfo.ComponentFromArg))
		foundStackCount := 0
		var foundStacks []string
		var foundConfigAndStacksInfo cfg.ConfigAndStacksInfo

		for stackName := range stacksMap {
			// Check if we've found the component config
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
				u.PrintErrorVerbose(cliConfig.Logs.Verbose, err)
				continue
			}

			configAndStacksInfo.ComponentEnvList = u.ConvertEnvVars(configAndStacksInfo.ComponentEnvSection)

			// Process context
			configAndStacksInfo.Context = cfg.GetContextFromVars(configAndStacksInfo.ComponentVarsSection)
			configAndStacksInfo.Context.Component = configAndStacksInfo.ComponentFromArg
			configAndStacksInfo.Context.BaseComponent = configAndStacksInfo.BaseComponentPath
			configAndStacksInfo.ContextPrefix, err = cfg.GetContextPrefix(configAndStacksInfo.Stack,
				configAndStacksInfo.Context,
				cliConfig.Stacks.NamePattern,
				stackName,
			)
			if err != nil {
				// If any of the stack config files throws error (which also means that we can't find the component in that stack),
				// print the error to the console and continue searching for the component in the other stack config files.
				u.PrintErrorVerbose(cliConfig.Logs.Verbose, err)
				continue
			}

			// Check if we've found the stack
			if configAndStacksInfo.Stack == configAndStacksInfo.ContextPrefix {
				configAndStacksInfo.StackFile = stackName
				foundConfigAndStacksInfo = configAndStacksInfo
				foundStackCount++
				foundStacks = append(foundStacks, stackName)

				u.PrintInfoVerbose(
					cliConfig.Logs.Verbose,
					fmt.Sprintf("Found config for the component '%s' for the stack '%s' in the stack file '%s'",
						configAndStacksInfo.ComponentFromArg,
						configAndStacksInfo.Stack,
						stackName,
					))
			}
		}

		if foundStackCount == 0 {
			y, _ := u.ConvertToYAML(cliConfig)

			return configAndStacksInfo,
				fmt.Errorf("\nSearched all stack YAML files, but could not find config for the component '%s' in the stack '%s'.\n"+
					"Check that all variables in the stack name pattern '%s' are correctly defined in the stack config files.\n"+
					"Are the component and stack names correct? Did you forget an import?\n\n\nCLI config:\n\n%v",
					configAndStacksInfo.ComponentFromArg,
					configAndStacksInfo.Stack,
					cliConfig.Stacks.NamePattern,
					y)
		} else if foundStackCount > 1 {
			err = fmt.Errorf("\nFound duplicate config for the component '%s' for the stack '%s' in the files: %v.\n"+
				"Check that all context variables in the stack name pattern '%s' are correctly defined in the files and not duplicated.\n"+
				"Check that all imports are valid.",
				configAndStacksInfo.ComponentFromArg,
				configAndStacksInfo.Stack,
				strings.Join(foundStacks, ", "),
				cliConfig.Stacks.NamePattern)
			u.PrintErrorToStdErrorAndExit(err)
		} else {
			configAndStacksInfo = foundConfigAndStacksInfo
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
			baseComponentPartsWithoutLast := baseComponentPathParts[:baseComponentPathPartsLength-1]
			configAndStacksInfo.ComponentFolderPrefix = strings.Join(baseComponentPartsWithoutLast, "/")
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
		cliConfig.Stacks.NamePattern,
		configAndStacksInfo.ComponentMetadataSection,
		configAndStacksInfo.Context,
	)
	if err != nil {
		return configAndStacksInfo, err
	}

	configAndStacksInfo.TerraformWorkspace = workspace
	configAndStacksInfo.ComponentSection["workspace"] = workspace

	// sources (stack config files where the variables and other settings are defined)
	sources, err := findConfigSources(configAndStacksInfo, rawStackConfigs)
	if err != nil {
		return configAndStacksInfo, err
	}

	configAndStacksInfo.ComponentSection["sources"] = sources

	return configAndStacksInfo, nil
}

// findConfigSources finds the sources (files) for all variables for a component in a stack
func findConfigSources(configAndStacksInfo cfg.ConfigAndStacksInfo, rawStackConfigs map[string]map[string]any) (map[string]map[string]any, error) {
	result := map[string]map[string]any{}
	vars := map[string]any{}
	result["vars"] = vars

	for varKey, varVal := range configAndStacksInfo.ComponentVarsSection {
		varKeyStr := varKey.(string)
		varObj := map[string]any{}
		varObj["final"] = varVal
		varObj["inherited"] = map[string]any{}
		vars[varKeyStr] = varObj
	}

	return result, nil
}

// processArgsAndFlags processes args and flags from the provided CLI arguments/flags
func processArgsAndFlags(componentType string, inputArgsAndFlags []string) (cfg.ArgsAndFlagsInfo, error) {
	var info cfg.ArgsAndFlagsInfo
	var additionalArgsAndFlags []string
	var globalOptions []string

	var indexesToRemove []int

	// https://github.com/roboll/helmfile#cli-reference
	var globalOptionsFlagIndex int

	for i, arg := range inputArgsAndFlags {
		if arg == cfg.GlobalOptionsFlag {
			globalOptionsFlagIndex = i + 1
		} else if strings.HasPrefix(arg+"=", cfg.GlobalOptionsFlag) {
			globalOptionsFlagIndex = i
		}

		if arg == cfg.TerraformDirFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, fmt.Errorf("invalid flag: %s", arg)
			}
			info.TerraformDir = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", cfg.TerraformDirFlag) {
			var terraformDirFlagParts = strings.Split(arg, "=")
			if len(terraformDirFlagParts) != 2 {
				return info, fmt.Errorf("invalid flag: %s", arg)
			}
			info.TerraformDir = terraformDirFlagParts[1]
		}

		if arg == cfg.HelmfileDirFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, fmt.Errorf("invalid flag: %s", arg)
			}
			info.HelmfileDir = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", cfg.HelmfileDirFlag) {
			var helmfileDirFlagParts = strings.Split(arg, "=")
			if len(helmfileDirFlagParts) != 2 {
				return info, fmt.Errorf("invalid flag: %s", arg)
			}
			info.HelmfileDir = helmfileDirFlagParts[1]
		}

		if arg == cfg.CliConfigDirFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, fmt.Errorf("invalid flag: %s", arg)
			}
			info.ConfigDir = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", cfg.CliConfigDirFlag) {
			var configDirFlagParts = strings.Split(arg, "=")
			if len(configDirFlagParts) != 2 {
				return info, fmt.Errorf("invalid flag: %s", arg)
			}
			info.ConfigDir = configDirFlagParts[1]
		}

		if arg == cfg.StackDirFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, fmt.Errorf("invalid flag: %s", arg)
			}
			info.StacksDir = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", cfg.StackDirFlag) {
			var stacksDirFlagParts = strings.Split(arg, "=")
			if len(stacksDirFlagParts) != 2 {
				return info, fmt.Errorf("invalid flag: %s", arg)
			}
			info.StacksDir = stacksDirFlagParts[1]
		}

		if arg == cfg.BasePathFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, fmt.Errorf("invalid flag: %s", arg)
			}
			info.BasePath = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", cfg.BasePathFlag) {
			var stacksDirFlagParts = strings.Split(arg, "=")
			if len(stacksDirFlagParts) != 2 {
				return info, fmt.Errorf("invalid flag: %s", arg)
			}
			info.BasePath = stacksDirFlagParts[1]
		}

		if arg == cfg.DeployRunInitFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, fmt.Errorf("invalid flag: %s", arg)
			}
			info.DeployRunInit = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", cfg.DeployRunInitFlag) {
			var deployRunInitFlagParts = strings.Split(arg, "=")
			if len(deployRunInitFlagParts) != 2 {
				return info, fmt.Errorf("invalid flag: %s", arg)
			}
			info.DeployRunInit = deployRunInitFlagParts[1]
		}

		if arg == cfg.AutoGenerateBackendFileFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, fmt.Errorf("invalid flag: %s", arg)
			}
			info.AutoGenerateBackendFile = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", cfg.AutoGenerateBackendFileFlag) {
			var autoGenerateBackendFileFlagParts = strings.Split(arg, "=")
			if len(autoGenerateBackendFileFlagParts) != 2 {
				return info, fmt.Errorf("invalid flag: %s", arg)
			}
			info.AutoGenerateBackendFile = autoGenerateBackendFileFlagParts[1]
		}

		if arg == cfg.WorkflowDirFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, fmt.Errorf("invalid flag: %s", arg)
			}
			info.WorkflowsDir = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", cfg.WorkflowDirFlag) {
			var workflowDirFlagParts = strings.Split(arg, "=")
			if len(workflowDirFlagParts) != 2 {
				return info, fmt.Errorf("invalid flag: %s", arg)
			}
			info.WorkflowsDir = workflowDirFlagParts[1]
		}

		if arg == cfg.InitRunReconfigure {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, fmt.Errorf("invalid flag: %s", arg)
			}
			info.InitRunReconfigure = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", cfg.InitRunReconfigure) {
			var initRunReconfigureParts = strings.Split(arg, "=")
			if len(initRunReconfigureParts) != 2 {
				return info, fmt.Errorf("invalid flag: %s", arg)
			}
			info.InitRunReconfigure = initRunReconfigureParts[1]
		}

		if arg == cfg.JsonSchemaDirFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, fmt.Errorf("invalid flag: %s", arg)
			}
			info.JsonSchemaDir = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", cfg.JsonSchemaDirFlag) {
			var jsonschemaDirFlagParts = strings.Split(arg, "=")
			if len(jsonschemaDirFlagParts) != 2 {
				return info, fmt.Errorf("invalid flag: %s", arg)
			}
			info.JsonSchemaDir = jsonschemaDirFlagParts[1]
		}

		if arg == cfg.OpaDirFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, fmt.Errorf("invalid flag: %s", arg)
			}
			info.OpaDir = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", cfg.OpaDirFlag) {
			var opaDirFlagParts = strings.Split(arg, "=")
			if len(opaDirFlagParts) != 2 {
				return info, fmt.Errorf("invalid flag: %s", arg)
			}
			info.OpaDir = opaDirFlagParts[1]
		}

		if arg == cfg.CueDirFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, fmt.Errorf("invalid flag: %s", arg)
			}
			info.CueDir = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", cfg.CueDirFlag) {
			var cueDirFlagParts = strings.Split(arg, "=")
			if len(cueDirFlagParts) != 2 {
				return info, fmt.Errorf("invalid flag: %s", arg)
			}
			info.CueDir = cueDirFlagParts[1]
		}

		if arg == cfg.FromPlanFlag {
			info.UseTerraformPlan = true
		}

		if arg == cfg.DryRunFlag {
			info.DryRun = true
		}

		if arg == cfg.SkipInitFlag {
			info.SkipInit = true
		}

		if arg == cfg.HelpFlag1 || arg == cfg.HelpFlag2 {
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
			if strings.HasPrefix(arg, cfg.GlobalOptionsFlag+"=") {
				parts := strings.SplitN(arg, "=", 2)
				globalOptions = strings.Split(parts[1], " ")
			} else {
				globalOptions = strings.Split(arg, " ")
			}
		}
	}

	info.GlobalOptions = globalOptions

	if info.NeedHelp {
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
			// https://www.terraform.io/cli/commands/workspace
			if additionalArgsAndFlags[0] == "workspace" &&
				u.SliceContainsString([]string{"list", "select", "new", "delete", "show"}, additionalArgsAndFlags[1]) {
				info.SubCommand = "workspace"
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
func generateComponentBackendConfig(backendType string, backendConfig map[any]any) map[string]any {
	return map[string]any{
		"terraform": map[string]any{
			"backend": map[string]any{
				backendType: backendConfig,
			},
		},
	}
}

// printOrWriteToFile takes the output format (`yaml` or `json`) and a file name,
// and prints the data to the console or to a file (if file is specified)
func printOrWriteToFile(format string, file string, data any) error {
	switch format {
	case "yaml":
		if file == "" {
			err := u.PrintAsYAML(data)
			if err != nil {
				return err
			}
		} else {
			err := u.WriteToFileAsYAML(file, data, 0644)
			if err != nil {
				return err
			}
		}

	case "json":
		if file == "" {
			err := u.PrintAsJSON(data)
			if err != nil {
				return err
			}
		} else {
			err := u.WriteToFileAsJSON(file, data, 0644)
			if err != nil {
				return err
			}
		}

	default:
		return fmt.Errorf("invalid 'format': %s", format)
	}

	return nil
}

func removeTempDir(path string) {
	err := os.RemoveAll(path)
	if err != nil {
		u.PrintError(err)
	}
}
