package exec

import (
	"errors"
	"fmt"
	"strings"

	"github.com/go-viper/mapstructure/v2"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/pager"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

type DescribeStacksArgs struct {
	Query                string
	FilterByStack        string
	Components           []string
	ComponentTypes       []string
	Sections             []string
	IgnoreMissingFiles   bool
	ProcessTemplates     bool
	ProcessYamlFunctions bool
	IncludeEmptyStacks   bool
	Skip                 []string
	Format               string
	File                 string
	AuthManager          auth.AuthManager // Optional: Auth manager for credential management (from --identity flag).
}

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_$GOFILE -package=$GOPACKAGE
type DescribeStacksExec interface {
	Execute(atmosConfig *schema.AtmosConfiguration, args *DescribeStacksArgs) error
}

type describeStacksExec struct {
	pageCreator           pager.PageCreator
	isTTYSupportForStdout func() bool
	printOrWriteToFile    func(atmosConfig *schema.AtmosConfiguration, format string, file string, data any) error
	executeDescribeStacks func(
		atmosConfig *schema.AtmosConfiguration,
		filterByStack string,
		components []string,
		componentTypes []string,
		sections []string,
		ignoreMissingFiles bool,
		processTemplates bool,
		processYamlFunctions bool,
		includeEmptyStacks bool,
		skip []string,
		authManager auth.AuthManager,
	) (map[string]any, error)
}

func NewDescribeStacksExec() DescribeStacksExec {
	defer perf.Track(nil, "exec.NewDescribeStacksExec")()

	return &describeStacksExec{
		pageCreator:           pager.New(),
		isTTYSupportForStdout: term.IsTTYSupportForStdout,
		printOrWriteToFile:    printOrWriteToFile,
		executeDescribeStacks: ExecuteDescribeStacks,
	}
}

// Execute executes `describe stacks` command.
func (d *describeStacksExec) Execute(atmosConfig *schema.AtmosConfiguration, args *DescribeStacksArgs) error {
	defer perf.Track(atmosConfig, "exec.DescribeStacksExec.Execute")()

	finalStacksMap, err := d.executeDescribeStacks(
		atmosConfig,
		args.FilterByStack,
		args.Components,
		args.ComponentTypes,
		args.Sections,
		false,
		args.ProcessTemplates,
		args.ProcessYamlFunctions,
		args.IncludeEmptyStacks,
		args.Skip,
		args.AuthManager,
	)
	if err != nil {
		return err
	}

	var res any

	if args.Query != "" {
		res, err = u.EvaluateYqExpression(atmosConfig, finalStacksMap, args.Query)
		if err != nil {
			return err
		}
	} else {
		res = finalStacksMap
	}

	return viewWithScroll(&viewWithScrollProps{
		pageCreator:           d.pageCreator,
		isTTYSupportForStdout: d.isTTYSupportForStdout,
		printOrWriteToFile:    d.printOrWriteToFile,
		atmosConfig:           atmosConfig,
		displayName:           "Stacks",
		format:                args.Format,
		file:                  args.File,
		res:                   res,
	})
}

// ExecuteDescribeStacks processes stack manifests and returns the final map of stacks and components.
func ExecuteDescribeStacks(
	atmosConfig *schema.AtmosConfiguration,
	filterByStack string,
	components []string,
	componentTypes []string,
	sections []string,
	ignoreMissingFiles bool,
	processTemplates bool,
	processYamlFunctions bool,
	includeEmptyStacks bool,
	skip []string,
	authManager auth.AuthManager,
) (map[string]any, error) {
	defer perf.Track(atmosConfig, "exec.ExecuteDescribeStacks")()

	stacksMap, _, err := FindStacksMap(atmosConfig, ignoreMissingFiles)
	if err != nil {
		return nil, err
	}

	finalStacksMap := make(map[string]any)
	processedStacks := make(map[string]bool)
	var varsSection map[string]any
	var metadataSection map[string]any
	var authSection map[string]any
	var settingsSection map[string]any
	var envSection map[string]any
	var providersSection map[string]any
	var hooksSection map[string]any
	var overridesSection map[string]any
	var backendSection map[string]any
	var backendTypeSection string
	var stackName string

	for stackFileName, stackSection := range stacksMap {
		var context schema.Context

		// Delete the stack-wide imports.
		delete(stackSection.(map[string]any), "imports")

		// Check if the `components` section exists and has explicit components.
		hasExplicitComponents := false
		if componentsSection, ok := stackSection.(map[string]any)[cfg.ComponentsSectionName]; ok {
			if componentsSection != nil {
				if terraformSection, ok := componentsSection.(map[string]any)[cfg.TerraformSectionName].(map[string]any); ok {
					hasExplicitComponents = len(terraformSection) > 0
				}
				if helmfileSection, ok := componentsSection.(map[string]any)[cfg.HelmfileSectionName].(map[string]any); ok {
					hasExplicitComponents = hasExplicitComponents || len(helmfileSection) > 0
				}
				if packerSection, ok := componentsSection.(map[string]any)[cfg.PackerSectionName].(map[string]any); ok {
					hasExplicitComponents = hasExplicitComponents || len(packerSection) > 0
				}
			}
		}

		// Also check for imports.
		hasImports := false
		if importsSection, ok := stackSection.(map[string]any)["import"].([]any); ok {
			hasImports = len(importsSection) > 0
		}

		// Skip stacks without components or imports when includeEmptyStacks is false.
		if !includeEmptyStacks && !hasExplicitComponents && !hasImports {
			continue
		}

		stackName = stackFileName
		if processedStacks[stackName] {
			continue
		}
		processedStacks[stackName] = true

		if !u.MapKeyExists(finalStacksMap, stackName) {
			finalStacksMap[stackName] = make(map[string]any)
			finalStacksMap[stackName].(map[string]any)[cfg.ComponentsSectionName] = make(map[string]any)
		}

		if componentsSection, ok := stackSection.(map[string]any)[cfg.ComponentsSectionName].(map[string]any); ok {

			// Terraform.
			if len(componentTypes) == 0 || u.SliceContainsString(componentTypes, cfg.TerraformSectionName) {
				if terraformSection, ok := componentsSection[cfg.TerraformSectionName].(map[string]any); ok {
					for componentName, compSection := range terraformSection {
						componentSection, ok := compSection.(map[string]any)
						if !ok {
							return nil, fmt.Errorf("invalid 'components.terraform.%s' section in the file '%s'", componentName, stackFileName)
						}

						if comp, ok := componentSection[cfg.ComponentSectionName].(string); !ok || comp == "" {
							componentSection[cfg.ComponentSectionName] = componentName
						}

						// Find all derived components of the provided components and include them in the output.
						derivedComponents, err := FindComponentsDerivedFromBaseComponents(stackFileName, terraformSection, components)
						if err != nil {
							return nil, err
						}

						if varsSection, ok = componentSection[cfg.VarsSectionName].(map[string]any); !ok {
							varsSection = map[string]any{}
						}

						if metadataSection, ok = componentSection[cfg.MetadataSectionName].(map[string]any); !ok {
							metadataSection = map[string]any{}
						}

						if settingsSection, ok = componentSection[cfg.SettingsSectionName].(map[string]any); !ok {
							settingsSection = map[string]any{}
						}

						if envSection, ok = componentSection[cfg.EnvSectionName].(map[string]any); !ok {
							envSection = map[string]any{}
						}

						if authSection, ok = componentSection[cfg.AuthSectionName].(map[string]any); !ok {
							authSection = map[string]any{}
						}

						if providersSection, ok = componentSection[cfg.ProvidersSectionName].(map[string]any); !ok {
							providersSection = map[string]any{}
						}

						if hooksSection, ok = componentSection[cfg.HooksSectionName].(map[string]any); !ok {
							hooksSection = map[string]any{}
						}

						if overridesSection, ok = componentSection[cfg.OverridesSectionName].(map[string]any); !ok {
							overridesSection = map[string]any{}
						}

						if backendSection, ok = componentSection[cfg.BackendSectionName].(map[string]any); !ok {
							backendSection = map[string]any{}
						}

						if backendTypeSection, ok = componentSection[cfg.BackendTypeSectionName].(string); !ok {
							backendTypeSection = ""
						}

						configAndStacksInfo := schema.ConfigAndStacksInfo{
							ComponentFromArg:          componentName,
							Stack:                     stackName,
							ComponentMetadataSection:  metadataSection,
							ComponentVarsSection:      varsSection,
							ComponentSettingsSection:  settingsSection,
							ComponentEnvSection:       envSection,
							ComponentAuthSection:      authSection,
							ComponentProvidersSection: providersSection,
							ComponentHooksSection:     hooksSection,
							ComponentOverridesSection: overridesSection,
							ComponentBackendSection:   backendSection,
							ComponentBackendType:      backendTypeSection,
							ComponentSection: map[string]any{
								cfg.VarsSectionName:        varsSection,
								cfg.MetadataSectionName:    metadataSection,
								cfg.SettingsSectionName:    settingsSection,
								cfg.EnvSectionName:         envSection,
								cfg.AuthSectionName:        authSection,
								cfg.ProvidersSectionName:   providersSection,
								cfg.HooksSectionName:       hooksSection,
								cfg.OverridesSectionName:   overridesSection,
								cfg.BackendSectionName:     backendSection,
								cfg.BackendTypeSectionName: backendTypeSection,
							},
						}

						// Populate AuthContext from AuthManager if provided (from --identity flag).
						if authManager != nil {
							managerStackInfo := authManager.GetStackInfo()
							if managerStackInfo != nil && managerStackInfo.AuthContext != nil {
								configAndStacksInfo.AuthContext = managerStackInfo.AuthContext
							}
						}

						if comp, ok := configAndStacksInfo.ComponentSection[cfg.ComponentSectionName].(string); !ok || comp == "" {
							configAndStacksInfo.ComponentSection[cfg.ComponentSectionName] = componentName
						}

						// Stack name.
						if atmosConfig.Stacks.NameTemplate != "" {
							stackName, err = ProcessTmpl(atmosConfig, "describe-stacks-name-template", atmosConfig.Stacks.NameTemplate, configAndStacksInfo.ComponentSection, false)
							if err != nil {
								return nil, err
							}
						} else {
							context = cfg.GetContextFromVars(varsSection)
							configAndStacksInfo.Context = context
							stackName, err = cfg.GetContextPrefix(stackFileName, context, GetStackNamePattern(atmosConfig), stackFileName)
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

						// Only create the stack entry if it doesn't exist.
						if !u.MapKeyExists(finalStacksMap, stackName) {
							finalStacksMap[stackName] = make(map[string]any)
						}

						configAndStacksInfo.ComponentSection["atmos_component"] = componentName
						configAndStacksInfo.ComponentSection["atmos_stack"] = stackName
						configAndStacksInfo.ComponentSection["stack"] = stackName
						configAndStacksInfo.ComponentSection["atmos_stack_file"] = stackFileName
						configAndStacksInfo.ComponentSection["atmos_manifest"] = stackFileName

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

							// Atmos component, stack, and stack manifest file.
							configAndStacksInfo.Stack = stackName
							componentSection["atmos_component"] = componentName
							componentSection["atmos_stack"] = stackName
							componentSection["stack"] = stackName
							componentSection["atmos_stack_file"] = stackFileName
							componentSection["atmos_manifest"] = stackFileName

							// Terraform workspace.
							workspace, err := BuildTerraformWorkspace(atmosConfig, configAndStacksInfo)
							if err != nil {
								return nil, err
							}
							componentSection["workspace"] = workspace
							configAndStacksInfo.ComponentSection["workspace"] = workspace

							// Process `Go` templates.
							if processTemplates {
								componentSectionStr, err := u.ConvertToYAML(componentSection)
								if err != nil {
									return nil, err
								}

								var settingsSectionStruct schema.Settings
								err = mapstructure.Decode(settingsSection, &settingsSectionStruct)
								if err != nil {
									return nil, err
								}

								componentSectionProcessed, err := ProcessTmplWithDatasources(
									atmosConfig,
									&configAndStacksInfo,
									settingsSectionStruct,
									"describe-stacks-all-sections",
									componentSectionStr,
									configAndStacksInfo.ComponentSection,
									true,
								)
								if err != nil {
									return nil, err
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

								componentSection = componentSectionConverted
							}

							// Process YAML functions.
							if processYamlFunctions {
								componentSectionConverted, err := ProcessCustomYamlTags(
									atmosConfig,
									componentSection,
									configAndStacksInfo.Stack,
									skip,
									&configAndStacksInfo,
								)
								if err != nil {
									return nil, err
								}

								componentSection = componentSectionConverted
							}

							// Check if we should include empty sections.
							includeEmpty := true // Default to true if `setting` is not provided.
							if atmosConfig.Describe.Settings.IncludeEmpty != nil {
								includeEmpty = *atmosConfig.Describe.Settings.IncludeEmpty
							}

							// Add sections.
							for sectionName, section := range componentSection {
								// Skip empty sections if includeEmpty is false.
								if !includeEmpty {
									if sectionMap, ok := section.(map[string]any); ok {
										if len(sectionMap) == 0 {
											continue
										}
									}
								}

								if len(sections) == 0 || u.SliceContainsString(sections, sectionName) {
									finalStacksMap[stackName].(map[string]any)["components"].(map[string]any)["terraform"].(map[string]any)[componentName].(map[string]any)[sectionName] = section
								}
							}
						}
					}
				}
			}

			// Helmfile.
			if len(componentTypes) == 0 || u.SliceContainsString(componentTypes, cfg.HelmfileSectionName) {
				if helmfileSection, ok := componentsSection[cfg.HelmfileSectionName].(map[string]any); ok {
					for componentName, compSection := range helmfileSection {
						componentSection, ok := compSection.(map[string]any)
						if !ok {
							return nil, fmt.Errorf("invalid 'components.helmfile.%s' section in the file '%s'", componentName, stackFileName)
						}

						if comp, ok := componentSection[cfg.ComponentSectionName].(string); !ok || comp == "" {
							componentSection[cfg.ComponentSectionName] = componentName
						}

						// Find all derived components of the provided components and include them in the output.
						derivedComponents, err := FindComponentsDerivedFromBaseComponents(stackFileName, helmfileSection, components)
						if err != nil {
							return nil, err
						}

						if varsSection, ok = componentSection[cfg.VarsSectionName].(map[string]any); !ok {
							varsSection = map[string]any{}
						}

						if metadataSection, ok = componentSection[cfg.MetadataSectionName].(map[string]any); !ok {
							metadataSection = map[string]any{}
						}

						if settingsSection, ok = componentSection[cfg.SettingsSectionName].(map[string]any); !ok {
							settingsSection = map[string]any{}
						}

						if envSection, ok = componentSection[cfg.EnvSectionName].(map[string]any); !ok {
							envSection = map[string]any{}
						}

						if authSection, ok = componentSection[cfg.AuthSectionName].(map[string]any); !ok {
							authSection = map[string]any{}
						}

						if providersSection, ok = componentSection[cfg.ProvidersSectionName].(map[string]any); !ok {
							providersSection = map[string]any{}
						}

						if hooksSection, ok = componentSection[cfg.HooksSectionName].(map[string]any); !ok {
							hooksSection = map[string]any{}
						}

						if overridesSection, ok = componentSection[cfg.OverridesSectionName].(map[string]any); !ok {
							overridesSection = map[string]any{}
						}

						if backendSection, ok = componentSection[cfg.BackendSectionName].(map[string]any); !ok {
							backendSection = map[string]any{}
						}

						if backendTypeSection, ok = componentSection[cfg.BackendTypeSectionName].(string); !ok {
							backendTypeSection = ""
						}

						configAndStacksInfo := schema.ConfigAndStacksInfo{
							ComponentFromArg:          componentName,
							Stack:                     stackName,
							ComponentMetadataSection:  metadataSection,
							ComponentVarsSection:      varsSection,
							ComponentSettingsSection:  settingsSection,
							ComponentEnvSection:       envSection,
							ComponentAuthSection:      authSection,
							ComponentProvidersSection: providersSection,
							ComponentHooksSection:     hooksSection,
							ComponentOverridesSection: overridesSection,
							ComponentBackendSection:   backendSection,
							ComponentBackendType:      backendTypeSection,
							ComponentSection: map[string]any{
								cfg.VarsSectionName:        varsSection,
								cfg.MetadataSectionName:    metadataSection,
								cfg.SettingsSectionName:    settingsSection,
								cfg.EnvSectionName:         envSection,
								cfg.AuthSectionName:        authSection,
								cfg.ProvidersSectionName:   providersSection,
								cfg.HooksSectionName:       hooksSection,
								cfg.OverridesSectionName:   overridesSection,
								cfg.BackendSectionName:     backendSection,
								cfg.BackendTypeSectionName: backendTypeSection,
							},
						}

						// Populate AuthContext from AuthManager if provided (from --identity flag).
						if authManager != nil {
							managerStackInfo := authManager.GetStackInfo()
							if managerStackInfo != nil && managerStackInfo.AuthContext != nil {
								configAndStacksInfo.AuthContext = managerStackInfo.AuthContext
							}
						}

						if comp, ok := configAndStacksInfo.ComponentSection[cfg.ComponentSectionName].(string); !ok || comp == "" {
							configAndStacksInfo.ComponentSection[cfg.ComponentSectionName] = componentName
						}

						// Stack name.
						if atmosConfig.Stacks.NameTemplate != "" {
							stackName, err = ProcessTmpl(atmosConfig, "describe-stacks-name-template", atmosConfig.Stacks.NameTemplate, configAndStacksInfo.ComponentSection, false)
							if err != nil {
								return nil, err
							}
						} else {
							context = cfg.GetContextFromVars(varsSection)
							configAndStacksInfo.Context = context
							stackName, err = cfg.GetContextPrefix(stackFileName, context, GetStackNamePattern(atmosConfig), stackFileName)
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

						// Only create the stack entry if it doesn't exist.
						if !u.MapKeyExists(finalStacksMap, stackName) {
							finalStacksMap[stackName] = make(map[string]any)
						}

						configAndStacksInfo.Stack = stackName
						configAndStacksInfo.ComponentSection["atmos_component"] = componentName
						configAndStacksInfo.ComponentSection["atmos_stack"] = stackName
						configAndStacksInfo.ComponentSection["stack"] = stackName
						configAndStacksInfo.ComponentSection["atmos_stack_file"] = stackFileName
						configAndStacksInfo.ComponentSection["atmos_manifest"] = stackFileName

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

							// Atmos component, stack, and stack manifest file.
							componentSection["atmos_component"] = componentName
							componentSection["atmos_stack"] = stackName
							componentSection["stack"] = stackName
							componentSection["atmos_stack_file"] = stackFileName
							componentSection["atmos_manifest"] = stackFileName

							// Process `Go` templates.
							if processTemplates {
								componentSectionStr, err := u.ConvertToYAML(componentSection)
								if err != nil {
									return nil, err
								}

								var settingsSectionStruct schema.Settings
								err = mapstructure.Decode(settingsSection, &settingsSectionStruct)
								if err != nil {
									return nil, err
								}

								componentSectionProcessed, err := ProcessTmplWithDatasources(
									atmosConfig,
									&configAndStacksInfo,
									settingsSectionStruct,
									"templates-describe-stacks-all-atmos-sections",
									componentSectionStr,
									configAndStacksInfo.ComponentSection,
									true,
								)
								if err != nil {
									return nil, err
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

								componentSection = componentSectionConverted
							}

							// Process YAML functions.
							if processYamlFunctions {
								componentSectionConverted, err := ProcessCustomYamlTags(
									atmosConfig,
									componentSection,
									configAndStacksInfo.Stack,
									skip,
									&configAndStacksInfo,
								)
								if err != nil {
									return nil, err
								}

								componentSection = componentSectionConverted
							}

							// Add sections.
							for sectionName, section := range componentSection {
								if len(sections) == 0 || u.SliceContainsString(sections, sectionName) {
									finalStacksMap[stackName].(map[string]any)["components"].(map[string]any)["helmfile"].(map[string]any)[componentName].(map[string]any)[sectionName] = section
								}
							}
						}
					}
				}
			}

			// Packer.
			if len(componentTypes) == 0 || u.SliceContainsString(componentTypes, cfg.PackerSectionName) {
				if packerSection, ok := componentsSection[cfg.PackerSectionName].(map[string]any); ok {
					for componentName, compSection := range packerSection {
						componentSection, ok := compSection.(map[string]any)
						if !ok {
							return nil, fmt.Errorf("invalid 'components.packer.%s' section in the file '%s'", componentName, stackFileName)
						}

						if comp, ok := componentSection[cfg.ComponentSectionName].(string); !ok || comp == "" {
							componentSection[cfg.ComponentSectionName] = componentName
						}

						// Find all derived components of the provided components and include them in the output.
						derivedComponents, err := FindComponentsDerivedFromBaseComponents(stackFileName, packerSection, components)
						if err != nil {
							return nil, err
						}

						if varsSection, ok = componentSection[cfg.VarsSectionName].(map[string]any); !ok {
							varsSection = map[string]any{}
						}

						if metadataSection, ok = componentSection[cfg.MetadataSectionName].(map[string]any); !ok {
							metadataSection = map[string]any{}
						}

						if settingsSection, ok = componentSection[cfg.SettingsSectionName].(map[string]any); !ok {
							settingsSection = map[string]any{}
						}

						if envSection, ok = componentSection[cfg.EnvSectionName].(map[string]any); !ok {
							envSection = map[string]any{}
						}

						if authSection, ok = componentSection[cfg.AuthSectionName].(map[string]any); !ok {
							authSection = map[string]any{}
						}

						if providersSection, ok = componentSection[cfg.ProvidersSectionName].(map[string]any); !ok {
							providersSection = map[string]any{}
						}

						if hooksSection, ok = componentSection[cfg.HooksSectionName].(map[string]any); !ok {
							hooksSection = map[string]any{}
						}

						if overridesSection, ok = componentSection[cfg.OverridesSectionName].(map[string]any); !ok {
							overridesSection = map[string]any{}
						}

						if backendSection, ok = componentSection[cfg.BackendSectionName].(map[string]any); !ok {
							backendSection = map[string]any{}
						}

						if backendTypeSection, ok = componentSection[cfg.BackendTypeSectionName].(string); !ok {
							backendTypeSection = ""
						}

						configAndStacksInfo := schema.ConfigAndStacksInfo{
							ComponentFromArg:          componentName,
							Stack:                     stackName,
							ComponentMetadataSection:  metadataSection,
							ComponentVarsSection:      varsSection,
							ComponentSettingsSection:  settingsSection,
							ComponentEnvSection:       envSection,
							ComponentAuthSection:      authSection,
							ComponentProvidersSection: providersSection,
							ComponentHooksSection:     hooksSection,
							ComponentOverridesSection: overridesSection,
							ComponentBackendSection:   backendSection,
							ComponentBackendType:      backendTypeSection,
							ComponentSection: map[string]any{
								cfg.VarsSectionName:        varsSection,
								cfg.MetadataSectionName:    metadataSection,
								cfg.SettingsSectionName:    settingsSection,
								cfg.EnvSectionName:         envSection,
								cfg.AuthSectionName:        authSection,
								cfg.ProvidersSectionName:   providersSection,
								cfg.HooksSectionName:       hooksSection,
								cfg.OverridesSectionName:   overridesSection,
								cfg.BackendSectionName:     backendSection,
								cfg.BackendTypeSectionName: backendTypeSection,
							},
						}

						// Populate AuthContext from AuthManager if provided (from --identity flag).
						if authManager != nil {
							managerStackInfo := authManager.GetStackInfo()
							if managerStackInfo != nil && managerStackInfo.AuthContext != nil {
								configAndStacksInfo.AuthContext = managerStackInfo.AuthContext
							}
						}

						if comp, ok := configAndStacksInfo.ComponentSection[cfg.ComponentSectionName].(string); !ok || comp == "" {
							configAndStacksInfo.ComponentSection[cfg.ComponentSectionName] = componentName
						}

						// Stack name.
						if atmosConfig.Stacks.NameTemplate != "" {
							stackName, err = ProcessTmpl(atmosConfig, "describe-stacks-name-template", atmosConfig.Stacks.NameTemplate, configAndStacksInfo.ComponentSection, false)
							if err != nil {
								return nil, err
							}
						} else {
							context = cfg.GetContextFromVars(varsSection)
							configAndStacksInfo.Context = context
							stackName, err = cfg.GetContextPrefix(stackFileName, context, GetStackNamePattern(atmosConfig), stackFileName)
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

						// Only create the stack entry if it doesn't exist.
						if !u.MapKeyExists(finalStacksMap, stackName) {
							finalStacksMap[stackName] = make(map[string]any)
						}

						configAndStacksInfo.Stack = stackName
						configAndStacksInfo.ComponentSection["atmos_component"] = componentName
						configAndStacksInfo.ComponentSection["atmos_stack"] = stackName
						configAndStacksInfo.ComponentSection["stack"] = stackName
						configAndStacksInfo.ComponentSection["atmos_stack_file"] = stackFileName
						configAndStacksInfo.ComponentSection["atmos_manifest"] = stackFileName

						if len(components) == 0 || u.SliceContainsString(components, componentName) || u.SliceContainsString(derivedComponents, componentName) {
							if !u.MapKeyExists(finalStacksMap[stackName].(map[string]any), cfg.ComponentsSectionName) {
								finalStacksMap[stackName].(map[string]any)[cfg.ComponentsSectionName] = make(map[string]any)
							}
							if !u.MapKeyExists(finalStacksMap[stackName].(map[string]any)[cfg.ComponentsSectionName].(map[string]any), cfg.PackerSectionName) {
								finalStacksMap[stackName].(map[string]any)[cfg.ComponentsSectionName].(map[string]any)[cfg.PackerSectionName] = make(map[string]any)
							}
							if !u.MapKeyExists(finalStacksMap[stackName].(map[string]any)[cfg.ComponentsSectionName].(map[string]any)[cfg.PackerSectionName].(map[string]any), componentName) {
								finalStacksMap[stackName].(map[string]any)[cfg.ComponentsSectionName].(map[string]any)[cfg.PackerSectionName].(map[string]any)[componentName] = make(map[string]any)
							}

							// Atmos component, stack, and stack manifest file.
							componentSection["atmos_component"] = componentName
							componentSection["atmos_stack"] = stackName
							componentSection["stack"] = stackName
							componentSection["atmos_stack_file"] = stackFileName
							componentSection["atmos_manifest"] = stackFileName

							// Process `Go` templates.
							if processTemplates {
								componentSectionStr, err := u.ConvertToYAML(componentSection)
								if err != nil {
									return nil, err
								}

								var settingsSectionStruct schema.Settings
								err = mapstructure.Decode(settingsSection, &settingsSectionStruct)
								if err != nil {
									return nil, err
								}

								componentSectionProcessed, err := ProcessTmplWithDatasources(
									atmosConfig,
									&configAndStacksInfo,
									settingsSectionStruct,
									"templates-describe-stacks-all-atmos-sections",
									componentSectionStr,
									configAndStacksInfo.ComponentSection,
									true,
								)
								if err != nil {
									return nil, err
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

								componentSection = componentSectionConverted
							}

							// Process YAML functions.
							if processYamlFunctions {
								componentSectionConverted, err := ProcessCustomYamlTags(
									atmosConfig,
									componentSection,
									configAndStacksInfo.Stack,
									skip,
									&configAndStacksInfo,
								)
								if err != nil {
									return nil, err
								}

								componentSection = componentSectionConverted
							}

							// Add sections.
							for sectionName, section := range componentSection {
								if len(sections) == 0 || u.SliceContainsString(sections, sectionName) {
									finalStacksMap[stackName].(map[string]any)[cfg.ComponentsSectionName].(map[string]any)[cfg.PackerSectionName].(map[string]any)[componentName].(map[string]any)[sectionName] = section
								}
							}
						}
					}
				}
			}
		}
	}

	// Filter out empty stacks after processing all stack files.
	if !includeEmptyStacks {
		for stackName := range finalStacksMap {
			if stackName == "" {
				delete(finalStacksMap, stackName)
				continue
			}

			stackEntry, ok := finalStacksMap[stackName].(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid stack entry type for stack %s", stackName)
			}
			componentsSection, hasComponents := stackEntry["components"].(map[string]any)

			if !hasComponents {
				delete(finalStacksMap, stackName)
				continue
			}

			// Check if any component type (terraform/helmfile/packer) has components.
			hasNonEmptyComponents := false
			for _, components := range componentsSection {
				if compTypeMap, ok := components.(map[string]any); ok {
					for _, comp := range compTypeMap {
						if compContent, ok := comp.(map[string]any); ok {
							// Check for any meaningful content.
							relevantSections := []string{"vars", "metadata", "settings", "env", "workspace"}
							for _, section := range relevantSections {
								if _, hasSection := compContent[section]; hasSection {
									hasNonEmptyComponents = true
									break
								}
							}
						}
					}
				}
				if hasNonEmptyComponents {
					break
				}
			}

			if !hasNonEmptyComponents {
				delete(finalStacksMap, stackName)
				continue
			}
		}
	} else {
		// Process stacks normally without special handling for any prefixes.
		for stackName, stackConfig := range finalStacksMap {
			finalStacksMap[stackName] = stackConfig
		}
	}

	return finalStacksMap, nil
}
