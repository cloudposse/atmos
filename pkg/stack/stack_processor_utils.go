package stack

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/mitchellh/mapstructure"

	cfg "github.com/cloudposse/atmos/pkg/config"
	c "github.com/cloudposse/atmos/pkg/convert"
	m "github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

var (
	getFileContentSyncMap = sync.Map{}
)

// FindComponentStacks finds all infrastructure stack manifests where the component or the base component is defined
func FindComponentStacks(
	componentType string,
	component string,
	baseComponent string,
	componentStackMap map[string]map[string][]string) ([]string, error) {

	var stacks []string

	if componentStackConfig, componentStackConfigExists := componentStackMap[componentType]; componentStackConfigExists {
		if componentStacks, componentStacksExist := componentStackConfig[component]; componentStacksExist {
			stacks = append(stacks, componentStacks...)
		}

		if baseComponent != "" {
			if baseComponentStacks, baseComponentStacksExist := componentStackConfig[baseComponent]; baseComponentStacksExist {
				stacks = append(stacks, baseComponentStacks...)
			}
		}
	}

	unique := u.UniqueStrings(stacks)
	sort.Strings(unique)
	return unique, nil
}

// FindComponentDependenciesLegacy finds all imports where the component or the base component(s) are defined
// Component depends on the imported config file if any of the following conditions is true:
//  1. The imported config file has any of the global `backend`, `backend_type`, `env`, `remote_state_backend`, `remote_state_backend_type`,
//     `settings` or `vars` sections which are not empty.
//  2. The imported config file has the component type section, which has any of the `backend`, `backend_type`, `env`, `remote_state_backend`,
//     `remote_state_backend_type`, `settings` or `vars` sections which are not empty.
//  3. The imported config file has the "components" section, which has the component type section, which has the component section.
//  4. The imported config file has the "components" section, which has the component type section, which has the base component(s) section,
//     and the base component section is defined inline (not imported).
func FindComponentDependenciesLegacy(
	stack string,
	componentType string,
	component string,
	baseComponents []string,
	stackImports map[string]map[any]any) ([]string, error) {

	var deps []string

	sectionsToCheck := []string{
		"backend",
		"backend_type",
		"env",
		"remote_state_backend",
		"remote_state_backend_type",
		"settings",
		"vars",
	}

	for stackImportName, stackImportMap := range stackImports {

		if sectionContainsAnyNotEmptySections(stackImportMap, sectionsToCheck) {
			deps = append(deps, stackImportName)
			continue
		}

		if sectionContainsAnyNotEmptySections(stackImportMap, []string{componentType}) {
			if sectionContainsAnyNotEmptySections(stackImportMap[componentType].(map[any]any), sectionsToCheck) {
				deps = append(deps, stackImportName)
				continue
			}
		}

		stackImportMapComponentsSection, ok := stackImportMap["components"].(map[any]any)
		if !ok {
			continue
		}

		stackImportMapComponentTypeSection, ok := stackImportMapComponentsSection[componentType].(map[any]any)
		if !ok {
			continue
		}

		if stackImportMapComponentSection, ok := stackImportMapComponentTypeSection[component].(map[any]any); ok {
			if len(stackImportMapComponentSection) > 0 {
				deps = append(deps, stackImportName)
				continue
			}
		}

		// Process base component(s)
		// Only include the imported config file into "deps" if all the following conditions are `true`:
		// 1. The imported config file has the base component(s) section(s)
		// 2. The imported config file does not import other config files (which means that instead it defined the base component sections inline)
		// 3. If the imported config file does import other config files, check that the base component sections in them are different by using
		// `reflect.DeepEqual`. If they are the same, don't include the imported config file since it does not specify anything for the base component
		for _, baseComponent := range baseComponents {
			baseComponentSection, ok := stackImportMapComponentTypeSection[baseComponent].(map[any]any)

			if !ok || len(baseComponentSection) == 0 {
				continue
			}

			importOfStackImportStructs, err := processImportSection(stackImportMap, stack)
			if err != nil {
				return nil, err
			}

			if len(importOfStackImportStructs) == 0 {
				deps = append(deps, stackImportName)
				continue
			}

			for _, importOfStackImportStruct := range importOfStackImportStructs {
				importOfStackImportMap, ok := stackImports[importOfStackImportStruct.Path]
				if !ok {
					continue
				}

				importOfStackImportComponentsSection, ok := importOfStackImportMap["components"].(map[any]any)
				if !ok {
					continue
				}

				importOfStackImportComponentTypeSection, ok := importOfStackImportComponentsSection[componentType].(map[any]any)
				if !ok {
					continue
				}

				importOfStackImportBaseComponentSection, ok := importOfStackImportComponentTypeSection[baseComponent].(map[any]any)
				if !ok {
					continue
				}

				if !reflect.DeepEqual(baseComponentSection, importOfStackImportBaseComponentSection) {
					deps = append(deps, stackImportName)
					break
				}
			}
		}
	}

	deps = append(deps, stack)
	unique := u.UniqueStrings(deps)
	sort.Strings(unique)
	return unique, nil
}

