package stack

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"text/template"
	"text/template/parse"

	"github.com/Masterminds/sprig/v3"
	"github.com/pkg/errors"
	"github.com/santhosh-tekuri/jsonschema/v5"
	"gopkg.in/yaml.v2"

	cfg "github.com/cloudposse/atmos/pkg/config"
	c "github.com/cloudposse/atmos/pkg/convert"
	m "github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

var (
	// Mutex to serialize updates of the result map of ProcessYAMLConfigFiles function
	processYAMLConfigFilesLock = &sync.Mutex{}
)

// ProcessYAMLConfigFiles takes a list of paths to stack manifests, processes and deep-merges all imports,
// and returns a list of stack configs
func ProcessYAMLConfigFiles(
	stacksBasePath string,
	terraformComponentsBasePath string,
	helmfileComponentsBasePath string,
	filePaths []string,
	processStackDeps bool,
	processComponentDeps bool,
	ignoreMissingFiles bool,
) (
	[]string,
	map[string]any,
	map[string]map[string]any,
	error,
) {

	count := len(filePaths)
	listResult := make([]string, count)
	mapResult := map[string]any{}
	rawStackConfigs := map[string]map[string]any{}
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

			stackFileName := strings.TrimSuffix(
				strings.TrimSuffix(
					u.TrimBasePathFromPath(stackBasePath+"/", p),
					cfg.DefaultStackConfigFileExtension),
				".yml",
			)

			deepMergedStackConfig, importsConfig, stackConfig, err := ProcessYAMLConfigFile(
				stackBasePath,
				p,
				map[string]map[any]any{},
				nil,
				ignoreMissingFiles,
				false,
				false,
				map[any]any{},
				map[any]any{},
				"",
			)

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

			finalConfig, err := ProcessStackConfig(
				stackBasePath,
				terraformComponentsBasePath,
				helmfileComponentsBasePath,
				p,
				deepMergedStackConfig,
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

			processYAMLConfigFilesLock.Lock()
			defer processYAMLConfigFilesLock.Unlock()

			listResult[i] = string(yamlConfig)
			mapResult[stackFileName] = finalConfig
			rawStackConfigs[stackFileName] = map[string]any{}
			rawStackConfigs[stackFileName]["stack"] = stackConfig
			rawStackConfigs[stackFileName]["imports"] = importsConfig
			rawStackConfigs[stackFileName]["import_files"] = uniqueImports
		}(i, filePath)
	}

	wg.Wait()

	if errorResult != nil {
		return nil, nil, nil, errorResult
	}

	return listResult, mapResult, rawStackConfigs, nil
}

