package exec

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-config-inspect/tfconfig"
	"github.com/mitchellh/mapstructure"
	"github.com/spf13/cobra"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
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
		cfg.TerraformCommandFlag,
		cfg.TerraformDirFlag,
		cfg.HelmfileCommandFlag,
		cfg.HelmfileDirFlag,
		cfg.CliConfigDirFlag,
		cfg.StackDirFlag,
		cfg.BasePathFlag,
		cfg.VendorBasePathFlag,
		cfg.GlobalOptionsFlag,
		cfg.DeployRunInitFlag,
		cfg.InitRunReconfigure,
		cfg.AutoGenerateBackendFileFlag,
		cfg.AppendUserAgentFlag,
		cfg.FromPlanFlag,
		cfg.PlanFileFlag,
		cfg.HelpFlag1,
		cfg.HelpFlag2,
		cfg.HelpFlag3,
		cfg.WorkflowDirFlag,
		cfg.JsonSchemaDirFlag,
		cfg.OpaDirFlag,
		cfg.CueDirFlag,
		cfg.AtmosManifestJsonSchemaFlag,
		cfg.RedirectStdErrFlag,
		cfg.LogsLevelFlag,
		cfg.LogsFileFlag,
	}
)

// ProcessComponentConfig processes component config sections
func ProcessComponentConfig(
	configAndStacksInfo *schema.ConfigAndStacksInfo,
	stack string,
	stacksMap map[string]any,
	componentType string,
	component string,
) error {

	var stackSection map[string]any
	var componentsSection map[string]any
	var componentTypeSection map[string]any
	var componentSection map[string]any
	var componentVarsSection map[string]any
	var componentSettingsSection map[string]any
	var componentOverridesSection map[string]any
	var componentProvidersSection map[string]any
	var componentImportsSection []string
	var componentEnvSection map[string]any
	var componentBackendSection map[string]any
	var componentBackendType string
	var command string
	var componentInheritanceChain []string
	var ok bool

	if len(stack) == 0 {
		return errors.New("stack must be provided and must not be empty")
	}
	if len(component) == 0 {
		return errors.New("component must be provided and must not be empty")
	}
	if len(componentType) == 0 {
		return errors.New("component type must be provided and must not be empty")
	}
	if stackSection, ok = stacksMap[stack].(map[string]any); !ok {
		return fmt.Errorf("could not find the stack '%s'", stack)
	}
	if componentsSection, ok = stackSection["components"].(map[string]any); !ok {
		return fmt.Errorf("'components' section is missing in the stack manifest '%s'", stack)
	}
	if componentTypeSection, ok = componentsSection[componentType].(map[string]any); !ok {
		return fmt.Errorf("'components.%s' section is missing in the stack manifest '%s'", componentType, stack)
	}
	if componentSection, ok = componentTypeSection[component].(map[string]any); !ok {
		return fmt.Errorf("no config found for the component '%s' in the stack manifest '%s'", component, stack)
	}
	if componentVarsSection, ok = componentSection["vars"].(map[string]any); !ok {
		return fmt.Errorf("missing 'vars' section for the component '%s' in the stack manifest '%s'", component, stack)
	}
	if componentProvidersSection, ok = componentSection[cfg.ProvidersSectionName].(map[string]any); !ok {
		componentProvidersSection = map[string]any{}
	}
	if componentBackendSection, ok = componentSection[cfg.BackendSectionName].(map[string]any); !ok {
		componentBackendSection = nil
	}
	if componentBackendType, ok = componentSection[cfg.BackendTypeSectionName].(string); !ok {
		componentBackendType = ""
	}
	if componentImportsSection, ok = stackSection["imports"].([]string); !ok {
		componentImportsSection = nil
	}
	if command, ok = componentSection[cfg.CommandSectionName].(string); !ok {
		command = ""
	}
	if componentEnvSection, ok = componentSection[cfg.EnvSectionName].(map[string]any); !ok {
		componentEnvSection = map[string]any{}
	}
	if componentSettingsSection, ok = componentSection[cfg.SettingsSectionName].(map[string]any); !ok {
		componentSettingsSection = map[string]any{}
	}
	if componentOverridesSection, ok = componentSection[cfg.OverridesSectionName].(map[string]any); !ok {
		componentOverridesSection = map[string]any{}
	}
	if componentInheritanceChain, ok = componentSection["inheritance"].([]string); !ok {
		componentInheritanceChain = []string{}
	}

	// Process component metadata and find a base component (if any) and whether the component is real or abstract
	componentMetadata, baseComponentName, componentIsAbstract, componentIsEnabled := ProcessComponentMetadata(component, componentSection)
	configAndStacksInfo.ComponentIsEnabled = componentIsEnabled

	// Remove the ENV vars that are set to `null` in the `env` section.
	// Setting an ENV var to `null` in stack config has the effect of unsetting it
	// because the exec.Command, which sets these ENV vars, is itself executed in a separate process started by the os.StartProcess function.
	componentEnvSectionFiltered := map[string]any{}

	for k, v := range componentEnvSection {
		if v != nil {
			componentEnvSectionFiltered[k] = v
		}
	}

	configAndStacksInfo.ComponentSection = componentSection
	configAndStacksInfo.ComponentVarsSection = componentVarsSection
	configAndStacksInfo.ComponentSettingsSection = componentSettingsSection
	configAndStacksInfo.ComponentOverridesSection = componentOverridesSection
	configAndStacksInfo.ComponentProvidersSection = componentProvidersSection
	configAndStacksInfo.ComponentEnvSection = componentEnvSectionFiltered
	configAndStacksInfo.ComponentBackendSection = componentBackendSection
	configAndStacksInfo.ComponentBackendType = componentBackendType
	configAndStacksInfo.BaseComponentPath = baseComponentName
	configAndStacksInfo.Command = command
	configAndStacksInfo.ComponentInheritanceChain = componentInheritanceChain
	configAndStacksInfo.ComponentIsAbstract = componentIsAbstract
	configAndStacksInfo.ComponentMetadataSection = componentMetadata
	configAndStacksInfo.ComponentImportsSection = componentImportsSection

	return nil
}

