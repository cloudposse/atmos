package stack

import (
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	c "github.com/cloudposse/atmos/pkg/convert"
	g "github.com/cloudposse/atmos/pkg/globals"
	m "github.com/cloudposse/atmos/pkg/merge"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

var (
	// Mutex to serialize updates of the result map of ProcessYAMLConfigFiles function
	processYAMLConfigFilesLock = &sync.Mutex{}
)

// ProcessYAMLConfigFiles takes a list of paths to YAML config files, processes and deep-merges all imports,
// and returns a list of stack configs
func ProcessYAMLConfigFiles(
	stacksBasePath string,
	terraformComponentsBasePath string,
	helmfileComponentsBasePath string,
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

			stackBasePath := stacksBasePath
			if len(stackBasePath) < 1 {
				stackBasePath = path.Dir(p)
			}

			stackConfig, importsConfig, err := ProcessYAMLConfigFile(stackBasePath, p, map[string]map[interface{}]interface{}{})
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
			// TODO: this feature is not used anywhere, it has old code and it has issues with some YAML stack configs
			// TODO: review it to use the new `atmos.yaml CLI stackConfig
			//if processStackDeps {
			//	componentStackMap, err = CreateComponentStackMap(stackBasePath, p)
			//	if err != nil {
			//		errorResult = err
			//		return
			//	}
			//}

			finalConfig, err := ProcessStackConfig(
				stackBasePath,
				terraformComponentsBasePath,
				helmfileComponentsBasePath,
				p,
				stackConfig,
				processStackDeps,
				processComponentDeps,
				"",
				componentStackMap,
				importsConfig,
				true)
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
					u.TrimBasePathFromPath(stackBasePath+"/", p),
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

// ProcessYAMLConfigFile takes a path to a YAML stack config file,
// recursively processes and deep-merges all imports,
// and returns stack config as map[interface{}]interface{}
func ProcessYAMLConfigFile(
	basePath string,
	filePath string,
	importsConfig map[string]map[interface{}]interface{}) (map[interface{}]interface{}, map[string]map[interface{}]interface{}, error) {

	var configs []map[interface{}]interface{}
	relativeFilePath := u.TrimBasePathFromPath(basePath+"/", filePath)

	stackYamlConfig, err := getFileContent(filePath)
	if err != nil {
		return nil, nil, err
	}

	stackMapConfig, err := c.YAMLToMapOfInterfaces(stackYamlConfig)
	if err != nil {
		e := errors.New(fmt.Sprintf("Invalid YAML file '%s'\n%v", relativeFilePath, err))
		return nil, nil, e
	}

	// Find and process all imports
	if importsSection, ok := stackMapConfig["import"]; ok {
		imports, ok := importsSection.([]interface{})
		if !ok {
			return nil, nil, errors.New(fmt.Sprintf("Invalid 'import' section in the file '%s'\nThe 'import' section must be a list of strings",
				relativeFilePath))
		}

		for _, im := range imports {
			imp, ok := im.(string)

			if !ok {
				if im == nil {
					return nil, nil, errors.New(fmt.Sprintf("Invalid import in the file '%s'\nThe import is an empty string",
						relativeFilePath))
				}
				return nil, nil, errors.New(fmt.Sprintf("Invalid import in the file '%s'\nThe import '%v' is not a valid string",
					relativeFilePath,
					im))
			}

			// If the import file is specified without extension, use `.yaml` as default
			impWithExt := imp
			ext := filepath.Ext(imp)
			if ext == "" {
				ext = g.DefaultStackConfigFileExtension
				impWithExt = imp + ext
			}

			impWithExtPath := path.Join(basePath, impWithExt)

			if impWithExtPath == filePath {
				errorMessage := fmt.Sprintf("Invalid import in the file '%s'\nThe file imports itself in '%s'",
					relativeFilePath,
					imp)
				return nil, nil, errors.New(errorMessage)
			}

			// Find all import matches in the glob
			importMatches, err := GetGlobMatches(impWithExtPath)

			if err != nil || importMatches == nil {
				importMatches, err = GetGlobMatches(impWithExtPath)
				if err != nil || importMatches == nil {
					errorMessage := fmt.Sprintf("Invalid import in the file '%s'\nNo matches found for the import '%s'",
						relativeFilePath,
						imp)
					return nil, nil, errors.New(errorMessage)
				}
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

	// Deep-merge the stack config file and all the imports
	result, err := m.Merge(configs)
	if err != nil {
		return nil, nil, err
	}

	return result, importsConfig, nil
}

// ProcessStackConfig takes a raw stack config, deep-merges all variables, settings, environments and backends,
// and returns the final stack configuration for all Terraform and helmfile components
func ProcessStackConfig(
	stacksBasePath string,
	terraformComponentsBasePath string,
	helmfileComponentsBasePath string,
	stack string,
	config map[interface{}]interface{},
	processStackDeps bool,
	processComponentDeps bool,
	componentTypeFilter string,
	componentStackMap map[string]map[string][]string,
	importsConfig map[string]map[interface{}]interface{},
	checkBaseComponentExists bool,
) (map[interface{}]interface{}, error) {

	stackName := strings.TrimSuffix(
		strings.TrimSuffix(
			u.TrimBasePathFromPath(stacksBasePath+"/", stack),
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
		terraformVars, ok = i.(map[interface{}]interface{})

		if !ok {
			return nil, errors.New(fmt.Sprintf("Invalid 'terraform.vars' section in the file '%s'", stack))
		}
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

	// Global backend
	globalBackendType := ""
	globalBackendSection := map[interface{}]interface{}{}
	if i, ok := globalTerraformSection["backend_type"]; ok {
		globalBackendType = i.(string)
	}
	if i, ok := globalTerraformSection["backend"]; ok {
		globalBackendSection = i.(map[interface{}]interface{})
	}

	// Global remote state backend
	globalRemoteStateBackendType := ""
	globalRemoteStateBackendSection := map[interface{}]interface{}{}
	if i, ok := globalTerraformSection["remote_state_backend_type"]; ok {
		globalRemoteStateBackendType = i.(string)
	}
	if i, ok := globalTerraformSection["remote_state_backend"]; ok {
		globalRemoteStateBackendSection = i.(map[interface{}]interface{})
	}

	// Helmfile section
	if i, ok := globalHelmfileSection["vars"]; ok {
		helmfileVars, ok = i.(map[interface{}]interface{})

		if !ok {
			return nil, errors.New(fmt.Sprintf("Invalid 'helmfile.vars' section in the file '%s'", stack))
		}
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

			allTerraformComponentsMap, ok := allTerraformComponents.(map[interface{}]interface{})
			if !ok {
				return nil, errors.New(fmt.Sprintf("Invalid 'components.terraform' section in the file '%s'", stack))
			}

			for cmp, v := range allTerraformComponentsMap {
				component := cmp.(string)
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

				// Component metadata.
				// This is per component, not deep-merged and not inherited from base components and globals.
				componentMetadata := map[interface{}]interface{}{}
				if i, ok2 := componentMap["metadata"]; ok2 {
					componentMetadata = i.(map[interface{}]interface{})
				}

				// Component backend
				componentBackendType := ""
				componentBackendSection := map[interface{}]interface{}{}
				if i, ok2 := componentMap["backend_type"]; ok2 {
					componentBackendType = i.(string)
				}
				if i, ok2 := componentMap["backend"]; ok2 {
					componentBackendSection = i.(map[interface{}]interface{})
				}

				// Component remote state backend
				componentRemoteStateBackendType := ""
				componentRemoteStateBackendSection := map[interface{}]interface{}{}
				if i, ok2 := componentMap["remote_state_backend_type"]; ok2 {
					componentRemoteStateBackendType = i.(string)
				}
				if i, ok2 := componentMap["remote_state_backend"]; ok2 {
					componentRemoteStateBackendSection = i.(map[interface{}]interface{})
				}

				componentTerraformCommand := ""
				if i, ok2 := componentMap["command"]; ok2 {
					componentTerraformCommand = i.(string)
				}

				// Process base component(s)
				baseComponentName := ""
				baseComponentVars := map[interface{}]interface{}{}
				baseComponentSettings := map[interface{}]interface{}{}
				baseComponentEnv := map[interface{}]interface{}{}
				baseComponentTerraformCommand := ""
				baseComponentBackendType := ""
				baseComponentBackendSection := map[interface{}]interface{}{}
				baseComponentRemoteStateBackendType := ""
				baseComponentRemoteStateBackendSection := map[interface{}]interface{}{}
				var baseComponentConfig BaseComponentConfig
				var componentInheritanceChain []string

				// Deprecated way of specifying inheritance with the top-level `component` attribute
				if baseComponent, baseComponentExist := componentMap["component"].(string); baseComponentExist {
					baseComponentName = baseComponent

					// Process the base components recursively to find `componentInheritanceChain`
					err = processBaseComponentConfig(
						&baseComponentConfig,
						allTerraformComponentsMap,
						component,
						stack,
						baseComponentName,
						terraformComponentsBasePath,
						checkBaseComponentExists,
					)
					if err != nil {
						return nil, err
					}

					baseComponentVars = baseComponentConfig.BaseComponentVars
					baseComponentSettings = baseComponentConfig.BaseComponentSettings
					baseComponentEnv = baseComponentConfig.BaseComponentEnv
					baseComponentName = baseComponentConfig.FinalBaseComponentName
					baseComponentTerraformCommand = baseComponentConfig.BaseComponentCommand
					baseComponentBackendType = baseComponentConfig.BaseComponentBackendType
					baseComponentBackendSection = baseComponentConfig.BaseComponentBackendSection
					baseComponentRemoteStateBackendType = baseComponentConfig.BaseComponentRemoteStateBackendType
					baseComponentRemoteStateBackendSection = baseComponentConfig.BaseComponentRemoteStateBackendSection
					componentInheritanceChain = baseComponentConfig.ComponentInheritanceChain
				}

				// Multiple inheritance (and multiple-inheritance chain) using `metadata.component` and `metadata.inherit`.
				// `metadata.component` points to the component implementation (e.g. in `components/terraform` folder),
				//  it does not specify inheritance (it overrides the deprecated top-level `component` attribute).
				// `metadata.inherit` is a list of component names from which the current component inherits.
				// It uses a method similar to Method Resolution Order (MRO), which is how Python supports multiple inheritance.
				//
				// In the case of multiple base components, it is processed left to right, in the order by which it was declared.
				// For example: `metadata.inherits: [componentA, componentB]`
				// will deep-merge all the base components of `componentA` (each component overriding its base),
				// then all the base components of `componentB` (each component overriding its base),
				// then the two results are deep-merged together (`componentB` inheritance chain will override values from `componentA' inheritance chain).
				if baseComponentFromMetadata, baseComponentFromMetadataExist := componentMetadata["component"].(string); baseComponentFromMetadataExist {
					baseComponentName = baseComponentFromMetadata
				}

				if inheritList, inheritListExist := componentMetadata["inherits"].([]interface{}); inheritListExist {
					for _, v := range inheritList {
						base := v.(string)

						if _, ok2 := allTerraformComponentsMap[base]; !ok2 {
							if checkBaseComponentExists {
								errorMessage := fmt.Sprintf("The component '%[1]s' in the stack '%[2]s' inherits from '%[3]s' "+
									"(using 'metadata.inherits'), but '%[3]s' is not defined in any of the YAML config files for the stack '%[2]s'",
									component,
									stackName,
									base,
								)
								return nil, errors.New(errorMessage)
							}
						}

						// Process the base components recursively to find `componentInheritanceChain`
						err = processBaseComponentConfig(
							&baseComponentConfig,
							allTerraformComponentsMap,
							component,
							stack,
							base,
							terraformComponentsBasePath,
							checkBaseComponentExists,
						)
						if err != nil {
							return nil, err
						}

						baseComponentVars = baseComponentConfig.BaseComponentVars
						baseComponentSettings = baseComponentConfig.BaseComponentSettings
						baseComponentEnv = baseComponentConfig.BaseComponentEnv
						baseComponentTerraformCommand = baseComponentConfig.BaseComponentCommand
						baseComponentBackendType = baseComponentConfig.BaseComponentBackendType
						baseComponentBackendSection = baseComponentConfig.BaseComponentBackendSection
						baseComponentRemoteStateBackendType = baseComponentConfig.BaseComponentRemoteStateBackendType
						baseComponentRemoteStateBackendSection = baseComponentConfig.BaseComponentRemoteStateBackendSection
						componentInheritanceChain = baseComponentConfig.ComponentInheritanceChain
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

				// Final backend
				finalComponentBackendType := globalBackendType
				if len(baseComponentBackendType) > 0 {
					finalComponentBackendType = baseComponentBackendType
				}
				if len(componentBackendType) > 0 {
					finalComponentBackendType = componentBackendType
				}

				finalComponentBackendSection, err := m.Merge([]map[interface{}]interface{}{globalBackendSection,
					baseComponentBackendSection,
					componentBackendSection})
				if err != nil {
					return nil, err
				}

				finalComponentBackend := map[interface{}]interface{}{}
				if i, ok2 := finalComponentBackendSection[finalComponentBackendType]; ok2 {
					finalComponentBackend = i.(map[interface{}]interface{})
				}

				// Check if `backend` section has `workspace_key_prefix` for `s3` backend type
				// If it does not, use the component name instead
				// It will also be propagated to `remote_state_backend` section of `s3` type
				if finalComponentBackendType == "s3" {
					if _, ok2 := finalComponentBackend["workspace_key_prefix"].(string); !ok2 {
						workspaceKeyPrefixComponent := component
						if baseComponentName != "" {
							workspaceKeyPrefixComponent = baseComponentName
						}
						finalComponentBackend["workspace_key_prefix"] = strings.Replace(workspaceKeyPrefixComponent, "/", "-", -1)
					}
				}

				// Check if component `backend` section has `key` for `azurerm` backend type
				// If it does not, use the component name instead and format it with the global backend key name to auto generate a unique tf state key
				// The backend state file will be formatted like so: {global key name}/{component name}.terraform.tfstate
				if finalComponentBackendType == "azurerm" {
					if componentAzurerm, componentAzurermExists := componentBackendSection["azurerm"].(map[interface{}]interface{}); !componentAzurermExists {
						if _, componentAzurermKeyExists := componentAzurerm["key"].(string); !componentAzurermKeyExists {
							azureKeyPrefixComponent := component
							baseKeyName := ""
							if baseComponentName != "" {
								azureKeyPrefixComponent = baseComponentName
							}
							if globalAzurerm, globalAzurermExists := globalBackendSection["azurerm"].(map[interface{}]interface{}); globalAzurermExists {
								baseKeyName = globalAzurerm["key"].(string)
							}
							componentKeyName := strings.Replace(azureKeyPrefixComponent, "/", "-", -1)
							finalComponentBackend["key"] = fmt.Sprintf("%s/%s.terraform.tfstate", baseKeyName, componentKeyName)

						}
					}
				}

				// Final remote state backend
				finalComponentRemoteStateBackendType := finalComponentBackendType
				if len(globalRemoteStateBackendType) > 0 {
					finalComponentRemoteStateBackendType = globalRemoteStateBackendType
				}
				if len(baseComponentRemoteStateBackendType) > 0 {
					finalComponentRemoteStateBackendType = baseComponentRemoteStateBackendType
				}
				if len(componentRemoteStateBackendType) > 0 {
					finalComponentRemoteStateBackendType = componentRemoteStateBackendType
				}

				finalComponentRemoteStateBackendSection, err := m.Merge([]map[interface{}]interface{}{globalRemoteStateBackendSection,
					baseComponentRemoteStateBackendSection,
					componentRemoteStateBackendSection})
				if err != nil {
					return nil, err
				}

				// Merge `backend` and `remote_state_backend` sections
				// This will allow keeping `remote_state_backend` section DRY
				finalComponentRemoteStateBackendSectionMerged, err := m.Merge([]map[interface{}]interface{}{finalComponentBackendSection,
					finalComponentRemoteStateBackendSection})
				if err != nil {
					return nil, err
				}

				finalComponentRemoteStateBackend := map[interface{}]interface{}{}
				if i, ok2 := finalComponentRemoteStateBackendSectionMerged[finalComponentRemoteStateBackendType]; ok2 {
					finalComponentRemoteStateBackend = i.(map[interface{}]interface{})
				}

				// Final binary to execute
				finalComponentTerraformCommand := "terraform"
				if len(baseComponentTerraformCommand) > 0 {
					finalComponentTerraformCommand = baseComponentTerraformCommand
				}
				if len(componentTerraformCommand) > 0 {
					finalComponentTerraformCommand = componentTerraformCommand
				}

				// If the component is not deployable (`metadata.type: abstract`), remove `settings.spacelift.workspace_enabled` from the map).
				// This will prevent the derived components from inheriting `settings.spacelift.workspace_enabled=false` of not-deployable components.
				// Also, removing `settings.spacelift.workspace_enabled` will effectively make it `false`
				// and `spacelift_stack_processor` will not create a Spacelift stack for the abstract component
				// even if `settings.spacelift.workspace_enabled` was set to `true`.
				// This is per component, not deep-merged and not inherited from base components and globals.
				componentIsAbstract := false
				if componentType, componentTypeAttributeExists := componentMetadata["type"].(string); componentTypeAttributeExists {
					if componentType == "abstract" {
						componentIsAbstract = true
					}
				}
				if componentIsAbstract == true {
					if i, ok2 := finalComponentSettings["spacelift"]; ok2 {
						spaceliftSettings := i.(map[interface{}]interface{})

						if _, ok3 := spaceliftSettings["workspace_enabled"]; ok3 {
							delete(spaceliftSettings, "workspace_enabled")
						}
					}
				}

				comp := map[string]interface{}{}
				comp["vars"] = finalComponentVars
				comp["settings"] = finalComponentSettings
				comp["env"] = finalComponentEnv
				comp["backend_type"] = finalComponentBackendType
				comp["backend"] = finalComponentBackend
				comp["remote_state_backend_type"] = finalComponentRemoteStateBackendType
				comp["remote_state_backend"] = finalComponentRemoteStateBackend
				comp["command"] = finalComponentTerraformCommand
				comp["inheritance"] = componentInheritanceChain
				comp["metadata"] = componentMetadata

				if baseComponentName != "" {
					comp["component"] = baseComponentName
				}

				// TODO: this feature is not used anywhere, it has old code and it has issues with some YAML stack configs
				// TODO: review it to use the new `atmos.yaml` CLI config
				//if processStackDeps == true {
				//	componentStacks, err := FindComponentStacks("terraform", component, baseComponentName, componentStackMap)
				//	if err != nil {
				//		return nil, err
				//	}
				//	comp["stacks"] = componentStacks
				//} else {
				//	comp["stacks"] = []string{}
				//}

				if processComponentDeps == true {
					componentDeps, err := FindComponentDependencies(stackName, "terraform", component, baseComponentName, importsConfig)
					if err != nil {
						return nil, err
					}
					comp["deps"] = componentDeps
				} else {
					comp["deps"] = []string{}
				}

				terraformComponents[component] = comp
			}
		}
	}

	// Process all helmfile components
	if componentTypeFilter == "" || componentTypeFilter == "helmfile" {
		if allHelmfileComponents, ok := globalComponentsSection["helmfile"]; ok {

			allHelmfileComponentsMap, ok := allHelmfileComponents.(map[interface{}]interface{})
			if !ok {
				return nil, errors.New(fmt.Sprintf("Invalid 'components.helmfile' section in the file '%s'", stack))
			}

			for cmp, v := range allHelmfileComponentsMap {
				component := cmp.(string)
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

				// Component metadata.
				// This is per component, not deep-merged and not inherited from base components and globals.
				componentMetadata := map[interface{}]interface{}{}
				if i, ok2 := componentMap["metadata"]; ok2 {
					componentMetadata = i.(map[interface{}]interface{})
				}

				componentHelmfileCommand := ""
				if i, ok2 := componentMap["command"]; ok2 {
					componentHelmfileCommand = i.(string)
				}

				// Process base component(s)
				baseComponentVars := map[interface{}]interface{}{}
				baseComponentSettings := map[interface{}]interface{}{}
				baseComponentEnv := map[interface{}]interface{}{}
				baseComponentName := ""
				baseComponentHelmfileCommand := ""
				var baseComponentConfig BaseComponentConfig
				var componentInheritanceChain []string

				// Deprecated way of specifying inheritance with the top-level `component` attribute
				if baseComponent, baseComponentExist := componentMap["component"].(string); baseComponentExist {
					baseComponentName = baseComponent

					// Process the base components recursively to find `componentInheritanceChain`
					err = processBaseComponentConfig(
						&baseComponentConfig,
						allHelmfileComponentsMap,
						component,
						stack,
						baseComponentName,
						helmfileComponentsBasePath,
						checkBaseComponentExists,
					)
					if err != nil {
						return nil, err
					}

					baseComponentVars = baseComponentConfig.BaseComponentVars
					baseComponentSettings = baseComponentConfig.BaseComponentSettings
					baseComponentEnv = baseComponentConfig.BaseComponentEnv
					baseComponentName = baseComponentConfig.FinalBaseComponentName
					baseComponentHelmfileCommand = baseComponentConfig.BaseComponentCommand
					componentInheritanceChain = baseComponentConfig.ComponentInheritanceChain
				}

				// Multiple inheritance (and multiple-inheritance chain) using `metadata.component` and `metadata.inherit`.
				// `metadata.component` points to the component implementation (e.g. in `components/terraform` folder),
				//  it does not specify inheritance (it overrides the deprecated top-level `component` attribute).
				// `metadata.inherit` is a list of component names from which the current component inherits.
				// It uses a method similar to Method Resolution Order (MRO), which is how Python supports multiple inheritance.
				//
				// In the case of multiple base components, it is processed left to right, in the order by which it was declared.
				// For example: `metadata.inherits: [componentA, componentB]`
				// will deep-merge all the base components of `componentA` (each component overriding its base),
				// then all the base components of `componentB` (each component overriding its base),
				// then the two results are deep-merged together (`componentB` inheritance chain will override values from `componentA' inheritance chain).
				if baseComponentFromMetadata, baseComponentFromMetadataExist := componentMetadata["component"].(string); baseComponentFromMetadataExist {
					baseComponentName = baseComponentFromMetadata
				}

				if inheritList, inheritListExist := componentMetadata["inherits"].([]interface{}); inheritListExist {
					for _, v := range inheritList {
						base := v.(string)

						if _, ok2 := allHelmfileComponentsMap[base]; !ok2 {
							if checkBaseComponentExists {
								errorMessage := fmt.Sprintf("The component '%[1]s' in the stack '%[2]s' inherits from '%[3]s' "+
									"(using 'metadata.inherits'), but '%[3]s' is not defined in any of the YAML config files for the stack '%[2]s'",
									component,
									stackName,
									base,
								)
								return nil, errors.New(errorMessage)
							}
						}

						// Process the base components recursively to find `componentInheritanceChain`
						err = processBaseComponentConfig(
							&baseComponentConfig,
							allHelmfileComponentsMap,
							component,
							stack,
							base,
							helmfileComponentsBasePath,
							checkBaseComponentExists,
						)
						if err != nil {
							return nil, err
						}

						baseComponentVars = baseComponentConfig.BaseComponentVars
						baseComponentSettings = baseComponentConfig.BaseComponentSettings
						baseComponentEnv = baseComponentConfig.BaseComponentEnv
						baseComponentName = baseComponentConfig.FinalBaseComponentName
						baseComponentHelmfileCommand = baseComponentConfig.BaseComponentCommand
						componentInheritanceChain = baseComponentConfig.ComponentInheritanceChain
					}
				}

				finalComponentVars, err := m.Merge([]map[interface{}]interface{}{globalAndHelmfileVars, baseComponentVars, componentVars})
				if err != nil {
					return nil, err
				}

				finalComponentSettings, err := m.Merge([]map[interface{}]interface{}{globalAndHelmfileSettings, baseComponentSettings, componentSettings})
				if err != nil {
					return nil, err
				}

				finalComponentEnv, err := m.Merge([]map[interface{}]interface{}{globalAndHelmfileEnv, baseComponentEnv, componentEnv})
				if err != nil {
					return nil, err
				}

				// Final binary to execute
				finalComponentHelmfileCommand := "helmfile"
				if len(baseComponentHelmfileCommand) > 0 {
					finalComponentHelmfileCommand = baseComponentHelmfileCommand
				}
				if len(componentHelmfileCommand) > 0 {
					finalComponentHelmfileCommand = componentHelmfileCommand
				}

				comp := map[string]interface{}{}
				comp["vars"] = finalComponentVars
				comp["settings"] = finalComponentSettings
				comp["env"] = finalComponentEnv
				comp["command"] = finalComponentHelmfileCommand
				comp["inheritance"] = componentInheritanceChain
				comp["metadata"] = componentMetadata

				if baseComponentName != "" {
					comp["component"] = baseComponentName
				}

				// TODO: this feature is not used anywhere, it has old code and it has issues with some YAML stack configs
				// TODO: review it to use the new `atmos.yaml` CLI config
				//if processStackDeps == true {
				//	componentStacks, err := FindComponentStacks("helmfile", component, baseComponentName, componentStackMap)
				//	if err != nil {
				//		return nil, err
				//	}
				//	comp["stacks"] = componentStacks
				//} else {
				//	comp["stacks"] = []string{}
				//}

				if processComponentDeps == true {
					componentDeps, err := FindComponentDependencies(stackName, "helmfile", component, baseComponentName, importsConfig)
					if err != nil {
						return nil, err
					}
					comp["deps"] = componentDeps
				} else {
					comp["deps"] = []string{}
				}

				helmfileComponents[component] = comp
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

// processBaseComponentConfig processes base component(s) config
func processBaseComponentConfig(
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

	if baseComponentSection, baseComponentSectionExist := allComponentsMap[baseComponent]; baseComponentSectionExist {
		baseComponentMap = baseComponentSection.(map[interface{}]interface{})

		// First, process the base component of this base component
		if baseComponentOfBaseComponent, baseComponentOfBaseComponentExist := baseComponentMap["component"]; baseComponentOfBaseComponentExist {
			err := processBaseComponentConfig(
				baseComponentConfig,
				allComponentsMap,
				baseComponent,
				stack,
				baseComponentOfBaseComponent.(string),
				componentBasePath,
				checkBaseComponentExists,
			)

			if err != nil {
				return err
			}
		}

		if baseComponentVarsSection, baseComponentVarsSectionExist := baseComponentMap["vars"]; baseComponentVarsSectionExist {
			baseComponentVars = baseComponentVarsSection.(map[interface{}]interface{})
		}

		if baseComponentSettingsSection, baseComponentSettingsSectionExist := baseComponentMap["settings"]; baseComponentSettingsSectionExist {
			baseComponentSettings = baseComponentSettingsSection.(map[interface{}]interface{})
		}

		if baseComponentEnvSection, baseComponentEnvSectionExist := baseComponentMap["env"]; baseComponentEnvSectionExist {
			baseComponentEnv = baseComponentEnvSection.(map[interface{}]interface{})
		}

		// Base component backend
		if i, ok2 := baseComponentMap["backend_type"]; ok2 {
			baseComponentBackendType = i.(string)
		}
		if i, ok2 := baseComponentMap["backend"]; ok2 {
			baseComponentBackendSection = i.(map[interface{}]interface{})
		}

		// Base component remote state backend
		if i, ok2 := baseComponentMap["remote_state_backend_type"]; ok2 {
			baseComponentRemoteStateBackendType = i.(string)
		}
		if i, ok2 := baseComponentMap["remote_state_backend"]; ok2 {
			baseComponentRemoteStateBackendSection = i.(map[interface{}]interface{})
		}

		// Base component `command`
		if baseComponentCommandSection, baseComponentCommandSectionExist := baseComponentMap["command"]; baseComponentCommandSectionExist {
			baseComponentCommand = baseComponentCommandSection.(string)
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
