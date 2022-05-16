package stack

import (
	"errors"
	"fmt"
	"github.com/bmatcuk/doublestar/v4"
	c "github.com/cloudposse/atmos/pkg/convert"
	g "github.com/cloudposse/atmos/pkg/globals"
	m "github.com/cloudposse/atmos/pkg/merge"
	u "github.com/cloudposse/atmos/pkg/utils"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

var (
	getFileContentSyncMap = sync.Map{}
	getGlobMatchesSyncMap = sync.Map{}
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

// FindComponentDependencies finds all imports where the component or the base component is defined
// Component depends on the imported config file if any of the following conditions is true:
// 1. The imported file has any of the global `backend`, `backend_type`, `env`, `remote_state_backend`, `remote_state_backend_type`,
//    `settings` or `vars` sections which are not empty
// 2. The imported file has the component type section, which has any of the `backend`, `backend_type`, `env`, `remote_state_backend`,
//    `remote_state_backend_type`, `settings` or `vars` sections which are not empty
// 3. The imported config file has the "components" section, which has the component type section, which has the component section
// 4. The imported config file has the "components" section, which has the component type section, which has the base component section
func FindComponentDependencies(
	stack string,
	componentType string,
	component string,
	baseComponent string,
	importsConfig map[string]map[interface{}]interface{}) ([]string, error) {

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
			if sectionContainsAnyNotEmptySections(importConfig[componentType].(map[interface{}]interface{}), []string{
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
			componentsSection := i.(map[interface{}]interface{})

			if i2, ok2 := componentsSection[componentType]; ok2 {
				componentTypeSection := i2.(map[interface{}]interface{})

				if i3, ok3 := componentTypeSection[component]; ok3 {
					componentSection := i3.(map[interface{}]interface{})

					if len(componentSection) > 0 {
						deps = append(deps, imp)
						continue
					}
				}

				if baseComponent != "" {
					if i3, ok3 := componentTypeSection[baseComponent]; ok3 {
						baseComponentSection := i3.(map[interface{}]interface{})

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
func sectionContainsAnyNotEmptySections(section map[interface{}]interface{}, sectionsToCheck []string) bool {
	for _, s := range sectionsToCheck {
		if len(s) > 0 {
			if v, ok := section[s]; ok {
				if v2, ok2 := v.(map[interface{}]interface{}); ok2 && len(v2) > 0 {
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
				config, _, err := ProcessYAMLConfigFile(stacksBasePath, p, map[string]map[interface{}]interface{}{})
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
					componentsSection := componentsConfig.(map[string]interface{})
					stackName := strings.Replace(p, stacksBasePath+"/", "", 1)

					if terraformConfig, terraformConfigExists := componentsSection["terraform"]; terraformConfigExists {
						terraformSection := terraformConfig.(map[string]interface{})

						for k := range terraformSection {
							stackComponentMap["terraform"][stackName] = append(stackComponentMap["terraform"][stackName], k)
						}
					}

					if helmfileConfig, helmfileConfigExists := componentsSection["helmfile"]; helmfileConfigExists {
						helmfileSection := helmfileConfig.(map[string]interface{})

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
			componentStackMap["terraform"][component] = append(componentStackMap["terraform"][component], strings.Replace(stack, g.DefaultStackConfigFileExtension, "", 1))
		}
	}

	for stack, components := range stackComponentMap["helmfile"] {
		for _, component := range components {
			componentStackMap["helmfile"][component] = append(componentStackMap["helmfile"][component], strings.Replace(stack, g.DefaultStackConfigFileExtension, "", 1))
		}
	}

	return componentStackMap, nil
}

// getFileContent tries to read and return the file content from the sync map if it exists in the map,
// otherwise it reads the file, stores its content in the map and returns the content
func getFileContent(filePath string) (string, error) {
	existingContent, found := getFileContentSyncMap.Load(filePath)
	if found == true && existingContent != nil {
		return fmt.Sprintf("%s", existingContent), nil
	}

	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	getFileContentSyncMap.Store(filePath, content)

	return string(content), nil
}

// GetGlobMatches tries to read and return the Glob matches content from the sync map if it exists in the map,
// otherwise it finds and returns all files matching the pattern, stores the files in the map and returns the files
func GetGlobMatches(pattern string) ([]string, error) {
	existingMatches, found := getGlobMatchesSyncMap.Load(pattern)
	if found == true && existingMatches != nil {
		return strings.Split(fmt.Sprintf("%s", existingMatches), ","), nil
	}

	base, cleanPattern := doublestar.SplitPattern(pattern)
	f := os.DirFS(base)

	matches, err := doublestar.Glob(f, cleanPattern)
	if err != nil {
		return nil, err
	}

	if matches == nil {
		return nil, errors.New(fmt.Sprintf("Failed to find a match for the import '%s' ('%s' + '%s')", pattern, base, cleanPattern))
	}

	var fullMatches []string
	for _, match := range matches {
		fullMatches = append(fullMatches, path.Join(base, match))
	}

	getGlobMatchesSyncMap.Store(pattern, strings.Join(fullMatches, ","))

	return fullMatches, nil
}

type BaseComponentConfig struct {
	BaseComponentVars                      map[interface{}]interface{}
	BaseComponentSettings                  map[interface{}]interface{}
	BaseComponentEnv                       map[interface{}]interface{}
	FinalBaseComponentName                 string
	BaseComponentCommand                   string
	BaseComponentBackendType               string
	BaseComponentBackendSection            map[interface{}]interface{}
	BaseComponentRemoteStateBackendType    string
	BaseComponentRemoteStateBackendSection map[interface{}]interface{}
	ComponentInheritanceChain              []string
}

// ProcessBaseComponentConfig processes base component(s) config
func ProcessBaseComponentConfig(
	baseComponentConfig *BaseComponentConfig,
	allComponentsMap map[interface{}]interface{},
	component string,
	stack string,
	baseComponent string,
	componentBasePath string,
	checkBaseComponentExists bool) error {

	if component == baseComponent {
		return nil
	}

	var baseComponentVars map[interface{}]interface{}
	var baseComponentSettings map[interface{}]interface{}
	var baseComponentEnv map[interface{}]interface{}
	var baseComponentCommand string
	var baseComponentBackendType string
	var baseComponentBackendSection map[interface{}]interface{}
	var baseComponentRemoteStateBackendType string
	var baseComponentRemoteStateBackendSection map[interface{}]interface{}
	var baseComponentMap map[interface{}]interface{}
	var ok bool

	if baseComponentSection, baseComponentSectionExist := allComponentsMap[baseComponent]; baseComponentSectionExist {
		baseComponentMap, ok = baseComponentSection.(map[interface{}]interface{})
		if !ok {
			// Depending on the code and libraries, the section can have diferent map types: map[interface{}]interface{} or map[string]interface{}
			// We try to convert to both
			baseComponentMap2, ok := baseComponentSection.(map[string]interface{})
			if !ok {
				return errors.New(fmt.Sprintf("Invalid config for the base component '%s' of the component '%s' in the stack '%s'",
					baseComponent, component, stack))
			}
			baseComponentMap = c.MapsOfStringsToMapsOfInterfaces(baseComponentMap2)
		}

		// First, process the base component of this base component
		if baseComponentOfBaseComponent, baseComponentOfBaseComponentExist := baseComponentMap["component"]; baseComponentOfBaseComponentExist {
			baseComponentOfBaseComponentString, ok := baseComponentOfBaseComponent.(string)
			if !ok {
				return errors.New(fmt.Sprintf("Invalid 'component:' section of the component '%s' in the stack '%s'",
					baseComponent, stack))
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
			baseComponentVars, ok = baseComponentVarsSection.(map[interface{}]interface{})
			if !ok {
				return errors.New(fmt.Sprintf("Invalid '%s.vars' section in the stack '%s'", baseComponent, stack))
			}
		}

		if baseComponentSettingsSection, baseComponentSettingsSectionExist := baseComponentMap["settings"]; baseComponentSettingsSectionExist {
			baseComponentSettings, ok = baseComponentSettingsSection.(map[interface{}]interface{})
			if !ok {
				return errors.New(fmt.Sprintf("Invalid '%s.settings' section in the stack '%s'", baseComponent, stack))
			}
		}

		if baseComponentEnvSection, baseComponentEnvSectionExist := baseComponentMap["env"]; baseComponentEnvSectionExist {
			baseComponentEnv, ok = baseComponentEnvSection.(map[interface{}]interface{})
			if !ok {
				return errors.New(fmt.Sprintf("Invalid '%s.env' section in the stack '%s'", baseComponent, stack))
			}
		}

		// Base component backend
		if i, ok2 := baseComponentMap["backend_type"]; ok2 {
			baseComponentBackendType, ok = i.(string)
			if !ok {
				return errors.New(fmt.Sprintf("Invalid '%s.backend_type' section in the stack '%s'", baseComponent, stack))
			}
		}

		if i, ok2 := baseComponentMap["backend"]; ok2 {
			baseComponentBackendSection, ok = i.(map[interface{}]interface{})
			if !ok {
				return errors.New(fmt.Sprintf("Invalid '%s.backend' section in the stack '%s'", baseComponent, stack))
			}
		}

		// Base component remote state backend
		if i, ok2 := baseComponentMap["remote_state_backend_type"]; ok2 {
			baseComponentRemoteStateBackendType, ok = i.(string)
			if !ok {
				return errors.New(fmt.Sprintf("Invalid '%s.remote_state_backend_type' section in the stack '%s'", baseComponent, stack))
			}
		}

		if i, ok2 := baseComponentMap["remote_state_backend"]; ok2 {
			baseComponentRemoteStateBackendSection, ok = i.(map[interface{}]interface{})
			if !ok {
				return errors.New(fmt.Sprintf("Invalid '%s.remote_state_backend' section in the stack '%s'", baseComponent, stack))
			}
		}

		// Base component `command`
		if baseComponentCommandSection, baseComponentCommandSectionExist := baseComponentMap["command"]; baseComponentCommandSectionExist {
			baseComponentCommand, ok = baseComponentCommandSection.(string)
			if !ok {
				return errors.New(fmt.Sprintf("Invalid '%s.command' section in the stack '%s'", baseComponent, stack))
			}
		}

		if len(baseComponentConfig.FinalBaseComponentName) == 0 {
			baseComponentConfig.FinalBaseComponentName = baseComponent
		}

		// Base component `vars`
		merged, err := m.Merge([]map[interface{}]interface{}{baseComponentConfig.BaseComponentVars, baseComponentVars})
		if err != nil {
			return err
		}
		baseComponentConfig.BaseComponentVars = merged

		// Base component `settings`
		merged, err = m.Merge([]map[interface{}]interface{}{baseComponentConfig.BaseComponentSettings, baseComponentSettings})
		if err != nil {
			return err
		}
		baseComponentConfig.BaseComponentSettings = merged

		// Base component `env`
		merged, err = m.Merge([]map[interface{}]interface{}{baseComponentConfig.BaseComponentEnv, baseComponentEnv})
		if err != nil {
			return err
		}
		baseComponentConfig.BaseComponentEnv = merged

		// Base component `command`
		baseComponentConfig.BaseComponentCommand = baseComponentCommand

		// Base component `backend_type`
		baseComponentConfig.BaseComponentBackendType = baseComponentBackendType

		// Base component `backend`
		merged, err = m.Merge([]map[interface{}]interface{}{baseComponentConfig.BaseComponentBackendSection, baseComponentBackendSection})
		if err != nil {
			return err
		}
		baseComponentConfig.BaseComponentBackendSection = merged

		// Base component `remote_state_backend_type`
		baseComponentConfig.BaseComponentRemoteStateBackendType = baseComponentRemoteStateBackendType

		// Base component `remote_state_backend`
		merged, err = m.Merge([]map[interface{}]interface{}{baseComponentConfig.BaseComponentRemoteStateBackendSection, baseComponentRemoteStateBackendSection})
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
	allComponents map[string]interface{},
	baseComponents []string,
) ([]string, error) {

	res := []string{}

	for component, compSection := range allComponents {
		componentSection, ok := compSection.(map[string]interface{})
		if !ok {
			return nil, errors.New(fmt.Sprintf("Invalid '%s' component section in the file '%s'", component, stack))
		}

		if base, baseComponentExist := componentSection["component"]; baseComponentExist {
			baseComponent, ok := base.(string)
			if !ok {
				return nil, errors.New(fmt.Sprintf("Invalid 'component' attribute in the component '%s' in the file '%s'", component, stack))
			}

			if baseComponent != "" && u.SliceContainsString(baseComponents, baseComponent) {
				res = append(res, component)
			}
		}
	}

	return res, nil
}