// processCommandLineArgs processes command-line args
func processCommandLineArgs(
	componentType string,
	cmd *cobra.Command,
	args []string,
	additionalArgsAndFlags []string,
) (schema.ConfigAndStacksInfo, error) {
	var configAndStacksInfo schema.ConfigAndStacksInfo

	cmd.DisableFlagParsing = false

	err := cmd.ParseFlags(args)
	if err != nil {
		return configAndStacksInfo, err
	}

	argsAndFlagsInfo, err := processArgsAndFlags(componentType, args)
	if err != nil {
		return configAndStacksInfo, err
	}

	finalAdditionalArgsAndFlags := argsAndFlagsInfo.AdditionalArgsAndFlags
	if len(additionalArgsAndFlags) > 0 {
		finalAdditionalArgsAndFlags = append(finalAdditionalArgsAndFlags, additionalArgsAndFlags...)
	}

	configAndStacksInfo.AdditionalArgsAndFlags = finalAdditionalArgsAndFlags
	configAndStacksInfo.SubCommand = argsAndFlagsInfo.SubCommand
	configAndStacksInfo.SubCommand2 = argsAndFlagsInfo.SubCommand2
	configAndStacksInfo.ComponentType = componentType
	configAndStacksInfo.ComponentFromArg = argsAndFlagsInfo.ComponentFromArg
	configAndStacksInfo.GlobalOptions = argsAndFlagsInfo.GlobalOptions
	configAndStacksInfo.BasePath = argsAndFlagsInfo.BasePath
	configAndStacksInfo.TerraformCommand = argsAndFlagsInfo.TerraformCommand
	configAndStacksInfo.TerraformDir = argsAndFlagsInfo.TerraformDir
	configAndStacksInfo.HelmfileCommand = argsAndFlagsInfo.HelmfileCommand
	configAndStacksInfo.HelmfileDir = argsAndFlagsInfo.HelmfileDir
	configAndStacksInfo.StacksDir = argsAndFlagsInfo.StacksDir
	configAndStacksInfo.ConfigDir = argsAndFlagsInfo.ConfigDir
	configAndStacksInfo.WorkflowsDir = argsAndFlagsInfo.WorkflowsDir
	configAndStacksInfo.DeployRunInit = argsAndFlagsInfo.DeployRunInit
	configAndStacksInfo.InitRunReconfigure = argsAndFlagsInfo.InitRunReconfigure
	configAndStacksInfo.AutoGenerateBackendFile = argsAndFlagsInfo.AutoGenerateBackendFile
	configAndStacksInfo.UseTerraformPlan = argsAndFlagsInfo.UseTerraformPlan
	configAndStacksInfo.PlanFile = argsAndFlagsInfo.PlanFile
	configAndStacksInfo.DryRun = argsAndFlagsInfo.DryRun
	configAndStacksInfo.SkipInit = argsAndFlagsInfo.SkipInit
	configAndStacksInfo.NeedHelp = argsAndFlagsInfo.NeedHelp
	configAndStacksInfo.JsonSchemaDir = argsAndFlagsInfo.JsonSchemaDir
	configAndStacksInfo.AtmosManifestJsonSchema = argsAndFlagsInfo.AtmosManifestJsonSchema
	configAndStacksInfo.OpaDir = argsAndFlagsInfo.OpaDir
	configAndStacksInfo.CueDir = argsAndFlagsInfo.CueDir
	configAndStacksInfo.RedirectStdErr = argsAndFlagsInfo.RedirectStdErr
	configAndStacksInfo.LogsLevel = argsAndFlagsInfo.LogsLevel
	configAndStacksInfo.LogsFile = argsAndFlagsInfo.LogsFile
	configAndStacksInfo.SettingsListMergeStrategy = argsAndFlagsInfo.SettingsListMergeStrategy

	// Check if `-h` or `--help` flags are specified
	if argsAndFlagsInfo.NeedHelp {
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
func FindStacksMap(cliConfig schema.CliConfiguration, ignoreMissingFiles bool) (
	map[string]any,
	map[string]map[string]any,
	error,
) {
	// Process stack config file(s)
	_, stacksMap, rawStackConfigs, err := ProcessYAMLConfigFiles(
		cliConfig,
		cliConfig.StacksBaseAbsolutePath,
		cliConfig.TerraformDirAbsolutePath,
		cliConfig.HelmfileDirAbsolutePath,
		cliConfig.StackConfigFilesAbsolutePaths,
		false,
		true,
		ignoreMissingFiles,
	)

	if err != nil {
		return nil, nil, err
	}

	return stacksMap, rawStackConfigs, nil
}

// ProcessStacks processes stack config
func ProcessStacks(
	cliConfig schema.CliConfiguration,
	configAndStacksInfo schema.ConfigAndStacksInfo,
	checkStack bool,
	processTemplates bool,
) (schema.ConfigAndStacksInfo, error) {

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

	// Initialize component section maps
	configAndStacksInfo.ComponentSection = make(map[string]any)
	configAndStacksInfo.ComponentVarsSection = make(map[string]any)
	configAndStacksInfo.ComponentSettingsSection = make(map[string]any)
	configAndStacksInfo.ComponentOverridesSection = make(map[string]any)
	configAndStacksInfo.ComponentProvidersSection = make(map[string]any)
	configAndStacksInfo.ComponentEnvSection = make(map[string]any)
	configAndStacksInfo.ComponentBackendSection = make(map[string]any)
	configAndStacksInfo.ComponentMetadataSection = make(map[string]any)

	stacksMap, rawStackConfigs, err := FindStacksMap(cliConfig, false)
	if err != nil {
		return configAndStacksInfo, err
	}

	// Print the stack config files
	if cliConfig.Logs.Level == u.LogLevelTrace {
		var msg string
		if cliConfig.StackType == "Directory" {
			msg = "\nFound stack manifest:"
		} else {
			msg = "\nFound stack manifests:"
		}
		u.LogTrace(cliConfig, msg)
		err = u.PrintAsYAMLToFileDescriptor(cliConfig, cliConfig.StackConfigFilesRelativePaths)
		if err != nil {
			return configAndStacksInfo, err
		}
	}

	// Check and process stacks
	if cliConfig.StackType == "Directory" {
		err = ProcessComponentConfig(
			&configAndStacksInfo,
			configAndStacksInfo.Stack,
			stacksMap,
			configAndStacksInfo.ComponentType,
			configAndStacksInfo.ComponentFromArg,
		)
		if err != nil {
			return configAndStacksInfo, err
		}

		configAndStacksInfo.StackFile = configAndStacksInfo.Stack

		// Process context
		configAndStacksInfo.Context = cfg.GetContextFromVars(configAndStacksInfo.ComponentVarsSection)
		configAndStacksInfo.Context.Component = configAndStacksInfo.ComponentFromArg
		configAndStacksInfo.Context.BaseComponent = configAndStacksInfo.BaseComponentPath

		configAndStacksInfo.ContextPrefix, err = cfg.GetContextPrefix(configAndStacksInfo.Stack,
			configAndStacksInfo.Context,
			GetStackNamePattern(cliConfig),
			configAndStacksInfo.Stack,
		)
		if err != nil {
			return configAndStacksInfo, err
		}
	} else {
		foundStackCount := 0
		var foundStacks []string
		var foundConfigAndStacksInfo schema.ConfigAndStacksInfo

		for stackName := range stacksMap {
			// Check if we've found the component in the stack
			err = ProcessComponentConfig(
				&configAndStacksInfo,
				stackName,
				stacksMap,
				configAndStacksInfo.ComponentType,
				configAndStacksInfo.ComponentFromArg,
			)
			if err != nil {
				continue
			}

			if cliConfig.Stacks.NameTemplate != "" {
				tmpl, err2 := ProcessTmpl("name-template", cliConfig.Stacks.NameTemplate, configAndStacksInfo.ComponentSection, false)
				if err2 != nil {
					continue
				}
				configAndStacksInfo.ContextPrefix = tmpl
			} else if cliConfig.Stacks.NamePattern != "" {
				// Process context
				configAndStacksInfo.Context = cfg.GetContextFromVars(configAndStacksInfo.ComponentVarsSection)

				configAndStacksInfo.ContextPrefix, err = cfg.GetContextPrefix(configAndStacksInfo.Stack,
					configAndStacksInfo.Context,
					GetStackNamePattern(cliConfig),
					stackName,
				)
				if err != nil {
					continue
				}
			} else {
				return configAndStacksInfo, errors.New("'stacks.name_pattern' or 'stacks.name_template' needs to be specified in 'atmos.yaml' CLI config")
			}

			configAndStacksInfo.Context.Component = configAndStacksInfo.ComponentFromArg
			configAndStacksInfo.Context.BaseComponent = configAndStacksInfo.BaseComponentPath

			// Check if we've found the stack
			if configAndStacksInfo.Stack == configAndStacksInfo.ContextPrefix {
				configAndStacksInfo.StackFile = stackName
				foundConfigAndStacksInfo = configAndStacksInfo
				foundStackCount++
				foundStacks = append(foundStacks, stackName)

				u.LogDebug(
					cliConfig,
					fmt.Sprintf("Found component '%s' in the stack '%s' in the stack manifest '%s'",
						configAndStacksInfo.ComponentFromArg,
						configAndStacksInfo.Stack,
						stackName,
					))
			}
		}

		if foundStackCount == 0 {
			// Allow proceeding without error if checkStack is false (e.g., for operations that don't require a stack)
			if !checkStack {
				return configAndStacksInfo, nil
			}
		}

		if foundStackCount == 0 && configAndStacksInfo.ComponentIsEnabled {
			cliConfigYaml := ""

			if cliConfig.Logs.Level == u.LogLevelTrace {
				y, _ := u.ConvertToYAML(cliConfig)
				cliConfigYaml = fmt.Sprintf("\n\n\nCLI config: %v\n", y)
			}

			return configAndStacksInfo,
				fmt.Errorf("\nCould not find the component '%s' in the stack '%s'.\n"+
					"Check that all the context variables are correctly defined in the stack manifests.\n"+
					"Are the component and stack names correct? Did you forget an import?%v\n",
					configAndStacksInfo.ComponentFromArg,
					configAndStacksInfo.Stack,
					cliConfigYaml)
		} else if foundStackCount > 1 {
			err = fmt.Errorf("\nFound duplicate config for the component '%s' in the stack '%s' in the manifests: %v.\n"+
				"Check that all the context variables are correctly defined in the manifests and not duplicated.\n"+
				"Check that all imports are valid.",
				configAndStacksInfo.ComponentFromArg,
				configAndStacksInfo.Stack,
				strings.Join(foundStacks, ", "),
			)
			u.LogErrorAndExit(cliConfig, err)
		} else {
			configAndStacksInfo = foundConfigAndStacksInfo
		}
	}

	// Add imports
	configAndStacksInfo.ComponentSection["imports"] = configAndStacksInfo.ComponentImportsSection

	// Add Atmos component and stack
	configAndStacksInfo.ComponentSection["atmos_component"] = configAndStacksInfo.ComponentFromArg
	configAndStacksInfo.ComponentSection["atmos_stack"] = configAndStacksInfo.StackFromArg
	configAndStacksInfo.ComponentSection["stack"] = configAndStacksInfo.StackFromArg
	configAndStacksInfo.ComponentSection["atmos_stack_file"] = configAndStacksInfo.StackFile
	configAndStacksInfo.ComponentSection["atmos_manifest"] = configAndStacksInfo.StackFile

	// Add Atmos CLI config
	atmosCliConfig := map[string]any{}
	atmosCliConfig["base_path"] = cliConfig.BasePath
	atmosCliConfig["components"] = cliConfig.Components
	atmosCliConfig["stacks"] = cliConfig.Stacks
	atmosCliConfig["workflows"] = cliConfig.Workflows
	configAndStacksInfo.ComponentSection["atmos_cli_config"] = atmosCliConfig

	// If the command-line component does not inherit anything, then the Terraform/Helmfile component is the same as the provided one
	if comp, ok := configAndStacksInfo.ComponentSection[cfg.ComponentSectionName].(string); !ok || comp == "" {
		configAndStacksInfo.ComponentSection[cfg.ComponentSectionName] = configAndStacksInfo.ComponentFromArg
	}

	// `sources` (stack config files where the variables and other settings are defined)
	sources, err := ProcessConfigSources(configAndStacksInfo, rawStackConfigs)
	if err != nil {
		return configAndStacksInfo, err
	}

	configAndStacksInfo.ComponentSection["sources"] = sources

	// Component dependencies
	componentDeps, componentDepsAll, err := FindComponentDependencies(configAndStacksInfo.StackFile, sources)
	if err != nil {
		return configAndStacksInfo, err
	}
	configAndStacksInfo.ComponentSection["deps"] = componentDeps
	configAndStacksInfo.ComponentSection["deps_all"] = componentDepsAll

	// Terraform workspace
	workspace, err := BuildTerraformWorkspace(cliConfig, configAndStacksInfo)
	if err != nil {
		return configAndStacksInfo, err
	}

	configAndStacksInfo.TerraformWorkspace = workspace
	configAndStacksInfo.ComponentSection["workspace"] = workspace

	// Process `Go` templates in Atmos manifest sections
	if processTemplates {
		componentSectionStr, err := u.ConvertToYAML(configAndStacksInfo.ComponentSection)
		if err != nil {
			return configAndStacksInfo, err
		}

		var settingsSectionStruct schema.Settings

		err = mapstructure.Decode(configAndStacksInfo.ComponentSettingsSection, &settingsSectionStruct)
		if err != nil {
			return configAndStacksInfo, err
		}

		componentSectionProcessed, err := ProcessTmplWithDatasources(
			cliConfig,
			settingsSectionStruct,
			"all-atmos-sections",
			componentSectionStr,
			configAndStacksInfo.ComponentSection,
			true,
		)
		if err != nil {
			// If any error returned from the templates processing, log it and exit
			u.LogErrorAndExit(cliConfig, err)
		}

		componentSectionConverted, err := u.UnmarshalYAML[schema.AtmosSectionMapType](componentSectionProcessed)
		if err != nil {
			if !cliConfig.Templates.Settings.Enabled {
				if strings.Contains(componentSectionStr, "{{") || strings.Contains(componentSectionStr, "}}") {
					errorMessage := "the stack manifests contain Go templates, but templating is disabled in atmos.yaml in 'templates.settings.enabled'\n" +
						"to enable templating, refer to https://atmos.tools/core-concepts/stacks/templates"
					err = errors.Join(err, errors.New(errorMessage))
				}
			}
			u.LogErrorAndExit(cliConfig, err)
		}

		componentSectionFinal, err := ProcessCustomYamlTags(cliConfig, componentSectionConverted)
		if err != nil {
			return configAndStacksInfo, err
		}

		configAndStacksInfo.ComponentSection = componentSectionFinal

		// Process Atmos manifest sections after processing `Go` templates and custom YAML tags
		if i, ok := configAndStacksInfo.ComponentSection[cfg.ProvidersSectionName].(map[string]any); ok {
			configAndStacksInfo.ComponentProvidersSection = i
		}

		if i, ok := configAndStacksInfo.ComponentSection[cfg.VarsSectionName].(map[string]any); ok {
			configAndStacksInfo.ComponentVarsSection = i
		}

		if i, ok := configAndStacksInfo.ComponentSection[cfg.SettingsSectionName].(map[string]any); ok {
			configAndStacksInfo.ComponentSettingsSection = i
		}

		if i, ok := configAndStacksInfo.ComponentSection[cfg.EnvSectionName].(map[string]any); ok {
			configAndStacksInfo.ComponentEnvSection = i
		}

		if i, ok := configAndStacksInfo.ComponentSection[cfg.OverridesSectionName].(map[string]any); ok {
			configAndStacksInfo.ComponentOverridesSection = i
		}

		if i, ok := configAndStacksInfo.ComponentSection[cfg.MetadataSectionName].(map[string]any); ok {
			configAndStacksInfo.ComponentMetadataSection = i
		}

		if i, ok := configAndStacksInfo.ComponentSection[cfg.BackendSectionName].(map[string]any); ok {
			configAndStacksInfo.ComponentBackendSection = i
		}

		if i, ok := configAndStacksInfo.ComponentSection[cfg.BackendTypeSectionName].(string); ok {
			configAndStacksInfo.ComponentBackendType = i
		}

		if i, ok := configAndStacksInfo.ComponentSection[cfg.ComponentSectionName].(string); ok {
			configAndStacksInfo.Component = i
		}

		if i, ok := configAndStacksInfo.ComponentSection[cfg.CommandSectionName].(string); ok {
			configAndStacksInfo.Command = i
		}
	}

	// Spacelift stack
	spaceliftStackName, err := BuildSpaceliftStackNameFromComponentConfig(cliConfig, configAndStacksInfo)
	if err != nil {
		return configAndStacksInfo, err
	}
	if spaceliftStackName != "" {
		configAndStacksInfo.ComponentSection["spacelift_stack"] = spaceliftStackName
	}

	// Atlantis project
	atlantisProjectName, err := BuildAtlantisProjectNameFromComponentConfig(cliConfig, configAndStacksInfo)
	if err != nil {
		return configAndStacksInfo, err
	}
	if atlantisProjectName != "" {
		configAndStacksInfo.ComponentSection["atlantis_project"] = atlantisProjectName
	}

	// Process the ENV variables from the `env` section
	configAndStacksInfo.ComponentEnvList = u.ConvertEnvVars(configAndStacksInfo.ComponentEnvSection)

	// Process component metadata
	_, baseComponentName, _, componentIsEnabled := ProcessComponentMetadata(configAndStacksInfo.ComponentFromArg, configAndStacksInfo.ComponentSection)
	configAndStacksInfo.BaseComponentPath = baseComponentName
	configAndStacksInfo.ComponentIsEnabled = componentIsEnabled

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
	configAndStacksInfo.ComponentFolderPrefixReplaced = strings.Replace(configAndStacksInfo.ComponentFolderPrefix, "/", "-", -1)

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
		configAndStacksInfo.ComponentFolderPrefixReplaced = strings.Replace(configAndStacksInfo.ComponentFolderPrefix, "/", "-", -1)
	}

	// Get the final component
	if len(configAndStacksInfo.BaseComponent) > 0 {
		configAndStacksInfo.FinalComponent = configAndStacksInfo.BaseComponent
	} else {
		configAndStacksInfo.FinalComponent = configAndStacksInfo.Component
	}

	// Add component info, including Terraform config
	componentInfo := map[string]any{}
	componentInfo["component_type"] = configAndStacksInfo.ComponentType

	if configAndStacksInfo.ComponentType == "terraform" {
		componentPath := constructTerraformComponentWorkingDir(cliConfig, configAndStacksInfo)
		componentInfo["component_path"] = componentPath
		terraformConfiguration, _ := tfconfig.LoadModule(componentPath)
		componentInfo["terraform_config"] = terraformConfiguration
	} else if configAndStacksInfo.ComponentType == "helmfile" {
		componentInfo["component_path"] = constructHelmfileComponentWorkingDir(cliConfig, configAndStacksInfo)
	}

	configAndStacksInfo.ComponentSection["component_info"] = componentInfo

	return configAndStacksInfo, nil
}

// processArgsAndFlags processes args and flags from the provided CLI arguments/flags
func processArgsAndFlags(componentType string, inputArgsAndFlags []string) (schema.ArgsAndFlagsInfo, error) {
	var info schema.ArgsAndFlagsInfo
	var additionalArgsAndFlags []string
	var globalOptions []string
	var indexesToRemove []int

	// For commands like `atmos terraform clean` and `atmos terraform plan`, show the command help
	if len(inputArgsAndFlags) == 1 {
		info.SubCommand = inputArgsAndFlags[0]
		info.NeedHelp = true
		return info, nil
	}

	// https://github.com/roboll/helmfile#cli-reference
	var globalOptionsFlagIndex int

	// For commands like `atmos terraform clean` and `atmos terraform plan`, show the command help
	if len(inputArgsAndFlags) == 1 {
		info.SubCommand = inputArgsAndFlags[0]
		info.NeedHelp = true
		return info, nil
	}

	for i, arg := range inputArgsAndFlags {
		if arg == cfg.GlobalOptionsFlag {
			globalOptionsFlagIndex = i + 1
		} else if strings.HasPrefix(arg+"=", cfg.GlobalOptionsFlag) {
			globalOptionsFlagIndex = i
		}

		if arg == cfg.TerraformCommandFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, fmt.Errorf("invalid flag: %s", arg)
			}
			info.TerraformCommand = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", cfg.TerraformCommandFlag) {
			var terraformCommandFlagParts = strings.Split(arg, "=")
			if len(terraformCommandFlagParts) != 2 {
				return info, fmt.Errorf("invalid flag: %s", arg)
			}
			info.TerraformCommand = terraformCommandFlagParts[1]
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

		if arg == cfg.AppendUserAgentFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, fmt.Errorf("invalid flag: %s", arg)
			}
			info.AppendUserAgent = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", cfg.AppendUserAgentFlag) {
			var appendUserAgentFlagParts = strings.Split(arg, "=")
			if len(appendUserAgentFlagParts) != 2 {
				return info, fmt.Errorf("invalid flag: %s", arg)
			}
			info.AppendUserAgent = appendUserAgentFlagParts[1]
		}

		if arg == cfg.HelmfileCommandFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, fmt.Errorf("invalid flag: %s", arg)
			}
			info.HelmfileCommand = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", cfg.HelmfileCommandFlag) {
			var helmfileCommandFlagParts = strings.Split(arg, "=")
			if len(helmfileCommandFlagParts) != 2 {
				return info, fmt.Errorf("invalid flag: %s", arg)
			}
			info.HelmfileCommand = helmfileCommandFlagParts[1]
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

		if arg == cfg.VendorBasePathFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, fmt.Errorf("invalid flag: %s", arg)
			}
			info.VendorBasePath = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", cfg.VendorBasePathFlag) {
			var vendorBasePathFlagParts = strings.Split(arg, "=")
			if len(vendorBasePathFlagParts) != 2 {
				return info, fmt.Errorf("invalid flag: %s", arg)
			}
			info.VendorBasePath = vendorBasePathFlagParts[1]
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

		if arg == cfg.AtmosManifestJsonSchemaFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, fmt.Errorf("invalid flag: %s", arg)
			}
			info.AtmosManifestJsonSchema = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", cfg.AtmosManifestJsonSchemaFlag) {
			var atmosManifestJsonSchemaFlagParts = strings.Split(arg, "=")
			if len(atmosManifestJsonSchemaFlagParts) != 2 {
				return info, fmt.Errorf("invalid flag: %s", arg)
			}
			info.AtmosManifestJsonSchema = atmosManifestJsonSchemaFlagParts[1]
		}

		if arg == cfg.RedirectStdErrFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, fmt.Errorf("invalid flag: %s", arg)
			}
			info.RedirectStdErr = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", cfg.RedirectStdErrFlag) {
			var redirectStderrParts = strings.Split(arg, "=")
			if len(redirectStderrParts) != 2 {
				return info, fmt.Errorf("invalid flag: %s", arg)
			}
			info.RedirectStdErr = redirectStderrParts[1]
		}

		if arg == cfg.PlanFileFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, fmt.Errorf("invalid flag: %s", arg)
			}
			info.PlanFile = inputArgsAndFlags[i+1]
			info.UseTerraformPlan = true
		} else if strings.HasPrefix(arg+"=", cfg.PlanFileFlag) {
			var planFileFlagParts = strings.Split(arg, "=")
			if len(planFileFlagParts) != 2 {
				return info, fmt.Errorf("invalid flag: %s", arg)
			}
			info.PlanFile = planFileFlagParts[1]
			info.UseTerraformPlan = true
		}

		if arg == cfg.LogsLevelFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, fmt.Errorf("invalid flag: %s", arg)
			}
			info.LogsLevel = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", cfg.LogsLevelFlag) {
			var logsLevelFlagParts = strings.Split(arg, "=")
			if len(logsLevelFlagParts) != 2 {
				return info, fmt.Errorf("invalid flag: %s", arg)
			}
			info.LogsLevel = logsLevelFlagParts[1]
		}

		if arg == cfg.LogsFileFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, fmt.Errorf("invalid flag: %s", arg)
			}
			info.LogsFile = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", cfg.LogsFileFlag) {
			var logsFileFlagParts = strings.Split(arg, "=")
			if len(logsFileFlagParts) != 2 {
				return info, fmt.Errorf("invalid flag: %s", arg)
			}
			info.LogsFile = logsFileFlagParts[1]
		}

		if arg == cfg.SettingsListMergeStrategyFlag {
			if len(inputArgsAndFlags) <= (i + 1) {
				return info, fmt.Errorf("invalid flag: %s", arg)
			}
			info.SettingsListMergeStrategy = inputArgsAndFlags[i+1]
		} else if strings.HasPrefix(arg+"=", cfg.SettingsListMergeStrategyFlag) {
			var settingsListMergeStrategyParts = strings.Split(arg, "=")
			if len(settingsListMergeStrategyParts) != 2 {
				return info, fmt.Errorf("invalid flag: %s", arg)
			}
			info.SettingsListMergeStrategy = settingsListMergeStrategyParts[1]
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

		if arg == cfg.HelpFlag1 || arg == cfg.HelpFlag2 || arg == cfg.HelpFlag3 {
			info.NeedHelp = true
			// For help commands, we don't need a component or stack
			info.ComponentFromArg = ""
			return info, nil
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
		// https://developer.hashicorp.com/terraform/cli/commands
		if componentType == "terraform" {
			// Handle the custom legacy command `terraform write varfile` (NOTE: use `terraform generate varfile` instead)
			if additionalArgsAndFlags[0] == "write" && additionalArgsAndFlags[1] == "varfile" {
				info.SubCommand = "write"
				info.SubCommand2 = "varfile"
				twoWordsCommand = true
			}

			// `terraform workspace` commands
			// https://developer.hashicorp.com/terraform/cli/commands/workspace
			if additionalArgsAndFlags[0] == "workspace" &&
				u.SliceContainsString([]string{"list", "select", "new", "delete", "show"}, additionalArgsAndFlags[1]) {
				info.SubCommand = "workspace"
				info.SubCommand2 = additionalArgsAndFlags[1]
				twoWordsCommand = true
			}

			// `terraform state` commands
			// https://developer.hashicorp.com/terraform/cli/commands/state
			if additionalArgsAndFlags[0] == "state" &&
				u.SliceContainsString([]string{"list", "mv", "pull", "push", "replace-provider", "rm", "show"}, additionalArgsAndFlags[1]) {
				info.SubCommand = fmt.Sprintf("state %s", additionalArgsAndFlags[1])
				twoWordsCommand = true
			}
		}

		if twoWordsCommand {
			if len(additionalArgsAndFlags) > 2 {
				info.ComponentFromArg = additionalArgsAndFlags[2]
			} else {
				return info, fmt.Errorf("command \"%s\" requires an argument", info.SubCommand)
			}
			if len(additionalArgsAndFlags) > 3 {
				info.AdditionalArgsAndFlags = additionalArgsAndFlags[3:]
			}
		} else {
			info.SubCommand = additionalArgsAndFlags[0]
			if len(additionalArgsAndFlags) > 1 {
				secondArg := additionalArgsAndFlags[1]
				if len(secondArg) == 0 {
					return info, fmt.Errorf("invalid empty argument provided")
				}
				if strings.HasPrefix(secondArg, "--") {
					if len(secondArg) <= 2 {
						return info, fmt.Errorf("invalid option format: %s", secondArg)
					}
					info.AdditionalArgsAndFlags = []string{secondArg}
				} else {
					info.ComponentFromArg = secondArg
				}
			}
			if len(additionalArgsAndFlags) > 2 {
				info.AdditionalArgsAndFlags = additionalArgsAndFlags[2:]
			}
		}
	}

	return info, nil
}