// processImportSection processes the `import` section in stack manifests
// The `import` section` can be of the following types:
// 1. list of `StackImport` structs
// 2. list of strings
// 3. List of strings and `StackImport` structs in the same file
func processImportSection(stackMap map[any]any, filePath string) ([]schema.StackImport, error) {
	stackImports, ok := stackMap[cfg.ImportSectionName]

	// If the stack file does not have the `import` section, return
	if !ok || stackImports == nil {
		return nil, nil
	}

	// Check if the `import` section is a list of objects
	importsList, ok := stackImports.([]any)
	if !ok || len(importsList) == 0 {
		return nil, fmt.Errorf("invalid 'import' section in the file '%s'", filePath)
	}

	var result []schema.StackImport

	for _, imp := range importsList {
		if imp == nil {
			return nil, fmt.Errorf("invalid import in the file '%s'", filePath)
		}

		// 1. Try to decode the import as the `StackImport` struct
		var importObj schema.StackImport
		err := mapstructure.Decode(imp, &importObj)
		if err == nil {
			result = append(result, importObj)
			continue
		}

		// 2. Try to cast the import to a string
		s, ok := imp.(string)
		if !ok {
			return nil, fmt.Errorf("invalid import '%v' in the file '%s'", imp, filePath)
		}
		if s == "" {
			return nil, fmt.Errorf("invalid empty import in the file '%s'", filePath)
		}

		result = append(result, schema.StackImport{Path: s})
	}

	return result, nil
}

// sectionContainsAnyNotEmptySections checks if a section contains any of the provided low-level sections, and it's not empty
func sectionContainsAnyNotEmptySections(section map[any]any, sectionsToCheck []string) bool {
	for _, s := range sectionsToCheck {
		if len(s) > 0 {
			if v, ok := section[s]; ok {
				if v2, ok2 := v.(map[any]any); ok2 && len(v2) > 0 {
					return true
				}
				if v2, ok2 := v.(string); ok2 && len(v2) > 0 {
					return true
				}
			}
		}
	}
	return false
}