// ProcessYAMLConfigFile takes a path to a YAML stack manifest,
// recursively processes and deep-merges all imports,
// and returns the final stack config
func ProcessYAMLConfigFile(
	basePath string,
	filePath string,
	importsConfig map[string]map[any]any,
	context map[string]any,
	ignoreMissingFiles bool,
	skipTemplatesProcessingInImports bool,
	ignoreMissingTemplateValues bool,
	parentTerraformOverrides map[any]any,
	parentHelmfileOverrides map[any]any,
	atmosManifestJsonSchemaFilePath string,
) (
	map[any]any,
	map[string]map[any]any,
	map[any]any,
	error,
) {

	var stackConfigs []map[any]any
	relativeFilePath := u.TrimBasePathFromPath(basePath+"/", filePath)

	globalTerraformSection := map[any]any{}
	globalHelmfileSection := map[any]any{}
	globalOverrides := map[any]any{}
	terraformOverrides := map[any]any{}
	helmfileOverrides := map[any]any{}
	globalAndTerraformOverrides := map[any]any{}
	globalAndHelmfileOverrides := map[any]any{}
	finalTerraformOverrides := map[any]any{}
	finalHelmfileOverrides := map[any]any{}

	stackYamlConfig, err := getFileContent(filePath)

	// If the file does not exist (`err != nil`), and `ignoreMissingFiles = true`, don't return the error.
	// `ignoreMissingFiles = true` is used when executing `atmos describe affected` command.
	// If we add a new stack manifest with some component configurations to the current branch, then the new file will not be present in
	// the remote branch (with which the current branch is compared), and `atmos` would throw an error.
	if err != nil && !ignoreMissingFiles {
		return nil, nil, nil, err
	}

	// Process `Go` templates in the stack manifest using the provided context
	if !skipTemplatesProcessingInImports && len(context) > 0 {
		stackYamlConfig, err = u.ProcessTmpl(relativeFilePath, stackYamlConfig, context, ignoreMissingTemplateValues)
		if err != nil {
			return nil, nil, nil, err
		}
	}

	stackConfigMap, err := c.YAMLToMapOfInterfaces(stackYamlConfig)
	if err != nil {
		e := fmt.Errorf("invalid stack manifest '%s'\n%v", relativeFilePath, err)
		return nil, nil, nil, e
	}

	// If the path to the Atmos manifest JSON Schema is provided, validate the stack manifest against it
	if atmosManifestJsonSchemaFilePath != "" {
		// Convert the data to JSON and back to Go map to prevent the error:
		// jsonschema: invalid jsonType: map[interface {}]interface {}
		dataJson, err := u.ConvertToJSONFast(stackConfigMap)
		if err != nil {
			return nil, nil, nil, err
		}

		dataFromJson, err := u.ConvertFromJSON(dataJson)
		if err != nil {
			return nil, nil, nil, err
		}

		compiler := jsonschema.NewCompiler()

		atmosManifestJsonSchemaFileReader, err := os.Open(atmosManifestJsonSchemaFilePath)
		if err != nil {
			return nil, nil, nil, err
		}

		if err := compiler.AddResource(atmosManifestJsonSchemaFilePath, atmosManifestJsonSchemaFileReader); err != nil {
			return nil, nil, nil, err
		}

		compiler.Draft = jsonschema.Draft2020

		compiledSchema, err := compiler.Compile(atmosManifestJsonSchemaFilePath)
		if err != nil {
			return nil, nil, nil, err
		}

		if err = compiledSchema.Validate(dataFromJson); err != nil {
			switch e := err.(type) {
			case *jsonschema.ValidationError:
				b, err2 := json.MarshalIndent(e.BasicOutput(), "", "  ")
				if err2 != nil {
					return nil, nil, nil, err2
				}
				return nil, nil, nil, errors.New(string(b))
			default:
				return nil, nil, nil, err
			}
		}
	}

	// Check if the `overrides` sections exist and if we need to process overrides for the components in this stack manifest and its imports
	if i, ok := stackConfigMap[cfg.OverridesSectionName]; ok {
		if globalOverrides, ok = i.(map[any]any); !ok {
			return nil, nil, nil, fmt.Errorf("invalid 'overrides' section in the stack manifest '%s'", relativeFilePath)
		}

		// Terraform overrides
		if o, ok := stackConfigMap["terraform"]; ok {
			if globalTerraformSection, ok = o.(map[any]any); !ok {
				return nil, nil, nil, fmt.Errorf("invalid 'terraform' section in the stack manifest '%s'", relativeFilePath)
			}

			if i, ok := globalTerraformSection[cfg.OverridesSectionName]; ok {
				if terraformOverrides, ok = i.(map[any]any); !ok {
					return nil, nil, nil, fmt.Errorf("invalid 'terraform.overrides' section in the stack manifest '%s'", relativeFilePath)
				}
			}

			globalAndTerraformOverrides, err = m.Merge([]map[any]any{globalOverrides, terraformOverrides})
			if err != nil {
				return nil, nil, nil, err
			}
		}

		finalTerraformOverrides, err = m.Merge([]map[any]any{globalAndTerraformOverrides, parentTerraformOverrides})
		if err != nil {
			return nil, nil, nil, err
		}

		// Helmfile overrides
		if o, ok := stackConfigMap["helmfile"]; ok {
			if globalHelmfileSection, ok = o.(map[any]any); !ok {
				return nil, nil, nil, fmt.Errorf("invalid 'helmfile' section in the stack manifest '%s'", relativeFilePath)
			}

			if i, ok := globalHelmfileSection[cfg.OverridesSectionName]; ok {
				if helmfileOverrides, ok = i.(map[any]any); !ok {
					return nil, nil, nil, fmt.Errorf("invalid 'terraform.overrides' section in the stack manifest '%s'", relativeFilePath)
				}
			}

			globalAndHelmfileOverrides, err = m.Merge([]map[any]any{globalOverrides, helmfileOverrides})
			if err != nil {
				return nil, nil, nil, err
			}
		}

		finalHelmfileOverrides, err = m.Merge([]map[any]any{globalAndHelmfileOverrides, parentHelmfileOverrides})
		if err != nil {
			return nil, nil, nil, err
		}
	} else {
		finalTerraformOverrides = parentTerraformOverrides
		finalHelmfileOverrides = parentHelmfileOverrides
	}

	// Add the `overrides` section for all components in this manifest
	if len(finalTerraformOverrides) > 0 || len(finalHelmfileOverrides) > 0 {
		if componentsSection, ok := stackConfigMap["components"].(map[any]any); ok {
			// Terraform
			if len(finalTerraformOverrides) > 0 {
				if terraformSection, ok := componentsSection["terraform"].(map[any]any); ok {
					for _, compSection := range terraformSection {
						if componentSection, ok := compSection.(map[any]any); ok {
							componentSection["overrides"] = finalTerraformOverrides
						}
					}
				}
			}

			// Helmfile
			if len(finalHelmfileOverrides) > 0 {
				if helmfileSection, ok := componentsSection["helmfile"].(map[any]any); ok {
					for _, compSection := range helmfileSection {
						if componentSection, ok := compSection.(map[any]any); ok {
							componentSection["overrides"] = finalHelmfileOverrides
						}
					}
				}
			}
		}
	}

	// Find and process all imports
	importStructs, err := processImportSection(stackConfigMap, relativeFilePath)
	if err != nil {
		return nil, nil, nil, err
	}

	for _, importStruct := range importStructs {
		imp := importStruct.Path

		if imp == "" {
			return nil, nil, nil, fmt.Errorf("invalid empty import in the file '%s'", relativeFilePath)
		}

		// If the import file is specified without extension, use `.yaml` as default
		impWithExt := imp
		ext := filepath.Ext(imp)
		if ext == "" {
			ext = cfg.DefaultStackConfigFileExtension
			impWithExt = imp + ext
		}

		impWithExtPath := path.Join(basePath, impWithExt)

		if impWithExtPath == filePath {
			errorMessage := fmt.Sprintf("invalid import in the file '%s'\nThe file imports itself in '%s'",
				relativeFilePath,
				imp)
			return nil, nil, nil, errors.New(errorMessage)
		}

		// Find all import matches in the glob
		importMatches, err := u.GetGlobMatches(impWithExtPath)
		if err != nil || len(importMatches) == 0 {
			// Retry (b/c we are using `doublestar` library and it sometimes has issues reading many files in a Docker container)
			// TODO: review `doublestar` library

			importMatches, err = u.GetGlobMatches(impWithExtPath)
			if err != nil || len(importMatches) == 0 {
				// The import was not found -> check if the import is a Go template; if not, return the error
				t, err2 := template.New(imp).Funcs(sprig.FuncMap()).Parse(imp)
				if err2 != nil {
					return nil, nil, nil, err2
				}

				isGoTemplate := false

				// Iterate over all nodes in the template and check if any of them is of type `NodeAction` (field evaluation)
				for _, node := range t.Root.Nodes {
					if node.Type() == parse.NodeAction {
						isGoTemplate = true
						break
					}
				}

				// If the import is not a Go template, return the error
				if !isGoTemplate {
					if err != nil {
						errorMessage := fmt.Sprintf("no matches found for the import '%s' in the file '%s'\nError: %s",
							imp,
							relativeFilePath,
							err,
						)
						return nil, nil, nil, errors.New(errorMessage)
					} else if importMatches == nil {
						errorMessage := fmt.Sprintf("no matches found for the import '%s' in the file '%s'",
							imp,
							relativeFilePath,
						)
						return nil, nil, nil, errors.New(errorMessage)
					}
				}
			}
		}

		// Support `context` in hierarchical imports.
		// Deep-merge the parent `context` with the current `context` and propagate the result to the entire chain of imports.
		// The parent `context` takes precedence over the current (imported) `context` and will override items with the same keys.
		// TODO: instead of calling the conversion functions, we need to switch to generics and update everything to support it
		listOfMaps := []map[any]any{c.MapsOfStringsToMapsOfInterfaces(importStruct.Context), c.MapsOfStringsToMapsOfInterfaces(context)}
		mergedContext, err := m.Merge(listOfMaps)
		if err != nil {
			return nil, nil, nil, err
		}

		for _, importFile := range importMatches {
			yamlConfig, _, yamlConfigRaw, err := ProcessYAMLConfigFile(
				basePath,
				importFile,
				importsConfig,
				c.MapsOfInterfacesToMapsOfStrings(mergedContext),
				ignoreMissingFiles,
				importStruct.SkipTemplatesProcessing,
				importStruct.IgnoreMissingTemplateValues,
				finalTerraformOverrides,
				finalHelmfileOverrides,
				"",
			)
			if err != nil {
				return nil, nil, nil, err
			}

			stackConfigs = append(stackConfigs, yamlConfig)
			importRelativePathWithExt := strings.Replace(importFile, basePath+"/", "", 1)
			ext2 := filepath.Ext(importRelativePathWithExt)
			if ext2 == "" {
				ext2 = cfg.DefaultStackConfigFileExtension
			}
			importRelativePathWithoutExt := strings.TrimSuffix(importRelativePathWithExt, ext2)
			importsConfig[importRelativePathWithoutExt] = yamlConfigRaw
		}
	}

	if len(stackConfigMap) > 0 {
		stackConfigs = append(stackConfigs, stackConfigMap)
	}

	// Deep-merge the stack manifest and all the imports
	stackConfigsDeepMerged, err := m.Merge(stackConfigs)
	if err != nil {
		return nil, nil, nil, err
	}

	return stackConfigsDeepMerged, importsConfig, stackConfigMap, nil
}

