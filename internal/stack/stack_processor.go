package stack

import (
	c "atmos/internal/convert"
	g "atmos/internal/globals"
	m "atmos/internal/merge"
	u "atmos/internal/utils"
	"fmt"
	"github.com/bmatcuk/doublestar"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

var (
	// Mutex to serialize updates of the result map of ProcessYAMLConfigFiles function
	processYAMLConfigFilesLock = &sync.Mutex{}

	// Mutex to serialize updates of the result map of ProcessYAMLConfigFile function
	processYAMLConfigFileLock = &sync.Mutex{}
)

// ProcessYAMLConfigFiles takes a list of paths to YAML config files, processes and deep-merges all imports,
// and returns a list of stack configs
func ProcessYAMLConfigFiles(
	basePath string,
	filePaths []string,
	processStackDeps bool,
	processComponentDeps bool) ([]string, map[string]interface{}, error) {

	count := len(filePaths)
	listResult := make([]string, count)
	mapResult := map[string]interface{}{}
	var errorResult error
	var wg sync.WaitGroup
	wg.Add(count)

	for i, filePath := range filePaths {
		go func(i int, p string) {
			defer wg.Done()

			config, importsConfig, err := ProcessYAMLConfigFile(basePath, p, map[string]map[interface{}]interface{}{})
			if err != nil {
				errorResult = err
				return
			}

			var imports []string
			for k := range importsConfig {
				imports = append(imports, k)
			}

			uniqueImports := u.UniqueStrings(imports)
			sort.Strings(uniqueImports)

			componentStackMap := map[string]map[string][]string{}
			if processStackDeps {
				componentStackMap, err = CreateComponentStackMap(basePath, p)
				if err != nil {
					errorResult = err
					return
				}
			}

			finalConfig, err := ProcessConfig(basePath, p, config, processStackDeps, processComponentDeps, "", componentStackMap, importsConfig)
			if err != nil {
				errorResult = err
				return
			}

			finalConfig["imports"] = uniqueImports

			yamlConfig, err := yaml.Marshal(finalConfig)
			if err != nil {
				errorResult = err
				return
			}

			stackName := strings.TrimSuffix(strings.TrimSuffix(strings.TrimPrefix(p, basePath+"/"), g.DefaultStackConfigFileExtension), ".yml")

			processYAMLConfigFilesLock.Lock()
			defer processYAMLConfigFilesLock.Unlock()

			listResult[i] = string(yamlConfig)
			mapResult[stackName] = finalConfig
		}(i, filePath)
	}

	wg.Wait()

	if errorResult != nil {
		return nil, nil, errorResult
	}
	return listResult, mapResult, nil
}

// ProcessYAMLConfigFile takes a path to a YAML config file,
// recursively processes and deep-merges all imports,
// and returns stack config as map[interface{}]interface{}
func ProcessYAMLConfigFile(
	basePath string,
	filePath string,
	importsConfig map[string]map[interface{}]interface{}) (map[interface{}]interface{}, map[string]map[interface{}]interface{}, error) {

	var configs []map[interface{}]interface{}

	stackYamlConfig, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, nil, err
	}

	stackMapConfig, err := c.YAMLToMapOfInterfaces(string(stackYamlConfig))
	if err != nil {
		return nil, nil, err
	}

	// Find and process all imports
	if importsSection, ok := stackMapConfig["import"]; ok {
		imports := importsSection.([]interface{})

		count := len(imports)
		var errorResult error
		var wg sync.WaitGroup
		wg.Add(count)

		for _, im := range imports {
			imp := im.(string)

			// If the import file is specified without extension, use `.yaml` as default
			impWithExt := imp
			ext := filepath.Ext(imp)
			if ext == "" {
				ext = g.DefaultStackConfigFileExtension
				impWithExt = imp + ext
			}

			impWithExtPath := path.Join(basePath, impWithExt)

			if impWithExtPath == filePath {
				errorMessage := fmt.Sprintf("Invalid import in config file %s. The file imports itself in import: '%s'",
					filePath,
					strings.Replace(impWithExt, basePath+"/", "", 1))
				return nil, nil, errors.New(errorMessage)
			}

			// Find all matches in the glob
			matches, err := doublestar.Glob(impWithExtPath)
			if err != nil {
				return nil, nil, err
			}

			if matches == nil {
				errorMessage := fmt.Sprintf("Invalid import in config file %s. No matches found for import: '%s'",
					filePath,
					strings.Replace(impWithExt, basePath+"/", "", 1))
				return nil, nil, errors.New(errorMessage)
			}

			// If we import a glob with more than 1 file, add the difference to the WaitGroup
			if len(matches) > 1 {
				wg.Add(len(matches) - 1)
			}

			for _, importFile := range matches {
				go func(p string) {
					defer wg.Done()

					yamlConfig, _, err := ProcessYAMLConfigFile(basePath, p, importsConfig)
					if err != nil {
						errorResult = err
						return
					}

					processYAMLConfigFileLock.Lock()
					defer processYAMLConfigFileLock.Unlock()
					configs = append(configs, yamlConfig)
					importRelativePathWithExt := strings.Replace(p, basePath+"/", "", 1)
					importRelativePathWithoutExt := strings.Replace(importRelativePathWithExt, ext, "", 1)
					importsConfig[importRelativePathWithoutExt] = yamlConfig
				}(importFile)
			}
		}

		wg.Wait()

		if errorResult != nil {
			return nil, nil, errorResult
		}
	}

	configs = append(configs, stackMapConfig)

	// Deep-merge the config file and the imports
	result, err := m.Merge(configs)
	if err != nil {
		return nil, nil, err
	}

	return result, importsConfig, nil
}