// CreateComponentStackMap accepts a config file and creates a map of component-stack dependencies
func CreateComponentStackMap(
	cliConfig schema.CliConfiguration,
	stacksBasePath string,
	terraformComponentsBasePath string,
	helmfileComponentsBasePath string,
	filePath string,
) (map[string]map[string][]string, error) {

	stackComponentMap := map[string]map[string][]string{}
	stackComponentMap["terraform"] = map[string][]string{}
	stackComponentMap["helmfile"] = map[string][]string{}

	componentStackMap := map[string]map[string][]string{}
	componentStackMap["terraform"] = map[string][]string{}
	componentStackMap["helmfile"] = map[string][]string{}

	dir := path.Dir(filePath)

	err := filepath.Walk(dir,
		func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			isDirectory, err := u.IsDirectory(p)
			if err != nil {
				return err
			}

			isYaml := u.IsYaml(p)

			if !isDirectory && isYaml {
				config, _, _, err := ProcessYAMLConfigFile(
					cliConfig,
					stacksBasePath,
					p,
					map[string]map[any]any{},
					nil,
					false,
					false,
					false,
					false,
					map[any]any{},
					map[any]any{},
					"",
				)
				if err != nil {
					return err
				}

				finalConfig, err := ProcessStackConfig(
					cliConfig,
					stacksBasePath,
					terraformComponentsBasePath,
					helmfileComponentsBasePath,
					p,
					config,
					false,
					false,
					"",
					nil,
					nil,
					true)
				if err != nil {
					return err
				}

				if componentsConfig, componentsConfigExists := finalConfig["components"]; componentsConfigExists {
					componentsSection := componentsConfig.(map[string]any)
					stackName := strings.Replace(p, stacksBasePath+"/", "", 1)

					if terraformConfig, terraformConfigExists := componentsSection["terraform"]; terraformConfigExists {
						terraformSection := terraformConfig.(map[string]any)

						for k := range terraformSection {
							stackComponentMap["terraform"][stackName] = append(stackComponentMap["terraform"][stackName], k)
						}
					}

					if helmfileConfig, helmfileConfigExists := componentsSection["helmfile"]; helmfileConfigExists {
						helmfileSection := helmfileConfig.(map[string]any)

						for k := range helmfileSection {
							stackComponentMap["helmfile"][stackName] = append(stackComponentMap["helmfile"][stackName], k)
						}
					}
				}
			}

			return nil
		})

	if err != nil {
		return nil, err
	}

	for stack, components := range stackComponentMap["terraform"] {
		for _, component := range components {
			componentStackMap["terraform"][component] = append(componentStackMap["terraform"][component], strings.Replace(stack, cfg.DefaultStackConfigFileExtension, "", 1))
		}
	}

	for stack, components := range stackComponentMap["helmfile"] {
		for _, component := range components {
			componentStackMap["helmfile"][component] = append(componentStackMap["helmfile"][component], strings.Replace(stack, cfg.DefaultStackConfigFileExtension, "", 1))
		}
	}

	return componentStackMap, nil
}