// ProcessStackConfig takes a stack manifest, deep-merges all variables, settings, environments and backends,
// and returns the final stack configuration for all Terraform and helmfile components
func ProcessStackConfig(
	stacksBasePath string,
	terraformComponentsBasePath string,
	helmfileComponentsBasePath string,
	stack string,
	config map[any]any,
	processStackDeps bool,
	processComponentDeps bool,
	componentTypeFilter string,
	componentStackMap map[string]map[string][]string,
	importsConfig map[string]map[any]any,
	checkBaseComponentExists bool,
) (map[any]any, error) {

	stackName := strings.TrimSuffix(
		strings.TrimSuffix(
			u.TrimBasePathFromPath(stacksBasePath+"/", stack),
			cfg.DefaultStackConfigFileExtension),
		".yml",
	)

	globalVarsSection := map[any]any{}
	globalSettingsSection := map[any]any{}
	globalEnvSection := map[any]any{}
	globalTerraformSection := map[any]any{}
	globalHelmfileSection := map[any]any{}
	globalComponentsSection := map[any]any{}

	terraformVars := map[any]any{}
	terraformSettings := map[any]any{}
	terraformEnv := map[any]any{}
	terraformCommand := ""

	helmfileVars := map[any]any{}
	helmfileSettings := map[any]any{}
	helmfileEnv := map[any]any{}
	helmfileCommand := ""

	terraformComponents := map[string]any{}
	helmfileComponents := map[string]any{}
	allComponents := map[string]any{}

	// Global sections
	if i, ok := config["vars"]; ok {
		globalVarsSection, ok = i.(map[any]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'vars' section in the file '%s'", stackName)
		}
	}

	if i, ok := config["settings"]; ok {
		globalSettingsSection, ok = i.(map[any]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'settings' section in the file '%s'", stackName)
		}
	}

	if i, ok := config["env"]; ok {
		globalEnvSection, ok = i.(map[any]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'env' section in the file '%s'", stackName)
		}
	}

	if i, ok := config["terraform"]; ok {
		globalTerraformSection, ok = i.(map[any]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'terraform' section in the file '%s'", stackName)
		}
	}

	if i, ok := config["helmfile"]; ok {
		globalHelmfileSection, ok = i.(map[any]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'helmfile' section in the file '%s'", stackName)
		}
	}

	if i, ok := config["components"]; ok {
		globalComponentsSection, ok = i.(map[any]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'components' section in the file '%s'", stackName)
		}
	}

	// Terraform section
	if i, ok := globalTerraformSection["command"]; ok {
		terraformCommand, ok = i.(string)
		if !ok {
			return nil, fmt.Errorf("invalid 'terraform.command' section in the file '%s'", stackName)
		}
	}

	if i, ok := globalTerraformSection["vars"]; ok {
		terraformVars, ok = i.(map[any]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'terraform.vars' section in the file '%s'", stackName)
		}
	}

	globalAndTerraformVars, err := m.Merge([]map[any]any{globalVarsSection, terraformVars})
	if err != nil {
		return nil, err
	}

	if i, ok := globalTerraformSection["settings"]; ok {
		terraformSettings, ok = i.(map[any]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'terraform.settings' section in the file '%s'", stackName)
		}
	}

	globalAndTerraformSettings, err := m.Merge([]map[any]any{globalSettingsSection, terraformSettings})
	if err != nil {
		return nil, err
	}

	if i, ok := globalTerraformSection["env"]; ok {
		terraformEnv, ok = i.(map[any]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'terraform.env' section in the file '%s'", stackName)
		}
	}

	globalAndTerraformEnv, err := m.Merge([]map[any]any{globalEnvSection, terraformEnv})
	if err != nil {
		return nil, err
	}

	// Global backend
	globalBackendType := ""
	globalBackendSection := map[any]any{}

	if i, ok := globalTerraformSection["backend_type"]; ok {
		globalBackendType, ok = i.(string)
		if !ok {
			return nil, fmt.Errorf("invalid 'terraform.backend_type' section in the file '%s'", stackName)
		}
	}

	if i, ok := globalTerraformSection["backend"]; ok {
		globalBackendSection, ok = i.(map[any]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'terraform.backend' section in the file '%s'", stackName)
		}
	}

	// Global remote state backend
	globalRemoteStateBackendType := ""
	globalRemoteStateBackendSection := map[any]any{}

	if i, ok := globalTerraformSection["remote_state_backend_type"]; ok {
		globalRemoteStateBackendType, ok = i.(string)
		if !ok {
			return nil, fmt.Errorf("invalid 'terraform.remote_state_backend_type' section in the file '%s'", stackName)
		}
	}

	if i, ok := globalTerraformSection["remote_state_backend"]; ok {
		globalRemoteStateBackendSection, ok = i.(map[any]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'terraform.remote_state_backend' section in the file '%s'", stackName)
		}
	}

	// Helmfile section
	if i, ok := globalHelmfileSection["command"]; ok {
		helmfileCommand, ok = i.(string)
		if !ok {
			return nil, fmt.Errorf("invalid 'helmfile.command' section in the file '%s'", stackName)
		}
	}

	if i, ok := globalHelmfileSection["vars"]; ok {
		helmfileVars, ok = i.(map[any]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'helmfile.vars' section in the file '%s'", stackName)
		}
	}

	globalAndHelmfileVars, err := m.Merge([]map[any]any{globalVarsSection, helmfileVars})
	if err != nil {
		return nil, err
	}

	if i, ok := globalHelmfileSection["settings"]; ok {
		helmfileSettings, ok = i.(map[any]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'helmfile.settings' section in the file '%s'", stackName)
		}
	}

	globalAndHelmfileSettings, err := m.Merge([]map[any]any{globalSettingsSection, helmfileSettings})
	if err != nil {
		return nil, err
	}

	if i, ok := globalHelmfileSection["env"]; ok {
		helmfileEnv, ok = i.(map[any]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'helmfile.env' section in the file '%s'", stackName)
		}
	}

	globalAndHelmfileEnv, err := m.Merge([]map[any]any{globalEnvSection, helmfileEnv})
	if err != nil {
		return nil, err
	}

	// Process all Terraform components
	if componentTypeFilter == "" || componentTypeFilter == "terraform" {
		if allTerraformComponents, ok := globalComponentsSection["terraform"]; ok {

			allTerraformComponentsMap, ok := allTerraformComponents.(map[any]any)
			if !ok {
				return nil, fmt.Errorf("invalid 'components.terraform' section in the file '%s'", stackName)
			}

			for cmp, v := range allTerraformComponentsMap {
				component := cmp.(string)

				componentMap, ok := v.(map[any]any)
				if !ok {
					return nil, fmt.Errorf("invalid 'components.terraform.%s' section in the file '%s'", component, stackName)
				}

				componentVars := map[any]any{}
				if i, ok := componentMap["vars"]; ok {
					componentVars, ok = i.(map[any]any)
					if !ok {
						return nil, fmt.Errorf("invalid 'components.terraform.%s.vars' section in the file '%s'", component, stackName)
					}
				}

				componentSettings := map[any]any{}
				if i, ok := componentMap["settings"]; ok {
					componentSettings, ok = i.(map[any]any)
					if !ok {
						return nil, fmt.Errorf("invalid 'components.terraform.%s.settings' section in the file '%s'", component, stackName)
					}

					if i, ok := componentSettings["spacelift"]; ok {
						_, ok = i.(map[any]any)
						if !ok {
							return nil, fmt.Errorf("invalid 'components.terraform.%s.settings.spacelift' section in the file '%s'", component, stackName)
						}
					}
				}

				componentEnv := map[any]any{}
				if i, ok := componentMap["env"]; ok {
					componentEnv, ok = i.(map[any]any)
					if !ok {
						return nil, fmt.Errorf("invalid 'components.terraform.%s.env' section in the file '%s'", component, stackName)
					}
				}

				// Component metadata.
				// This is per component, not deep-merged and not inherited from base components and globals.
				componentMetadata := map[any]any{}
				if i, ok := componentMap["metadata"]; ok {
					componentMetadata, ok = i.(map[any]any)
					if !ok {
						return nil, fmt.Errorf("invalid 'components.terraform.%s.metadata' section in the file '%s'", component, stackName)
					}
				}

				// Component backend
				componentBackendType := ""
				componentBackendSection := map[any]any{}

				if i, ok := componentMap["backend_type"]; ok {
					componentBackendType, ok = i.(string)
					if !ok {
						return nil, fmt.Errorf("invalid 'components.terraform.%s.backend_type' attribute in the file '%s'", component, stackName)
					}
				}

				if i, ok := componentMap["backend"]; ok {
					componentBackendSection, ok = i.(map[any]any)
					if !ok {
						return nil, fmt.Errorf("invalid 'components.terraform.%s.backend' section in the file '%s'", component, stackName)
					}
				}

				// Component remote state backend
				componentRemoteStateBackendType := ""
				componentRemoteStateBackendSection := map[any]any{}

				if i, ok := componentMap["remote_state_backend_type"]; ok {
					componentRemoteStateBackendType, ok = i.(string)
					if !ok {
						return nil, fmt.Errorf("invalid 'components.terraform.%s.remote_state_backend_type' attribute in the file '%s'", component, stackName)
					}
				}

				if i, ok := componentMap["remote_state_backend"]; ok {
					componentRemoteStateBackendSection, ok = i.(map[any]any)
					if !ok {
						return nil, fmt.Errorf("invalid 'components.terraform.%s.remote_state_backend' section in the file '%s'", component, stackName)
					}
				}

				componentTerraformCommand := ""
				if i, ok := componentMap["command"]; ok {
					componentTerraformCommand, ok = i.(string)
					if !ok {
						return nil, fmt.Errorf("invalid 'components.terraform.%s.command' attribute in the file '%s'", component, stackName)
					}
				}

				// Process overrides
				componentOverrides := map[any]any{}
				componentOverridesVars := map[any]any{}
				componentOverridesSettings := map[any]any{}
				componentOverridesEnv := map[any]any{}
				componentOverridesTerraformCommand := ""

				if i, ok := componentMap["overrides"]; ok {
					if componentOverrides, ok = i.(map[any]any); !ok {
						return nil, fmt.Errorf("invalid 'components.terraform.%s.overrides' in the manifest '%s'", component, stackName)
					}

					if i, ok = componentOverrides["vars"]; ok {
						if componentOverridesVars, ok = i.(map[any]any); !ok {
							return nil, fmt.Errorf("invalid 'components.terraform.%s.overrides.vars' in the manifest '%s'", component, stackName)
						}
					}

					if i, ok = componentOverrides["settings"]; ok {
						if componentOverridesSettings, ok = i.(map[any]any); !ok {
							return nil, fmt.Errorf("invalid 'components.terraform.%s.overrides.settings' in the manifest '%s'", component, stackName)
						}
					}

					if i, ok = componentOverrides["env"]; ok {
						if componentOverridesEnv, ok = i.(map[any]any); !ok {
							return nil, fmt.Errorf("invalid 'components.terraform.%s.overrides.env' in the manifest '%s'", component, stackName)
						}
					}

					if i, ok = componentOverrides["command"]; ok {
						if componentOverridesTerraformCommand, ok = i.(string); !ok {
							return nil, fmt.Errorf("invalid 'components.terraform.%s.overrides.command' in the manifest '%s'", component, stackName)
						}
					}
				}

				// Process base component(s)
				baseComponentName := ""
				baseComponentVars := map[any]any{}
				baseComponentSettings := map[any]any{}
				baseComponentEnv := map[any]any{}
				baseComponentTerraformCommand := ""
				baseComponentBackendType := ""
				baseComponentBackendSection := map[any]any{}
				baseComponentRemoteStateBackendType := ""
				baseComponentRemoteStateBackendSection := map[any]any{}
				var baseComponentConfig schema.BaseComponentConfig
				var componentInheritanceChain []string
				var baseComponents []string

				// Inheritance using the top-level `component` attribute
				if baseComponent, baseComponentExist := componentMap["component"]; baseComponentExist {
					baseComponentName, ok = baseComponent.(string)
					if !ok {
						return nil, fmt.Errorf("invalid 'components.terraform.%s.component' attribute in the file '%s'", component, stackName)
					}

					// Process the base components recursively to find `componentInheritanceChain`
					err = ProcessBaseComponentConfig(
						&baseComponentConfig,
						allTerraformComponentsMap,
						component,
						stack,
						baseComponentName,
						terraformComponentsBasePath,
						checkBaseComponentExists,
						&baseComponents,
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
				// it does not specify inheritance (it overrides the deprecated top-level `component` attribute).
				// `metadata.inherit` is a list of component names from which the current component inherits.
				// It uses a method similar to Method Resolution Order (MRO), which is how Python supports multiple inheritance.
				//
				// In the case of multiple base components, it is processed left to right, in the order by which it was declared.
				// For example: `metadata.inherits: [componentA, componentB]`
				// will deep-merge all the base components of `componentA` (each component overriding its base),
				// then all the base components of `componentB` (each component overriding its base),
				// then the two results are deep-merged together (`componentB` inheritance chain will override values from 'componentA' inheritance chain).
				if baseComponentFromMetadata, baseComponentFromMetadataExist := componentMetadata["component"]; baseComponentFromMetadataExist {
					baseComponentName, ok = baseComponentFromMetadata.(string)
					if !ok {
						return nil, fmt.Errorf("invalid 'components.terraform.%s.metadata.component' attribute in the file '%s'", component, stackName)
					}
				}

				baseComponents = append(baseComponents, baseComponentName)

				if inheritList, inheritListExist := componentMetadata["inherits"].([]any); inheritListExist {
					for _, v := range inheritList {
						baseComponentFromInheritList, ok := v.(string)
						if !ok {
							return nil, fmt.Errorf("invalid 'components.terraform.%s.metadata.inherits' section in the file '%s'", component, stackName)
						}

						if _, ok := allTerraformComponentsMap[baseComponentFromInheritList]; !ok {
							if checkBaseComponentExists {
								errorMessage := fmt.Sprintf("The component '%[1]s' in the stack '%[2]s' inherits from '%[3]s' "+
									"(using 'metadata.inherits'), but '%[3]s' is not defined in any of the config files for the stack '%[2]s'",
									component,
									stackName,
									baseComponentFromInheritList,
								)
								return nil, errors.New(errorMessage)
							}
						}

						// Process the baseComponentFromInheritList components recursively to find `componentInheritanceChain`
						err = ProcessBaseComponentConfig(
							&baseComponentConfig,
							allTerraformComponentsMap,
							component,
							stack,
							baseComponentFromInheritList,
							terraformComponentsBasePath,
							checkBaseComponentExists,
							&baseComponents,
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

				baseComponents = u.UniqueStrings(baseComponents)
				sort.Strings(baseComponents)

				// Final configs
				finalComponentVars, err := m.Merge([]map[any]any{
					globalAndTerraformVars,
					baseComponentVars,
					componentVars,
					componentOverridesVars,
				})
				if err != nil {
					return nil, err
				}

				finalComponentSettings, err := m.Merge([]map[any]any{
					globalAndTerraformSettings,
					baseComponentSettings,
					componentSettings,
					componentOverridesSettings,
				})
				if err != nil {
					return nil, err
				}

				finalComponentEnv, err := m.Merge([]map[any]any{
					globalAndTerraformEnv,
					baseComponentEnv,
					componentEnv,
					componentOverridesEnv,
				})
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

				finalComponentBackendSection, err := m.Merge([]map[any]any{
					globalBackendSection,
					baseComponentBackendSection,
					componentBackendSection,
				})
				if err != nil {
					return nil, err
				}

				finalComponentBackend := map[any]any{}
				if i, ok := finalComponentBackendSection[finalComponentBackendType]; ok {
					finalComponentBackend, ok = i.(map[any]any)
					if !ok {
						return nil, fmt.Errorf("invalid 'terraform.backend' section for the component '%s'", component)
					}
				}

				// Check if `backend` section has `workspace_key_prefix` for `s3` backend type
				// If it does not, use the component name instead
				// It will also be propagated to `remote_state_backend` section of `s3` type
				if finalComponentBackendType == "s3" {
					if _, ok := finalComponentBackend["workspace_key_prefix"].(string); !ok {
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
					if componentAzurerm, componentAzurermExists := componentBackendSection["azurerm"].(map[any]any); !componentAzurermExists {
						if _, componentAzurermKeyExists := componentAzurerm["key"].(string); !componentAzurermKeyExists {
							azureKeyPrefixComponent := component
							baseKeyName := ""
							if baseComponentName != "" {
								azureKeyPrefixComponent = baseComponentName
							}
							if globalAzurerm, globalAzurermExists := globalBackendSection["azurerm"].(map[any]any); globalAzurermExists {
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

				finalComponentRemoteStateBackendSection, err := m.Merge([]map[any]any{
					globalRemoteStateBackendSection,
					baseComponentRemoteStateBackendSection,
					componentRemoteStateBackendSection,
				})
				if err != nil {
					return nil, err
				}

				// Merge `backend` and `remote_state_backend` sections
				// This will allow keeping `remote_state_backend` section DRY
				finalComponentRemoteStateBackendSectionMerged, err := m.Merge([]map[any]any{
					finalComponentBackendSection,
					finalComponentRemoteStateBackendSection,
				})
				if err != nil {
					return nil, err
				}

				finalComponentRemoteStateBackend := map[any]any{}
				if i, ok := finalComponentRemoteStateBackendSectionMerged[finalComponentRemoteStateBackendType]; ok {
					finalComponentRemoteStateBackend, ok = i.(map[any]any)
					if !ok {
						return nil, fmt.Errorf("invalid 'terraform.remote_state_backend' section for the component '%s'", component)
					}
				}

				// Final binary to execute
				finalComponentTerraformCommand := "terraform"
				if len(terraformCommand) > 0 {
					finalComponentTerraformCommand = terraformCommand
				}
				if len(baseComponentTerraformCommand) > 0 {
					finalComponentTerraformCommand = baseComponentTerraformCommand
				}
				if len(componentTerraformCommand) > 0 {
					finalComponentTerraformCommand = componentTerraformCommand
				}
				if len(componentOverridesTerraformCommand) > 0 {
					finalComponentTerraformCommand = componentOverridesTerraformCommand
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
				if componentIsAbstract {
					if i, ok := finalComponentSettings["spacelift"]; ok {
						spaceliftSettings, ok := i.(map[any]any)
						if !ok {
							return nil, fmt.Errorf("invalid 'components.terraform.%s.settings.spacelift' section in the file '%s'", component, stackName)
						}
						delete(spaceliftSettings, "workspace_enabled")
					}
				}

				comp := map[string]any{}
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
				comp["overrides"] = componentOverrides

				if baseComponentName != "" {
					comp["component"] = baseComponentName
				}

				terraformComponents[component] = comp
			}
		}
	}

	// Process all helmfile components
	if componentTypeFilter == "" || componentTypeFilter == "helmfile" {
		if allHelmfileComponents, ok := globalComponentsSection["helmfile"]; ok {

			allHelmfileComponentsMap, ok := allHelmfileComponents.(map[any]any)
			if !ok {
				return nil, fmt.Errorf("invalid 'components.helmfile' section in the file '%s'", stackName)
			}

			for cmp, v := range allHelmfileComponentsMap {
				component := cmp.(string)

				componentMap, ok := v.(map[any]any)
				if !ok {
					return nil, fmt.Errorf("invalid 'components.helmfile.%s' section in the file '%s'", component, stackName)
				}

				componentVars := map[any]any{}
				if i2, ok := componentMap["vars"]; ok {
					componentVars, ok = i2.(map[any]any)
					if !ok {
						return nil, fmt.Errorf("invalid 'components.helmfile.%s.vars' section in the file '%s'", component, stackName)
					}
				}

				componentSettings := map[any]any{}
				if i, ok := componentMap["settings"]; ok {
					componentSettings, ok = i.(map[any]any)
					if !ok {
						return nil, fmt.Errorf("invalid 'components.helmfile.%s.settings' section in the file '%s'", component, stackName)
					}
				}

				componentEnv := map[any]any{}
				if i, ok := componentMap["env"]; ok {
					componentEnv, ok = i.(map[any]any)
					if !ok {
						return nil, fmt.Errorf("invalid 'components.helmfile.%s.env' section in the file '%s'", component, stackName)
					}
				}

				// Component metadata.
				// This is per component, not deep-merged and not inherited from base components and globals.
				componentMetadata := map[any]any{}
				if i, ok := componentMap["metadata"]; ok {
					componentMetadata, ok = i.(map[any]any)
					if !ok {
						return nil, fmt.Errorf("invalid 'components.helmfile.%s.metadata' section in the file '%s'", component, stackName)
					}
				}

				componentHelmfileCommand := ""
				if i, ok := componentMap["command"]; ok {
					componentHelmfileCommand, ok = i.(string)
					if !ok {
						return nil, fmt.Errorf("invalid 'components.helmfile.%s.command' attribute in the file '%s'", component, stackName)
					}
				}

				// Process overrides
				componentOverrides := map[any]any{}
				componentOverridesVars := map[any]any{}
				componentOverridesSettings := map[any]any{}
				componentOverridesEnv := map[any]any{}
				componentOverridesHelmfileCommand := ""

				if i, ok := componentMap["overrides"]; ok {
					if componentOverrides, ok = i.(map[any]any); !ok {
						return nil, fmt.Errorf("invalid 'components.helmfile.%s.overrides' in the manifest '%s'", component, stackName)
					}

					if i, ok = componentOverrides["vars"]; ok {
						if componentOverridesVars, ok = i.(map[any]any); !ok {
							return nil, fmt.Errorf("invalid 'components.helmfile.%s.overrides.vars' in the manifest '%s'", component, stackName)
						}
					}

					if i, ok = componentOverrides["settings"]; ok {
						if componentOverridesSettings, ok = i.(map[any]any); !ok {
							return nil, fmt.Errorf("invalid 'components.helmfile.%s.overrides.settings' in the manifest '%s'", component, stackName)
						}
					}

					if i, ok = componentOverrides["env"]; ok {
						if componentOverridesEnv, ok = i.(map[any]any); !ok {
							return nil, fmt.Errorf("invalid 'components.helmfile.%s.overrides.env' in the manifest '%s'", component, stackName)
						}
					}

					if i, ok = componentOverrides["command"]; ok {
						if componentOverridesHelmfileCommand, ok = i.(string); !ok {
							return nil, fmt.Errorf("invalid 'components.helmfile.%s.overrides.command' in the manifest '%s'", component, stackName)
						}
					}
				}

				// Process base component(s)
				baseComponentVars := map[any]any{}
				baseComponentSettings := map[any]any{}
				baseComponentEnv := map[any]any{}
				baseComponentName := ""
				baseComponentHelmfileCommand := ""
				var baseComponentConfig schema.BaseComponentConfig
				var componentInheritanceChain []string
				var baseComponents []string

				// Inheritance using the top-level `component` attribute
				if baseComponent, baseComponentExist := componentMap["component"]; baseComponentExist {
					baseComponentName, ok = baseComponent.(string)
					if !ok {
						return nil, fmt.Errorf("invalid 'components.helmfile.%s.component' attribute in the file '%s'", component, stackName)
					}

					// Process the base components recursively to find `componentInheritanceChain`
					err = ProcessBaseComponentConfig(
						&baseComponentConfig,
						allHelmfileComponentsMap,
						component,
						stack,
						baseComponentName,
						helmfileComponentsBasePath,
						checkBaseComponentExists,
						&baseComponents,
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
				// it does not specify inheritance (it overrides the deprecated top-level `component` attribute).
				// `metadata.inherit` is a list of component names from which the current component inherits.
				// It uses a method similar to Method Resolution Order (MRO), which is how Python supports multiple inheritance.
				//
				// In the case of multiple base components, it is processed left to right, in the order by which it was declared.
				// For example: `metadata.inherits: [componentA, componentB]`
				// will deep-merge all the base components of `componentA` (each component overriding its base),
				// then all the base components of `componentB` (each component overriding its base),
				// then the two results are deep-merged together (`componentB` inheritance chain will override values from 'componentA' inheritance chain).
				if baseComponentFromMetadata, baseComponentFromMetadataExist := componentMetadata["component"]; baseComponentFromMetadataExist {
					baseComponentName, ok = baseComponentFromMetadata.(string)
					if !ok {
						return nil, fmt.Errorf("invalid 'components.helmfile.%s.metadata.component' attribute in the file '%s'", component, stackName)
					}
				}

				baseComponents = append(baseComponents, baseComponentName)

				if inheritList, inheritListExist := componentMetadata["inherits"].([]any); inheritListExist {
					for _, v := range inheritList {
						baseComponentFromInheritList, ok := v.(string)
						if !ok {
							return nil, fmt.Errorf("invalid 'components.helmfile.%s.metadata.inherits' section in the file '%s'", component, stackName)
						}

						if _, ok := allHelmfileComponentsMap[baseComponentFromInheritList]; !ok {
							if checkBaseComponentExists {
								errorMessage := fmt.Sprintf("The component '%[1]s' in the stack '%[2]s' inherits from '%[3]s' "+
									"(using 'metadata.inherits'), but '%[3]s' is not defined in any of the config files for the stack '%[2]s'",
									component,
									stackName,
									baseComponentFromInheritList,
								)
								return nil, errors.New(errorMessage)
							}
						}

						// Process the baseComponentFromInheritList components recursively to find `componentInheritanceChain`
						err = ProcessBaseComponentConfig(
							&baseComponentConfig,
							allHelmfileComponentsMap,
							component,
							stack,
							baseComponentFromInheritList,
							helmfileComponentsBasePath,
							checkBaseComponentExists,
							&baseComponents,
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

				baseComponents = u.UniqueStrings(baseComponents)
				sort.Strings(baseComponents)

				// Final configs
				finalComponentVars, err := m.Merge([]map[any]any{
					globalAndHelmfileVars,
					baseComponentVars,
					componentVars,
					componentOverridesVars,
				})
				if err != nil {
					return nil, err
				}

				finalComponentSettings, err := m.Merge([]map[any]any{
					globalAndHelmfileSettings,
					baseComponentSettings,
					componentSettings,
					componentOverridesSettings,
				})
				if err != nil {
					return nil, err
				}

				finalComponentEnv, err := m.Merge([]map[any]any{
					globalAndHelmfileEnv,
					baseComponentEnv,
					componentEnv,
					componentOverridesEnv,
				})
				if err != nil {
					return nil, err
				}

				// Final binary to execute
				finalComponentHelmfileCommand := "helmfile"
				if len(helmfileCommand) > 0 {
					finalComponentHelmfileCommand = helmfileCommand
				}
				if len(baseComponentHelmfileCommand) > 0 {
					finalComponentHelmfileCommand = baseComponentHelmfileCommand
				}
				if len(componentHelmfileCommand) > 0 {
					finalComponentHelmfileCommand = componentHelmfileCommand
				}
				if len(componentOverridesHelmfileCommand) > 0 {
					finalComponentHelmfileCommand = componentOverridesHelmfileCommand
				}

				comp := map[string]any{}
				comp["vars"] = finalComponentVars
				comp["settings"] = finalComponentSettings
				comp["env"] = finalComponentEnv
				comp["command"] = finalComponentHelmfileCommand
				comp["inheritance"] = componentInheritanceChain
				comp["metadata"] = componentMetadata
				comp["overrides"] = componentOverrides

				if baseComponentName != "" {
					comp["component"] = baseComponentName
				}

				helmfileComponents[component] = comp
			}
		}
	}

	allComponents["terraform"] = terraformComponents
	allComponents["helmfile"] = helmfileComponents

	result := map[any]any{
		"components": allComponents,
	}

	return result, nil
}
