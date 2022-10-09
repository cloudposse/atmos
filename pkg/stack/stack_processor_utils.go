package stack

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	cfg "github.com/cloudposse/atmos/pkg/config"
	c "github.com/cloudposse/atmos/pkg/convert"
	m "github.com/cloudposse/atmos/pkg/merge"
	u "github.com/cloudposse/atmos/pkg/utils"
)

var (
	getFileContentSyncMap = sync.Map{}
)

// FindComponentStacks finds all infrastructure stack config files where the component or the base component is defined
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

// FindComponentDependencies finds all imports where the component or the base component(s) are defined
// Component depends on the imported config file if any of the following conditions is true:
//  1. The imported file has any of the global `backend`, `backend_type`, `env`, `remote_state_backend`, `remote_state_backend_type`,
//     `settings` or `vars` sections which are not empty
//  2. The imported file has the component type section, which has any of the `backend`, `backend_type`, `env`, `remote_state_backend`,
//     `remote_state_backend_type`, `settings` or `vars` sections which are not empty
//  3. The imported config file has the "components" section, which has the component type section, which has the component section
//  4. The imported config file has the "components" section, which has the component type section, which has the base component(s) section
func FindComponentDependencies(
	stack string,
	componentType string,
	component string,
	baseComponents []string,
	importsConfig map[string]map[any]any) ([]string, error) {

	var deps []string

	for imp, importConfig := range importsConfig {
		if sectionContainsAnyNotEmptySections(importConfig, []string{
			"backend",
			"backend_type",
			"env",
			"remote_state_backend",
			"remote_state_backend_type",
			"settings",
			"vars",
		}) {
			deps = append(deps, imp)
			continue
		}

		if sectionContainsAnyNotEmptySections(importConfig, []string{componentType}) {
			if sectionContainsAnyNotEmptySections(importConfig[componentType].(map[any]any), []string{
				"backend",
				"backend_type",
				"env",
				"remote_state_backend",
				"remote_state_backend_type",
				"settings",
				"vars",
			}) {
				deps = append(deps, imp)
				continue
			}
		}

		if i, ok := importConfig["components"]; ok {
			componentsSection := i.(map[any]any)

			if i2, ok2 := componentsSection[componentType]; ok2 {
				componentTypeSection := i2.(map[any]any)

				if i3, ok3 := componentTypeSection[component]; ok3 {
					componentSection := i3.(map[any]any)

					if len(componentSection) > 0 {
						deps = append(deps, imp)
						continue
					}
				}

				for _, baseComponent := range baseComponents {
					if i3, ok3 := componentTypeSection[baseComponent]; ok3 {
						baseComponentSection := i3.(map[any]any)

						if len(baseComponentSection) > 0 {
							deps = append(deps, imp)
							continue
						}
					}
				}
			}
		}
	}

	deps = append(deps, stack)
	unique := u.UniqueStrings(deps)
	sort.Strings(unique)
	return unique, nil
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
				config, _, err := ProcessYAMLConfigFile(stacksBasePath, p, map[string]map[any]any{})
				if err != nil {
					return err
				}

				finalConfig, err := ProcessStackConfig(
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

type BaseComponentConfig struct {
	BaseComponentVars                      map[any]any
	BaseComponentSettings                  map[any]any
	BaseComponentEnv                       map[any]any
	FinalBaseComponentName                 string
	BaseComponentCommand                   string
	BaseComponentBackendType               string
	BaseComponentBackendSection            map[any]any
	BaseComponentRemoteStateBackendType    string
	BaseComponentRemoteStateBackendSection map[any]any
	ComponentInheritanceChain              []string
}

// ProcessBaseComponentConfig processes base component(s) config
func ProcessBaseComponentConfig(
	baseComponentConfig *BaseComponentConfig,
	allComponentsMap map[any]any,
	component string,
	stack string,
	baseComponent string,
	componentBasePath string,
	checkBaseComponentExists bool) error {

	if component == baseComponent {
		return nil
	}

	var baseComponentVars map[any]any
	var baseComponentSettings map[any]any
	var baseComponentEnv map[any]any
	var baseComponentCommand string
	var baseComponentBackendType string
	var baseComponentBackendSection map[any]any
	var baseComponentRemoteStateBackendType string
	var baseComponentRemoteStateBackendSection map[any]any
	var baseComponentMap map[any]any
	var ok bool

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

		// First, process the base component of this base component
		if baseComponentOfBaseComponent, baseComponentOfBaseComponentExist := baseComponentMap["component"]; baseComponentOfBaseComponentExist {
			baseComponentOfBaseComponentString, ok := baseComponentOfBaseComponent.(string)
			if !ok {
				return fmt.Errorf("invalid 'component:' section of the component '%s' in the stack '%s'",
					baseComponent, stack)
			}

			err := ProcessBaseComponentConfig(
				baseComponentConfig,
				allComponentsMap,
				baseComponent,
				stack,
				baseComponentOfBaseComponentString,
				componentBasePath,
				checkBaseComponentExists,
			)

			if err != nil {
				return err
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
		if baseComponentCommandSection, baseComponentCommandSectionExist := baseComponentMap["command"]; baseComponentCommandSectionExist {
			baseComponentCommand, ok = baseComponentCommandSection.(string)
			if !ok {
				return fmt.Errorf("invalid '%s.command' section in the stack '%s'", baseComponent, stack)
			}
		}

		if len(baseComponentConfig.FinalBaseComponentName) == 0 {
			baseComponentConfig.FinalBaseComponentName = baseComponent
		}

		// Base component `vars`
		merged, err := m.Merge([]map[any]any{baseComponentConfig.BaseComponentVars, baseComponentVars})
		if err != nil {
			return err
		}
		baseComponentConfig.BaseComponentVars = merged

		// Base component `settings`
		merged, err = m.Merge([]map[any]any{baseComponentConfig.BaseComponentSettings, baseComponentSettings})
		if err != nil {
			return err
		}
		baseComponentConfig.BaseComponentSettings = merged

		// Base component `env`
		merged, err = m.Merge([]map[any]any{baseComponentConfig.BaseComponentEnv, baseComponentEnv})
		if err != nil {
			return err
		}
		baseComponentConfig.BaseComponentEnv = merged

		// Base component `command`
		baseComponentConfig.BaseComponentCommand = baseComponentCommand

		// Base component `backend_type`
		baseComponentConfig.BaseComponentBackendType = baseComponentBackendType

		// Base component `backend`
		merged, err = m.Merge([]map[any]any{baseComponentConfig.BaseComponentBackendSection, baseComponentBackendSection})
		if err != nil {
			return err
		}
		baseComponentConfig.BaseComponentBackendSection = merged

		// Base component `remote_state_backend_type`
		baseComponentConfig.BaseComponentRemoteStateBackendType = baseComponentRemoteStateBackendType

		// Base component `remote_state_backend`
		merged, err = m.Merge([]map[any]any{baseComponentConfig.BaseComponentRemoteStateBackendSection, baseComponentRemoteStateBackendSection})
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

		if base, baseComponentExist := componentSection["component"]; baseComponentExist {
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