// getFileContent tries to read and return the file content from the sync map if it exists in the map,
// otherwise it reads the file, stores its content in the map and returns the content
func getFileContent(filePath string) (string, error) {
	existingContent, found := getFileContentSyncMap.Load(filePath)
	if found && existingContent != nil {
		return fmt.Sprintf("%s", existingContent), nil
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	getFileContentSyncMap.Store(filePath, content)

	return string(content), nil
}

// ProcessBaseComponentConfig processes base component(s) config
func ProcessBaseComponentConfig(
	cliConfig schema.CliConfiguration,
	baseComponentConfig *schema.BaseComponentConfig,
	allComponentsMap map[any]any,
	component string,
	stack string,
	baseComponent string,
	componentBasePath string,
	checkBaseComponentExists bool,
	baseComponents *[]string,
) error {

	if component == baseComponent {
		return nil
	}

	var baseComponentVars map[any]any
	var baseComponentSettings map[any]any
	var baseComponentEnv map[any]any
	var baseComponentProviders map[any]any
	var baseComponentCommand string
	var baseComponentBackendType string
	var baseComponentBackendSection map[any]any
	var baseComponentRemoteStateBackendType string
	var baseComponentRemoteStateBackendSection map[any]any
	var baseComponentMap map[any]any
	var ok bool

	*baseComponents = append(*baseComponents, baseComponent)

	if baseComponentSection, baseComponentSectionExist := allComponentsMap[baseComponent]; baseComponentSectionExist {
		baseComponentMap, ok = baseComponentSection.(map[any]any)
		if !ok {
			// Depending on the code and libraries, the section can have different map types: map[any]any or map[string]any
			// We try to convert to both
			baseComponentMapOfStrings, ok := baseComponentSection.(map[string]any)
			if !ok {
				return fmt.Errorf("invalid config for the base component '%s' of the component '%s' in the stack '%s'",
					baseComponent, component, stack)
			}
			baseComponentMap = c.MapsOfStringsToMapsOfInterfaces(baseComponentMapOfStrings)
		}

		// First, process the base component(s) of this base component
		if baseComponentOfBaseComponent, baseComponentOfBaseComponentExist := baseComponentMap["component"]; baseComponentOfBaseComponentExist {
			baseComponentOfBaseComponentString, ok := baseComponentOfBaseComponent.(string)
			if !ok {
				return fmt.Errorf("invalid 'component:' section of the component '%s' in the stack '%s'",
					baseComponent, stack)
			}

			err := ProcessBaseComponentConfig(
				cliConfig,
				baseComponentConfig,
				allComponentsMap,
				baseComponent,
				stack,
				baseComponentOfBaseComponentString,
				componentBasePath,
				checkBaseComponentExists,
				baseComponents,
			)

			if err != nil {
				return err
			}
		}

		// Base component metadata.
		// This is per component, not deep-merged and not inherited from base components and globals.
		componentMetadata := map[any]any{}
		if i, ok := baseComponentMap["metadata"]; ok {
			componentMetadata, ok = i.(map[any]any)
			if !ok {
				return fmt.Errorf("invalid '%s.metadata' section in the stack '%s'", component, stack)
			}

			if inheritList, inheritListExist := componentMetadata["inherits"].([]any); inheritListExist {
				for _, v := range inheritList {
					baseComponentFromInheritList, ok := v.(string)
					if !ok {
						return fmt.Errorf("invalid '%s.metadata.inherits' section in the stack '%s'", component, stack)
					}

					if _, ok := allComponentsMap[baseComponentFromInheritList]; !ok {
						if checkBaseComponentExists {
							errorMessage := fmt.Sprintf("The component '%[1]s' in the stack manifest '%[2]s' inherits from '%[3]s' "+
								"(using 'metadata.inherits'), but '%[3]s' is not defined in any of the config files for the stack '%[2]s'",
								component,
								stack,
								baseComponentFromInheritList,
							)
							return errors.New(errorMessage)
						}
					}

					// Process the baseComponentFromInheritList components recursively to find `componentInheritanceChain`
					err := ProcessBaseComponentConfig(
						cliConfig,
						baseComponentConfig,
						allComponentsMap,
						component,
						stack,
						baseComponentFromInheritList,
						componentBasePath,
						checkBaseComponentExists,
						baseComponents,
					)
					if err != nil {
						return err
					}
				}
			}
		}

		if baseComponentVarsSection, baseComponentVarsSectionExist := baseComponentMap["vars"]; baseComponentVarsSectionExist {
			baseComponentVars, ok = baseComponentVarsSection.(map[any]any)
			if !ok {
				return fmt.Errorf("invalid '%s.vars' section in the stack '%s'", baseComponent, stack)
			}
		}

		if baseComponentSettingsSection, baseComponentSettingsSectionExist := baseComponentMap["settings"]; baseComponentSettingsSectionExist {
			baseComponentSettings, ok = baseComponentSettingsSection.(map[any]any)
			if !ok {
				return fmt.Errorf("invalid '%s.settings' section in the stack '%s'", baseComponent, stack)
			}
		}

		if baseComponentEnvSection, baseComponentEnvSectionExist := baseComponentMap["env"]; baseComponentEnvSectionExist {
			baseComponentEnv, ok = baseComponentEnvSection.(map[any]any)
			if !ok {
				return fmt.Errorf("invalid '%s.env' section in the stack '%s'", baseComponent, stack)
			}
		}

		if baseComponentProvidersSection, baseComponentProvidersSectionExist := baseComponentMap[cfg.ProvidersSectionName]; baseComponentProvidersSectionExist {
			baseComponentProviders, ok = baseComponentProvidersSection.(map[any]any)
			if !ok {
				return fmt.Errorf("invalid '%s.providers' section in the stack '%s'", baseComponent, stack)
			}
		}

		// Base component backend
		if i, ok2 := baseComponentMap["backend_type"]; ok2 {
			baseComponentBackendType, ok = i.(string)
			if !ok {
				return fmt.Errorf("invalid '%s.backend_type' section in the stack '%s'", baseComponent, stack)
			}
		}

		if i, ok2 := baseComponentMap["backend"]; ok2 {
			baseComponentBackendSection, ok = i.(map[any]any)
			if !ok {
				return fmt.Errorf("invalid '%s.backend' section in the stack '%s'", baseComponent, stack)
			}
		}

		// Base component remote state backend
		if i, ok2 := baseComponentMap["remote_state_backend_type"]; ok2 {
			baseComponentRemoteStateBackendType, ok = i.(string)
			if !ok {
				return fmt.Errorf("invalid '%s.remote_state_backend_type' section in the stack '%s'", baseComponent, stack)
			}
		}

		if i, ok2 := baseComponentMap["remote_state_backend"]; ok2 {
			baseComponentRemoteStateBackendSection, ok = i.(map[any]any)
			if !ok {
				return fmt.Errorf("invalid '%s.remote_state_backend' section in the stack '%s'", baseComponent, stack)
			}
		}

		// Base component `command`
		if baseComponentCommandSection, baseComponentCommandSectionExist := baseComponentMap[cfg.CommandSectionName]; baseComponentCommandSectionExist {
			baseComponentCommand, ok = baseComponentCommandSection.(string)
			if !ok {
				return fmt.Errorf("invalid '%s.command' section in the stack '%s'", baseComponent, stack)
			}
		}

		if len(baseComponentConfig.FinalBaseComponentName) == 0 {
			baseComponentConfig.FinalBaseComponentName = baseComponent
		}

		// Base component `vars`
		merged, err := m.Merge(cliConfig, []map[any]any{baseComponentConfig.BaseComponentVars, baseComponentVars})
		if err != nil {
			return err
		}
		baseComponentConfig.BaseComponentVars = merged

		// Base component `settings`
		merged, err = m.Merge(cliConfig, []map[any]any{baseComponentConfig.BaseComponentSettings, baseComponentSettings})
		if err != nil {
			return err
		}
		baseComponentConfig.BaseComponentSettings = merged

		// Base component `env`
		merged, err = m.Merge(cliConfig, []map[any]any{baseComponentConfig.BaseComponentEnv, baseComponentEnv})
		if err != nil {
			return err
		}
		baseComponentConfig.BaseComponentEnv = merged

		// Base component `providers`
		merged, err = m.Merge(cliConfig, []map[any]any{baseComponentConfig.BaseComponentProviders, baseComponentProviders})
		if err != nil {
			return err
		}
		baseComponentConfig.BaseComponentProviders = merged

		// Base component `command`
		baseComponentConfig.BaseComponentCommand = baseComponentCommand

		// Base component `backend_type`
		baseComponentConfig.BaseComponentBackendType = baseComponentBackendType

		// Base component `backend`
		merged, err = m.Merge(cliConfig, []map[any]any{baseComponentConfig.BaseComponentBackendSection, baseComponentBackendSection})
		if err != nil {
			return err
		}
		baseComponentConfig.BaseComponentBackendSection = merged

		// Base component `remote_state_backend_type`
		baseComponentConfig.BaseComponentRemoteStateBackendType = baseComponentRemoteStateBackendType

		// Base component `remote_state_backend`
		merged, err = m.Merge(cliConfig, []map[any]any{baseComponentConfig.BaseComponentRemoteStateBackendSection, baseComponentRemoteStateBackendSection})
		if err != nil {
			return err
		}
		baseComponentConfig.BaseComponentRemoteStateBackendSection = merged

		baseComponentConfig.ComponentInheritanceChain = u.UniqueStrings(append([]string{baseComponent}, baseComponentConfig.ComponentInheritanceChain...))
	} else {
		if checkBaseComponentExists {
			// Check if the base component exists as Terraform/Helmfile component
			// If it does exist, don't throw errors if it is not defined in YAML config
			componentPath := path.Join(componentBasePath, baseComponent)
			componentPathExists, err := u.IsDirectory(componentPath)
			if err != nil || !componentPathExists {
				return errors.New("The component '" + component + "' inherits from the base component '" +
					baseComponent + "' (using 'component:' attribute), " + "but `" + baseComponent + "' is not defined in any of the YAML config files for the stack '" + stack + "'")
			}
		}
	}

	return nil
}

// FindComponentsDerivedFromBaseComponents finds all components that derive from the given base components
func FindComponentsDerivedFromBaseComponents(
	stack string,
	allComponents map[string]any,
	baseComponents []string,
) ([]string, error) {

	res := []string{}

	for component, compSection := range allComponents {
		componentSection, ok := compSection.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid '%s' component section in the file '%s'", component, stack)
		}

		if base, baseComponentExist := componentSection[cfg.ComponentSectionName]; baseComponentExist {
			baseComponent, ok := base.(string)
			if !ok {
				return nil, fmt.Errorf("invalid 'component' attribute in the component '%s' in the file '%s'", component, stack)
			}

			if baseComponent != "" && u.SliceContainsString(baseComponents, baseComponent) {
				res = append(res, component)
			}
		}
	}

	return res, nil
}
