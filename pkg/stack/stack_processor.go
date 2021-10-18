package stack

import (
	"fmt"
	"github.com/bmatcuk/doublestar"
	g "github.com/cloudposse/atmos/internal/globals"
	c "github.com/cloudposse/atmos/pkg/convert"
	m "github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/utils"
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

			stackBasePath := basePath
			if len(stackBasePath) < 1 {
				stackBasePath = path.Dir(p)
			}

			config, importsConfig, err := ProcessYAMLConfigFile(stackBasePath, p, map[string]map[interface{}]interface{}{})
			if err != nil {
				errorResult = err
				return
			}

			var imports []string
			for k := range importsConfig {
				imports = append(imports, k)
			}

			uniqueImports := utils.UniqueStrings(imports)
			sort.Strings(uniqueImports)

			componentStackMap := map[string]map[string][]string{}
			if processStackDeps {
				componentStackMap, err = CreateComponentStackMap(stackBasePath, p)
				if err != nil {
					errorResult = err
					return
				}
			}

			finalConfig, err := ProcessConfig(stackBasePath, p, config, processStackDeps, processComponentDeps, "", componentStackMap, importsConfig)
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

			stackName := strings.TrimSuffix(
				strings.TrimSuffix(
					utils.TrimBasePathFromPath(stackBasePath+"/", p),
					g.DefaultStackConfigFileExtension),
				".yml",
			)

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
				errorMessage := fmt.Sprintf("Invalid import in the config file %s.\nThe file imports itself in '%s'",
					filePath,
					strings.Replace(impWithExt, basePath+"/", "", 1))
				return nil, nil, errors.New(errorMessage)
			}

			// Find all import matches in the glob
			importMatches, err := doublestar.Glob(impWithExtPath)
			if err != nil {
				return nil, nil, err
			}

			if importMatches == nil {
				errorMessage := fmt.Sprintf("Invalid import in the config file %s.\nNo matches found for the import '%s'",
					filePath,
					strings.Replace(impWithExt, basePath+"/", "", 1))
				return nil, nil, errors.New(errorMessage)
			}

			for _, importFile := range importMatches {
				yamlConfig, _, err := ProcessYAMLConfigFile(basePath, importFile, importsConfig)
				if err != nil {
					return nil, nil, err
				}

				configs = append(configs, yamlConfig)
				importRelativePathWithExt := strings.Replace(importFile, basePath+"/", "", 1)
				ext2 := filepath.Ext(importRelativePathWithExt)
				if ext2 == "" {
					ext2 = g.DefaultStackConfigFileExtension
				}
				importRelativePathWithoutExt := strings.TrimSuffix(importRelativePathWithExt, ext2)
				importsConfig[importRelativePathWithoutExt] = yamlConfig
			}
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

	stackName := strings.TrimSuffix(
		strings.TrimSuffix(
			utils.TrimBasePathFromPath(basePath+"/", stack),
			g.DefaultStackConfigFileExtension),
		".yml",
	)

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

				componentTerraformCommand := "terraform"
				if i, ok2 := componentMap["command"]; ok2 {
					componentTerraformCommand = i.(string)
				}

				baseComponentVars := map[interface{}]interface{}{}
				baseComponentSettings := map[interface{}]interface{}{}
				baseComponentEnv := map[interface{}]interface{}{}
				baseComponentBackend := map[interface{}]interface{}{}
				baseComponentName := ""
				baseComponentTerraformCommand := ""

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

						if baseComponentCommandSection, baseComponentCommandSectionExist := baseComponentMap["command"]; baseComponentCommandSectionExist {
							baseComponentTerraformCommand = baseComponentCommandSection.(string)
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

				finalComponentTerraformCommand := componentTerraformCommand
				if len(baseComponentTerraformCommand) > 0 {
					finalComponentTerraformCommand = baseComponentTerraformCommand
				}
				comp := map[string]interface{}{}
				comp["vars"] = finalComponentVars
				comp["settings"] = finalComponentSettings
				comp["env"] = finalComponentEnv
				comp["backend_type"] = backendType
				comp["backend"] = finalComponentBackend
				comp["command"] = finalComponentTerraformCommand

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

				componentHelmfileCommand := "helmfile"
				if i, ok2 := componentMap["command"]; ok2 {
					componentHelmfileCommand = i.(string)
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
				comp["command"] = componentHelmfileCommand

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