// ProcessConfig takes a raw stack config, deep-merges all variables, settings, environments and backends,
// and returns the final stack configuration for all Terraform and helmfile components
func ProcessConfig(
	basePath string,
	stack string,
	config map[interface{}]interface{},
	processStackDeps bool,
	processComponentDeps bool,
	componentTypeFilter string,
	componentStackMap map[string]map[string][]string,
	importsConfig map[string]map[interface{}]interface{}) (map[interface{}]interface{}, error) {

	stackName := strings.TrimSuffix(strings.TrimSuffix(strings.TrimPrefix(stack, basePath+"/"), g.DefaultStackConfigFileExtension), ".yml")

	globalVarsSection := map[interface{}]interface{}{}
	globalSettingsSection := map[interface{}]interface{}{}
	globalEnvSection := map[interface{}]interface{}{}
	globalTerraformSection := map[interface{}]interface{}{}
	globalHelmfileSection := map[interface{}]interface{}{}
	globalComponentsSection := map[interface{}]interface{}{}

	terraformVars := map[interface{}]interface{}{}
	terraformSettings := map[interface{}]interface{}{}
	terraformEnv := map[interface{}]interface{}{}

	helmfileVars := map[interface{}]interface{}{}
	helmfileSettings := map[interface{}]interface{}{}
	helmfileEnv := map[interface{}]interface{}{}

	backendType := "s3"
	backend := map[interface{}]interface{}{}

	terraformComponents := map[string]interface{}{}
	helmfileComponents := map[string]interface{}{}
	allComponents := map[string]interface{}{}

	// Global sections
	if i, ok := config["vars"]; ok {
		globalVarsSection = i.(map[interface{}]interface{})
	}

	if i, ok := config["settings"]; ok {
		globalSettingsSection = i.(map[interface{}]interface{})
	}

	if i, ok := config["env"]; ok {
		globalEnvSection = i.(map[interface{}]interface{})
	}

	if i, ok := config["terraform"]; ok {
		globalTerraformSection = i.(map[interface{}]interface{})
	}

	if i, ok := config["helmfile"]; ok {
		globalHelmfileSection = i.(map[interface{}]interface{})
	}

	if i, ok := config["components"]; ok {
		globalComponentsSection = i.(map[interface{}]interface{})
	}

	// Terraform section
	if i, ok := globalTerraformSection["vars"]; ok {
		terraformVars = i.(map[interface{}]interface{})
	}

	globalAndTerraformVars, err := m.Merge([]map[interface{}]interface{}{globalVarsSection, terraformVars})
	if err != nil {
		return nil, err
	}

	if i, ok := globalTerraformSection["settings"]; ok {
		terraformSettings = i.(map[interface{}]interface{})
	}

	globalAndTerraformSettings, err := m.Merge([]map[interface{}]interface{}{globalSettingsSection, terraformSettings})
	if err != nil {
		return nil, err
	}

	if i, ok := globalTerraformSection["env"]; ok {
		terraformEnv = i.(map[interface{}]interface{})
	}

	globalAndTerraformEnv, err := m.Merge([]map[interface{}]interface{}{globalEnvSection, terraformEnv})
	if err != nil {
		return nil, err
	}

	if i, ok := globalTerraformSection["backend_type"]; ok {
		backendType = i.(string)
	}

	if i, ok := globalTerraformSection["backend"]; ok {
		if backendSection, backendSectionExist := i.(map[interface{}]interface{})[backendType]; backendSectionExist {
			backend = backendSection.(map[interface{}]interface{})
		}
	}

	// Helmfile section
	if i, ok := globalHelmfileSection["vars"]; ok {
		helmfileVars = i.(map[interface{}]interface{})
	}

	globalAndHelmfileVars, err := m.Merge([]map[interface{}]interface{}{globalVarsSection, helmfileVars})
	if err != nil {
		return nil, err
	}

	if i, ok := globalHelmfileSection["settings"]; ok {
		helmfileSettings = i.(map[interface{}]interface{})
	}

	globalAndHelmfileSettings, err := m.Merge([]map[interface{}]interface{}{globalSettingsSection, helmfileSettings})
	if err != nil {
		return nil, err
	}

	if i, ok := globalHelmfileSection["env"]; ok {
		helmfileEnv = i.(map[interface{}]interface{})
	}

	globalAndHelmfileEnv, err := m.Merge([]map[interface{}]interface{}{globalEnvSection, helmfileEnv})
	if err != nil {
		return nil, err
	}

	// Process all Terraform components
	if componentTypeFilter == "" || componentTypeFilter == "terraform" {
		if allTerraformComponents, ok := globalComponentsSection["terraform"]; ok {
			allTerraformComponentsMap := allTerraformComponents.(map[interface{}]interface{})

			for component, v := range allTerraformComponentsMap {
				componentMap := v.(map[interface{}]interface{})

				componentVars := map[interface{}]interface{}{}
				if i, ok2 := componentMap["vars"]; ok2 {
					componentVars = i.(map[interface{}]interface{})
				}

				componentSettings := map[interface{}]interface{}{}
				if i, ok2 := componentMap["settings"]; ok2 {
					componentSettings = i.(map[interface{}]interface{})
				}

				componentEnv := map[interface{}]interface{}{}
				if i, ok2 := componentMap["env"]; ok2 {
					componentEnv = i.(map[interface{}]interface{})
				}

				componentBackend := map[interface{}]interface{}{}
				if i, ok2 := componentMap["backend"]; ok2 {
					componentBackend = i.(map[interface{}]interface{})[backendType].(map[interface{}]interface{})
				}

				baseComponentVars := map[interface{}]interface{}{}
				baseComponentSettings := map[interface{}]interface{}{}
				baseComponentEnv := map[interface{}]interface{}{}
				baseComponentBackend := map[interface{}]interface{}{}
				baseComponentName := ""

				if baseComponent, baseComponentExist := componentMap["component"]; baseComponentExist {
					baseComponentName = baseComponent.(string)

					if baseComponentSection, baseComponentSectionExist := allTerraformComponentsMap[baseComponentName]; baseComponentSectionExist {
						baseComponentMap := baseComponentSection.(map[interface{}]interface{})

						if baseComponentVarsSection, baseComponentVarsSectionExist := baseComponentMap["vars"]; baseComponentVarsSectionExist {
							baseComponentVars = baseComponentVarsSection.(map[interface{}]interface{})
						}

						if baseComponentSettingsSection, baseComponentSettingsSectionExist := baseComponentMap["settings"]; baseComponentSettingsSectionExist {
							baseComponentSettings = baseComponentSettingsSection.(map[interface{}]interface{})
						}

						if baseComponentEnvSection, baseComponentEnvSectionExist := baseComponentMap["env"]; baseComponentEnvSectionExist {
							baseComponentEnv = baseComponentEnvSection.(map[interface{}]interface{})
						}

						if baseComponentBackendSection, baseComponentBackendSectionExist := baseComponentMap["backend"]; baseComponentBackendSectionExist {
							if backendTypeSection, backendTypeSectionExist := baseComponentBackendSection.(map[interface{}]interface{})[backendType]; backendTypeSectionExist {
								baseComponentBackend = backendTypeSection.(map[interface{}]interface{})
							}
						}
					} else {
						return nil, errors.New("Terraform component '" + component.(string) + "' defines attribute 'component: " +
							baseComponentName + "', " + "but `" + baseComponentName + "' is not defined in the stack '" + stack + "'")
					}
				}

				finalComponentVars, err := m.Merge([]map[interface{}]interface{}{globalAndTerraformVars, baseComponentVars, componentVars})
				if err != nil {
					return nil, err
				}

				finalComponentSettings, err := m.Merge([]map[interface{}]interface{}{globalAndTerraformSettings, baseComponentSettings, componentSettings})
				if err != nil {
					return nil, err
				}

				finalComponentEnv, err := m.Merge([]map[interface{}]interface{}{globalAndTerraformEnv, baseComponentEnv, componentEnv})
				if err != nil {
					return nil, err
				}

				finalComponentBackend, err := m.Merge([]map[interface{}]interface{}{backend, baseComponentBackend, componentBackend})
				if err != nil {
					return nil, err
				}

				comp := map[string]interface{}{}
				comp["vars"] = finalComponentVars
				comp["settings"] = finalComponentSettings
				comp["env"] = finalComponentEnv
				comp["backend_type"] = backendType
				comp["backend"] = finalComponentBackend

				if baseComponentName != "" {
					comp["component"] = baseComponentName
				}

				if processStackDeps == true {
					componentStacks, err := FindComponentStacks("terraform", component.(string), baseComponentName, componentStackMap)
					if err != nil {
						return nil, err
					}
					comp["stacks"] = componentStacks
				} else {
					comp["stacks"] = []string{}
				}

				if processComponentDeps == true {
					componentDeps, err := FindComponentDependencies(stackName, "terraform", component.(string), baseComponentName, importsConfig)
					if err != nil {
						return nil, err
					}
					comp["deps"] = componentDeps
				} else {
					comp["deps"] = []string{}
				}

				terraformComponents[component.(string)] = comp
			}
		}
	}

	// Process all helmfile components
	if componentTypeFilter == "" || componentTypeFilter == "helmfile" {
		if allHelmfileComponents, ok := globalComponentsSection["helmfile"]; ok {
			allHelmfileComponentsMap := allHelmfileComponents.(map[interface{}]interface{})

			for component, v := range allHelmfileComponentsMap {
				componentMap := v.(map[interface{}]interface{})

				componentVars := map[interface{}]interface{}{}
				if i2, ok2 := componentMap["vars"]; ok2 {
					componentVars = i2.(map[interface{}]interface{})
				}

				componentSettings := map[interface{}]interface{}{}
				if i, ok2 := componentMap["settings"]; ok2 {
					componentSettings = i.(map[interface{}]interface{})
				}

				componentEnv := map[interface{}]interface{}{}
				if i, ok2 := componentMap["env"]; ok2 {
					componentEnv = i.(map[interface{}]interface{})
				}

				finalComponentVars, err := m.Merge([]map[interface{}]interface{}{globalAndHelmfileVars, componentVars})
				if err != nil {
					return nil, err
				}

				finalComponentSettings, err := m.Merge([]map[interface{}]interface{}{globalAndHelmfileSettings, componentSettings})
				if err != nil {
					return nil, err
				}

				finalComponentEnv, err := m.Merge([]map[interface{}]interface{}{globalAndHelmfileEnv, componentEnv})
				if err != nil {
					return nil, err
				}

				comp := map[string]interface{}{}
				comp["vars"] = finalComponentVars
				comp["settings"] = finalComponentSettings
				comp["env"] = finalComponentEnv

				if processStackDeps == true {
					componentStacks, err := FindComponentStacks("helmfile", component.(string), "", componentStackMap)
					if err != nil {
						return nil, err
					}
					comp["stacks"] = componentStacks
				} else {
					comp["stacks"] = []string{}
				}

				if processComponentDeps == true {
					componentDeps, err := FindComponentDependencies(stackName, "helmfile", component.(string), "", importsConfig)
					if err != nil {
						return nil, err
					}
					comp["deps"] = componentDeps
				} else {
					comp["deps"] = []string{}
				}

				helmfileComponents[component.(string)] = comp
			}
		}
	}

	allComponents["terraform"] = terraformComponents
	allComponents["helmfile"] = helmfileComponents

	result := map[interface{}]interface{}{
		"components": allComponents,
	}

	return result, nil
}
