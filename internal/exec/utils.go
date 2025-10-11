package exec

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-config-inspect/tfconfig"
	"github.com/mitchellh/mapstructure"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ProcessComponentConfig processes component config sections.
func ProcessComponentConfig(
	configAndStacksInfo *schema.ConfigAndStacksInfo,
	stack string,
	stacksMap map[string]any,
	componentType string,
	component string,
) error {
	defer perf.Track(nil, "exec.ProcessComponentConfig")()

	var stackSection map[string]any
	var componentsSection map[string]any
	var componentTypeSection map[string]any
	var componentSection map[string]any
	var componentVarsSection map[string]any
	var componentSettingsSection map[string]any
	var componentOverridesSection map[string]any
	var componentProvidersSection map[string]any
	var componentHooksSection map[string]any
	var componentImportsSection []string
	var componentEnvSection map[string]any
	var componentAuthSection map[string]any
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

	if componentHooksSection, ok = componentSection[cfg.HooksSectionName].(map[string]any); !ok {
		componentHooksSection = map[string]any{}
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

	if componentAuthSection, ok = componentSection[cfg.AuthSectionName].(map[string]any); !ok {
		componentAuthSection = map[string]any{}
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

	// Process component metadata and find a base component (if any) and whether the component is real or abstract.
	componentMetadata, baseComponentName, componentIsAbstract, componentIsEnabled, componentIsLocked := ProcessComponentMetadata(component, componentSection)
	configAndStacksInfo.ComponentIsEnabled = componentIsEnabled
	configAndStacksInfo.ComponentIsLocked = componentIsLocked

	// Remove the ENV vars that are set to `null` in the `env` section.
	// Setting an ENV var to `null` in stack config has the effect of unsetting it.
	// This is because the exec.Command, which sets these ENV vars, is itself executed in a separate process started by the os.StartProcess function.
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
	configAndStacksInfo.ComponentHooksSection = componentHooksSection
	configAndStacksInfo.ComponentEnvSection = componentEnvSectionFiltered
	configAndStacksInfo.ComponentAuthSection = componentAuthSection
	configAndStacksInfo.ComponentBackendSection = componentBackendSection
	configAndStacksInfo.ComponentBackendType = componentBackendType
	configAndStacksInfo.BaseComponentPath = baseComponentName
	configAndStacksInfo.ComponentInheritanceChain = componentInheritanceChain
	configAndStacksInfo.ComponentIsAbstract = componentIsAbstract
	configAndStacksInfo.ComponentMetadataSection = componentMetadata
	configAndStacksInfo.ComponentImportsSection = componentImportsSection

	if command != "" {
		configAndStacksInfo.Command = command
	}

	return nil
}

// FindStacksMap processes stack config and returns a map of all stacks.
func FindStacksMap(atmosConfig *schema.AtmosConfiguration, ignoreMissingFiles bool) (
	map[string]any,
	map[string]map[string]any,
	error,
) {
	defer perf.Track(atmosConfig, "exec.FindStacksMap")()

	// Process stack config file(s).
	_, stacksMap, rawStackConfigs, err := ProcessYAMLConfigFiles(
		atmosConfig,
		atmosConfig.StacksBaseAbsolutePath,
		atmosConfig.TerraformDirAbsolutePath,
		atmosConfig.HelmfileDirAbsolutePath,
		atmosConfig.PackerDirAbsolutePath,
		atmosConfig.StackConfigFilesAbsolutePaths,
		false,
		true,
		ignoreMissingFiles,
	)
	if err != nil {
		return nil, nil, err
	}

	return stacksMap, rawStackConfigs, nil
}

// ProcessStacks processes stack config.
func ProcessStacks(
	atmosConfig *schema.AtmosConfiguration,
	configAndStacksInfo schema.ConfigAndStacksInfo,
	checkStack bool,
	processTemplates bool,
	processYamlFunctions bool,
	skip []string,
) (schema.ConfigAndStacksInfo, error) {
	defer perf.Track(atmosConfig, "exec.ProcessStacks")()

	// Check if stack was provided.
	if checkStack && len(configAndStacksInfo.Stack) < 1 {
		return configAndStacksInfo, errUtils.ErrMissingStack
	}

	// Check if the component was provided.
	if len(configAndStacksInfo.ComponentFromArg) < 1 {
		message := fmt.Sprintf("`component` is required.\n\nUsage:\n\n`atmos %s <command> <component> <arguments_and_flags>`", configAndStacksInfo.ComponentType)
		return configAndStacksInfo, errors.New(message)
	}

	configAndStacksInfo.StackFromArg = configAndStacksInfo.Stack

	stacksMap, rawStackConfigs, err := FindStacksMap(atmosConfig, false)
	if err != nil {
		return configAndStacksInfo, err
	}

	// Print the stack config files.
	if atmosConfig.Logs.Level == u.LogLevelTrace {
		var msg string
		if atmosConfig.StackType == "Directory" {
			msg = "\nFound stack manifest:"
		} else {
			msg = "\nFound stack manifests:"
		}
		log.Debug(msg)
		err = u.PrintAsYAMLToFileDescriptor(atmosConfig, atmosConfig.StackConfigFilesRelativePaths)
		if err != nil {
			return configAndStacksInfo, err
		}
	}

	// Check and process stacks.
	if atmosConfig.StackType == "Directory" {
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

		// Process context.
		configAndStacksInfo.Context = cfg.GetContextFromVars(configAndStacksInfo.ComponentVarsSection)
		configAndStacksInfo.Context.Component = configAndStacksInfo.ComponentFromArg
		configAndStacksInfo.Context.BaseComponent = configAndStacksInfo.BaseComponentPath

		configAndStacksInfo.ContextPrefix, err = cfg.GetContextPrefix(configAndStacksInfo.Stack,
			configAndStacksInfo.Context,
			GetStackNamePattern(atmosConfig),
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
			// Check if we've found the component in the stack.
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

			if atmosConfig.Stacks.NameTemplate != "" {
				tmpl, err2 := ProcessTmpl(atmosConfig, "name-template", atmosConfig.Stacks.NameTemplate, configAndStacksInfo.ComponentSection, false)
				if err2 != nil {
					continue
				}
				configAndStacksInfo.ContextPrefix = tmpl
			} else if atmosConfig.Stacks.NamePattern != "" {
				// Process context.
				configAndStacksInfo.Context = cfg.GetContextFromVars(configAndStacksInfo.ComponentVarsSection)

				configAndStacksInfo.ContextPrefix, err = cfg.GetContextPrefix(configAndStacksInfo.Stack,
					configAndStacksInfo.Context,
					GetStackNamePattern(atmosConfig),
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

			// Check if we've found the stack.
			if configAndStacksInfo.Stack == configAndStacksInfo.ContextPrefix {
				configAndStacksInfo.StackFile = stackName
				foundConfigAndStacksInfo = configAndStacksInfo
				foundStackCount++
				foundStacks = append(foundStacks, stackName)

				log.Debug(
					fmt.Sprintf("Found component '%s' in the stack '%s' in the stack manifest '%s'",
						configAndStacksInfo.ComponentFromArg,
						configAndStacksInfo.Stack,
						stackName,
					))
			}
		}

		if foundStackCount == 0 && !checkStack {
			// Allow proceeding without error if checkStack is false (e.g., for operations that don't require a stack).
			return configAndStacksInfo, nil
		}

		if foundStackCount == 0 {
			cliConfigYaml := ""

			if atmosConfig.Logs.Level == u.LogLevelTrace {
				y, _ := u.ConvertToYAML(atmosConfig)
				cliConfigYaml = fmt.Sprintf("\n\n\nCLI config: %v\n", y)
			}

			return configAndStacksInfo,
				fmt.Errorf("%w: Could not find the component `%s` in the stack `%s`.\n"+
					"Check that all the context variables are correctly defined in the stack manifests.\n"+
					"Are the component and stack names correct? Did you forget an import?%v",
					errUtils.ErrInvalidComponent,
					configAndStacksInfo.ComponentFromArg,
					configAndStacksInfo.Stack,
					cliConfigYaml)
		} else if foundStackCount > 1 {
			err = fmt.Errorf("%w: Found duplicate config for the component `%s` in the stack `%s` in the manifests: %v\n"+
				"Check that all the context variables are correctly defined in the manifests and not duplicated\n"+
				"Check that all imports are valid",
				errUtils.ErrInvalidComponent,
				configAndStacksInfo.ComponentFromArg,
				configAndStacksInfo.Stack,
				strings.Join(foundStacks, ", "),
			)
			errUtils.CheckErrorPrintAndExit(err, "", "")
		} else {
			configAndStacksInfo = foundConfigAndStacksInfo
		}
	}

	if configAndStacksInfo.ComponentSection == nil {
		configAndStacksInfo.ComponentSection = make(map[string]any)
	}

	// Add imports.
	configAndStacksInfo.ComponentSection["imports"] = configAndStacksInfo.ComponentImportsSection

	// Add Atmos component and stack.
	configAndStacksInfo.ComponentSection["atmos_component"] = configAndStacksInfo.ComponentFromArg
	configAndStacksInfo.ComponentSection["atmos_stack"] = configAndStacksInfo.StackFromArg
	configAndStacksInfo.ComponentSection["stack"] = configAndStacksInfo.StackFromArg
	configAndStacksInfo.ComponentSection["atmos_stack_file"] = configAndStacksInfo.StackFile
	configAndStacksInfo.ComponentSection["atmos_manifest"] = configAndStacksInfo.StackFile

	// If the command-line component does not inherit anything, then the Terraform/Helmfile component is the same as the provided one.
	if comp, ok := configAndStacksInfo.ComponentSection[cfg.ComponentSectionName].(string); !ok || comp == "" {
		configAndStacksInfo.ComponentSection[cfg.ComponentSectionName] = configAndStacksInfo.ComponentFromArg
	}

	// `sources` (stack config files where the variables and other settings are defined).
	sources, err := ProcessConfigSources(configAndStacksInfo, rawStackConfigs)
	if err != nil {
		return configAndStacksInfo, err
	}

	configAndStacksInfo.ComponentSection["sources"] = sources

	// Component dependencies.
	componentDeps, componentDepsAll, err := FindComponentDependencies(configAndStacksInfo.StackFile, sources)
	if err != nil {
		return configAndStacksInfo, err
	}
	configAndStacksInfo.ComponentSection["deps"] = componentDeps
	configAndStacksInfo.ComponentSection["deps_all"] = componentDepsAll

	// Terraform workspace.
	workspace, err := BuildTerraformWorkspace(atmosConfig, configAndStacksInfo)
	if err != nil {
		return configAndStacksInfo, err
	}

	configAndStacksInfo.TerraformWorkspace = workspace
	configAndStacksInfo.ComponentSection["workspace"] = workspace

	// Process `Go` templates in Atmos manifest sections.
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
			atmosConfig,
			&configAndStacksInfo,
			settingsSectionStruct,
			"templates-all-atmos-sections",
			componentSectionStr,
			configAndStacksInfo.ComponentSection,
			true,
		)
		if err != nil {
			// If any error returned from the template processing, log it and exit.
			errUtils.CheckErrorPrintAndExit(err, "", "")
		}

		componentSectionConverted, err := u.UnmarshalYAML[schema.AtmosSectionMapType](componentSectionProcessed)
		if err != nil {
			if !atmosConfig.Templates.Settings.Enabled {
				if strings.Contains(componentSectionStr, "{{") || strings.Contains(componentSectionStr, "}}") {
					errorMessage := "the stack manifests contain Go templates, but templating is disabled in atmos.yaml in 'templates.settings.enabled'\n" +
						"to enable templating, refer to https://atmos.tools/core-concepts/stacks/templates"
					err = errors.Join(err, errors.New(errorMessage))
				}
			}
			errUtils.CheckErrorPrintAndExit(err, "", "")
		}

		configAndStacksInfo.ComponentSection = componentSectionConverted
	}

	// Process YAML functions in Atmos manifest sections.
	if processYamlFunctions {
		componentSectionConverted, err := ProcessCustomYamlTags(atmosConfig, configAndStacksInfo.ComponentSection, configAndStacksInfo.Stack, skip)
		if err != nil {
			return configAndStacksInfo, err
		}

		configAndStacksInfo.ComponentSection = componentSectionConverted
	}

	if processTemplates || processYamlFunctions {
		postProcessTemplatesAndYamlFunctions(&configAndStacksInfo)
	}

	// Spacelift stack.
	spaceliftStackName, err := BuildSpaceliftStackNameFromComponentConfig(atmosConfig, configAndStacksInfo)
	if err != nil {
		return configAndStacksInfo, err
	}
	if spaceliftStackName != "" {
		configAndStacksInfo.ComponentSection["spacelift_stack"] = spaceliftStackName
	}

	// Atlantis project.
	atlantisProjectName, err := BuildAtlantisProjectNameFromComponentConfig(atmosConfig, configAndStacksInfo)
	if err != nil {
		return configAndStacksInfo, err
	}
	if atlantisProjectName != "" {
		configAndStacksInfo.ComponentSection["atlantis_project"] = atlantisProjectName
	}

	// Process the ENV variables from the `env` section.
	configAndStacksInfo.ComponentEnvList = u.ConvertEnvVars(configAndStacksInfo.ComponentEnvSection)

	// Process component metadata.
	_, baseComponentName, _, componentIsEnabled, componentIsLocked := ProcessComponentMetadata(configAndStacksInfo.ComponentFromArg, configAndStacksInfo.ComponentSection)
	configAndStacksInfo.BaseComponentPath = baseComponentName
	configAndStacksInfo.ComponentIsEnabled = componentIsEnabled
	configAndStacksInfo.ComponentIsLocked = componentIsLocked

	// Process component path and name.
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

	// Process base component path and name.
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

	// Get the final component.
	if len(configAndStacksInfo.BaseComponent) > 0 {
		configAndStacksInfo.FinalComponent = configAndStacksInfo.BaseComponent
	} else {
		configAndStacksInfo.FinalComponent = configAndStacksInfo.Component
	}

	// Add component info, including Terraform config.
	componentInfo := map[string]any{}
	componentInfo["component_type"] = configAndStacksInfo.ComponentType

	switch configAndStacksInfo.ComponentType {
	case cfg.TerraformComponentType:
		componentPath := constructTerraformComponentWorkingDir(atmosConfig, &configAndStacksInfo)
		componentInfo[cfg.ComponentPathSectionName] = componentPath
		terraformConfiguration, _ := tfconfig.LoadModule(componentPath)
		componentInfo["terraform_config"] = terraformConfiguration
	case cfg.HelmfileComponentType:
		componentInfo[cfg.ComponentPathSectionName] = constructHelmfileComponentWorkingDir(atmosConfig, &configAndStacksInfo)
	case cfg.PackerComponentType:
		componentInfo[cfg.ComponentPathSectionName] = constructPackerComponentWorkingDir(atmosConfig, &configAndStacksInfo)
	}

	configAndStacksInfo.ComponentSection["component_info"] = componentInfo

	// Add command-line arguments and vars to the component section.
	// It will allow using them when validating with OPA policies or JSON Schema.
	args := append(configAndStacksInfo.CliArgs, configAndStacksInfo.AdditionalArgsAndFlags...)

	var filteredArgs []string
	for _, item := range args {
		if item != "" {
			filteredArgs = append(filteredArgs, item)
		}
	}

	configAndStacksInfo.ComponentSection[cfg.CliArgsSectionName] = filteredArgs

	cliVars, err := getCliVars(configAndStacksInfo.AdditionalArgsAndFlags)
	if err != nil {
		return configAndStacksInfo, err
	}
	configAndStacksInfo.ComponentSection[cfg.TerraformCliVarsSectionName] = cliVars

	// Add TF_CLI_ARGS arguments and variables to the component section.
	tfEnvCliArgs := GetTerraformEnvCliArgs()
	if len(tfEnvCliArgs) > 0 {
		configAndStacksInfo.ComponentSection[cfg.TerraformCliArgsEnvSectionName] = tfEnvCliArgs
	}

	tfEnvCliVars, err := GetTerraformEnvCliVars()
	if err != nil {
		return configAndStacksInfo, err
	}
	if len(tfEnvCliVars) > 0 {
		configAndStacksInfo.ComponentSection[cfg.TerraformCliVarsEnvSectionName] = tfEnvCliVars
	}

	// Add Atmos CLI config.
	atmosCliConfig := map[string]any{}
	atmosCliConfig["base_path"] = atmosConfig.BasePath
	atmosCliConfig["components"] = atmosConfig.Components
	atmosCliConfig["stacks"] = atmosConfig.Stacks
	atmosCliConfig["workflows"] = atmosConfig.Workflows
	configAndStacksInfo.ComponentSection["atmos_cli_config"] = atmosCliConfig

	return configAndStacksInfo, nil
}

// generateComponentBackendConfig generates backend config for components.
func generateComponentBackendConfig(backendType string, backendConfig map[string]any, terraformWorkspace string) (map[string]any, error) {
	// Generate backend config file for Terraform Cloud.
	// https://developer.hashicorp.com/terraform/cli/cloud/settings
	if backendType == "cloud" {
		backendConfigFinal := backendConfig

		if terraformWorkspace != "" {
			// Process template tokens in the backend config.
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

	// Generate backend config file for all other Terraform backends.
	return map[string]any{
		"terraform": map[string]any{
			"backend": map[string]any{
				backendType: backendConfig,
			},
		},
	}, nil
}

// generateComponentProviderOverrides generates provider overrides for components.
func generateComponentProviderOverrides(providerOverrides map[string]any) map[string]any {
	return map[string]any{
		"provider": providerOverrides,
	}
}

// FindComponentDependencies finds all imports that the component depends on, and all imports that the component has any sections defined in.
func FindComponentDependencies(currentStack string, sources schema.ConfigSources) ([]string, []string, error) {
	defer perf.Track(nil, "exec.FindComponentDependencies")()

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

// postProcessTemplatesAndYamlFunctions restores Atmos sections after processing `Go` templates and custom YAML functions/tags.
func postProcessTemplatesAndYamlFunctions(configAndStacksInfo *schema.ConfigAndStacksInfo) {
	if i, ok := configAndStacksInfo.ComponentSection[cfg.ProvidersSectionName].(map[string]any); ok {
		configAndStacksInfo.ComponentProvidersSection = i
	}

	if i, ok := configAndStacksInfo.ComponentSection[cfg.AuthSectionName].(map[string]any); ok {
		configAndStacksInfo.ComponentAuthSection = i
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

	if i, ok := configAndStacksInfo.ComponentSection[cfg.WorkspaceSectionName].(string); ok {
		configAndStacksInfo.TerraformWorkspace = i
	}
}