// generateComponentBackendConfig generates backend config for components
func generateComponentBackendConfig(backendType string, backendConfig map[string]any, terraformWorkspace string) (map[string]any, error) {

	// Generate backend config file for Terraform Cloud
	// https://developer.hashicorp.com/terraform/cli/cloud/settings
	if backendType == "cloud" {
		var backendConfigFinal = backendConfig

		if terraformWorkspace != "" {
			// Process template tokens in the backend config
			backendConfigStr, err := u.ConvertToYAML(backendConfig)
			if err != nil {
				return nil, err
			}

			ctx := schema.Context{
				TerraformWorkspace: terraformWorkspace,
			}

			backendConfigStrReplaced := cfg.ReplaceContextTokens(ctx, backendConfigStr)

			backendConfigFinal, err = u.UnmarshalYAML[schema.AtmosSectionMapType](backendConfigStrReplaced)
			if err != nil {
				return nil, err
			}
		}

		return map[string]any{
			"terraform": map[string]any{
				"cloud": backendConfigFinal,
			},
		}, nil
	}

	// Generate backend config file for all other Terraform backends
	return map[string]any{
		"terraform": map[string]any{
			"backend": map[string]any{
				backendType: backendConfig,
			},
		},
	}, nil
}

// generateComponentProviderOverrides generates provider overrides for components
func generateComponentProviderOverrides(providerOverrides map[string]any) map[string]any {
	return map[string]any{
		"provider": providerOverrides,
	}
}

// FindComponentDependencies finds all imports that the component depends on, and all imports that the component has any sections defined in
func FindComponentDependencies(currentStack string, sources schema.ConfigSources) ([]string, []string, error) {
	var deps []string
	var depsAll []string

	for _, source := range sources {
		for _, v := range source {
			for i, dep := range v.StackDependencies {
				if dep.StackFile != "" {
					depsAll = append(depsAll, dep.StackFile)
					if i == 0 {
						deps = append(deps, dep.StackFile)
					}
				}
			}
		}
	}

	depsAll = append(depsAll, currentStack)
	unique := u.UniqueStrings(deps)
	uniqueAll := u.UniqueStrings(depsAll)
	sort.Strings(unique)
	sort.Strings(uniqueAll)
	return unique, uniqueAll, nil
}
