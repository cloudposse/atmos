package exec

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/santhosh-tekuri/jsonschema/v5"

	cfg "github.com/cloudposse/atmos/pkg/config"
	m "github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

var (
	// Error constants.
	ErrInvalidHooksSection          = errors.New("invalid 'hooks' section in the file")
	ErrInvalidTerraformHooksSection = errors.New("invalid 'terraform.hooks' section in the file")

	// File content sync map.
	getFileContentSyncMap = sync.Map{}

	// Mutex to serialize updates of the result map of ProcessYAMLConfigFiles function.
	processYAMLConfigFilesLock = &sync.Mutex{}
)

// ProcessYAMLConfigFiles takes a list of paths to stack manifests, processes and deep-merges all imports,
// and returns a list of stack configs
func ProcessYAMLConfigFiles(
	atmosConfig schema.AtmosConfiguration,
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
				stackBasePath = filepath.Dir(p)
			}

			stackFileName := strings.TrimSuffix(
				strings.TrimSuffix(
					u.TrimBasePathFromPath(stackBasePath+"/", p),
					u.DefaultStackConfigFileExtension),
				".yml",
			)

			deepMergedStackConfig, importsConfig, stackConfig, _, _, _, _, err := ProcessYAMLConfigFile(
				atmosConfig,
				stackBasePath,
				p,
				map[string]map[string]any{},
				nil,
				ignoreMissingFiles,
				false,
				false,
				false,
				map[string]any{},
				map[string]any{},
				map[string]any{},
				map[string]any{},
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
				atmosConfig,
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

			yamlConfig, err := u.ConvertToYAML(finalConfig)
			if err != nil {
				errorResult = err
				return
			}

			processYAMLConfigFilesLock.Lock()
			defer processYAMLConfigFilesLock.Unlock()

			listResult[i] = yamlConfig
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
	atmosConfig schema.AtmosConfiguration,
	basePath string,
	filePath string,
	importsConfig map[string]map[string]any,
	context map[string]any,
	ignoreMissingFiles bool,
	skipTemplatesProcessingInImports bool,
	ignoreMissingTemplateValues bool,
	skipIfMissing bool,
	parentTerraformOverridesInline map[string]any,
	parentTerraformOverridesImports map[string]any,
	parentHelmfileOverridesInline map[string]any,
	parentHelmfileOverridesImports map[string]any,
	atmosManifestJsonSchemaFilePath string,
) (
	map[string]any,
	map[string]map[string]any,
	map[string]any,
	map[string]any,
	map[string]any,
	map[string]any,
	map[string]any,
	error,
) {
	var stackConfigs []map[string]any
	relativeFilePath := u.TrimBasePathFromPath(basePath+"/", filePath)

	globalTerraformSection := map[string]any{}
	globalHelmfileSection := map[string]any{}
	globalOverrides := map[string]any{}
	terraformOverrides := map[string]any{}
	helmfileOverrides := map[string]any{}
	finalTerraformOverrides := map[string]any{}
	finalHelmfileOverrides := map[string]any{}

	stackYamlConfig, err := GetFileContent(filePath)
	// If the file does not exist (`err != nil`), and `ignoreMissingFiles = true`, don't return the error.
	//
	// `ignoreMissingFiles = true` is used when executing `atmos describe affected` command.
	// If we add a new stack manifest with some component configurations to the current branch, then the new file will not be present in
	// the remote branch (with which the current branch is compared), and Atmos would throw an error.
	//
	// `skipIfMissing` is used in Atmos imports (https://atmos.tools/core-concepts/stacks/imports).
	// Set it to `true` to ignore the imported manifest if it does not exist, and don't throw an error.
	// This is useful when generating Atmos manifests using other tools, but the imported files are not present yet at the generation time.
	if err != nil {
		if ignoreMissingFiles || skipIfMissing {
			return map[string]any{}, map[string]map[string]any{}, map[string]any{}, map[string]any{}, map[string]any{}, map[string]any{}, map[string]any{}, nil
		} else {
			return nil, nil, nil, nil, nil, nil, nil, err
		}
	}
	if stackYamlConfig == "" {
		return map[string]any{}, map[string]map[string]any{}, map[string]any{}, map[string]any{}, map[string]any{}, map[string]any{}, map[string]any{}, nil
	}

	stackManifestTemplatesProcessed := stackYamlConfig
	stackManifestTemplatesErrorMessage := ""

	// Process `Go` templates in the imported stack manifest using the provided `context`
	// https://atmos.tools/core-concepts/stacks/imports#go-templates-in-imports
	if !skipTemplatesProcessingInImports && len(context) > 0 {
		stackManifestTemplatesProcessed, err = ProcessTmpl(relativeFilePath, stackYamlConfig, context, ignoreMissingTemplateValues)
		if err != nil {
			if atmosConfig.Logs.Level == u.LogLevelTrace || atmosConfig.Logs.Level == u.LogLevelDebug {
				stackManifestTemplatesErrorMessage = fmt.Sprintf("\n\n%s", stackYamlConfig)
			}
			e := fmt.Errorf("invalid stack manifest '%s'\n%v%s", relativeFilePath, err, stackManifestTemplatesErrorMessage)
			return nil, nil, nil, nil, nil, nil, nil, e
		}
	}

	stackConfigMap, err := u.UnmarshalYAMLFromFile[schema.AtmosSectionMapType](&atmosConfig, stackManifestTemplatesProcessed, filePath)
	if err != nil {
		if atmosConfig.Logs.Level == u.LogLevelTrace || atmosConfig.Logs.Level == u.LogLevelDebug {
			stackManifestTemplatesErrorMessage = fmt.Sprintf("\n\n%s", stackYamlConfig)
		}
		e := fmt.Errorf("invalid stack manifest '%s'\n%v%s", relativeFilePath, err, stackManifestTemplatesErrorMessage)
		return nil, nil, nil, nil, nil, nil, nil, e
	}

	// If the path to the Atmos manifest JSON Schema is provided, validate the stack manifest against it
	if atmosManifestJsonSchemaFilePath != "" {
		// Convert the data to JSON and back to Go map to prevent the error:
		// jsonschema: invalid jsonType: map[interface {}]interface {}
		dataJson, err := u.ConvertToJSONFast(stackConfigMap)
		if err != nil {
			return nil, nil, nil, nil, nil, nil, nil, err
		}

		dataFromJson, err := u.ConvertFromJSON(dataJson)
		if err != nil {
			return nil, nil, nil, nil, nil, nil, nil, err
		}

		compiler := jsonschema.NewCompiler()

		atmosManifestJsonSchemaValidationErrorFormat := "Atmos manifest JSON Schema validation error in the file '%s':\n%v"

		atmosManifestJsonSchemaFileReader, err := os.Open(atmosManifestJsonSchemaFilePath)
		if err != nil {
			return nil, nil, nil, nil, nil, nil, nil, errors.Errorf(atmosManifestJsonSchemaValidationErrorFormat, relativeFilePath, err)
		}

		if err := compiler.AddResource(atmosManifestJsonSchemaFilePath, atmosManifestJsonSchemaFileReader); err != nil {
			return nil, nil, nil, nil, nil, nil, nil, errors.Errorf(atmosManifestJsonSchemaValidationErrorFormat, relativeFilePath, err)
		}

		compiler.Draft = jsonschema.Draft2020

		compiledSchema, err := compiler.Compile(atmosManifestJsonSchemaFilePath)
		if err != nil {
			return nil, nil, nil, nil, nil, nil, nil, errors.Errorf(atmosManifestJsonSchemaValidationErrorFormat, relativeFilePath, err)
		}

		if err = compiledSchema.Validate(dataFromJson); err != nil {
			switch e := err.(type) {
			case *jsonschema.ValidationError:
				b, err2 := json.MarshalIndent(e.BasicOutput(), "", "  ")
				if err2 != nil {
					return nil, nil, nil, nil, nil, nil, nil, errors.Errorf(atmosManifestJsonSchemaValidationErrorFormat, relativeFilePath, err2)
				}
				return nil, nil, nil, nil, nil, nil, nil, errors.Errorf(atmosManifestJsonSchemaValidationErrorFormat, relativeFilePath, string(b))
			default:
				return nil, nil, nil, nil, nil, nil, nil, errors.Errorf(atmosManifestJsonSchemaValidationErrorFormat, relativeFilePath, err)
			}
		}
	}

	// Check if the `overrides` sections exist and if we need to process overrides for the components in this stack manifest and its imports

	// Global overrides in this stack manifest
	if i, ok := stackConfigMap[cfg.OverridesSectionName]; ok {
		if globalOverrides, ok = i.(map[string]any); !ok {
			return nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("invalid 'overrides' section in the stack manifest '%s'", relativeFilePath)
		}
	}

	// Terraform overrides in this stack manifest
	if o, ok := stackConfigMap[cfg.TerraformSectionName]; ok {
		if globalTerraformSection, ok = o.(map[string]any); !ok {
			return nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("invalid 'terraform' section in the stack manifest '%s'", relativeFilePath)
		}

		if i, ok := globalTerraformSection[cfg.OverridesSectionName]; ok {
			if terraformOverrides, ok = i.(map[string]any); !ok {
				return nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("invalid 'terraform.overrides' section in the stack manifest '%s'", relativeFilePath)
			}
		}
	}

	// Helmfile overrides in this stack manifest
	if o, ok := stackConfigMap[cfg.HelmfileSectionName]; ok {
		if globalHelmfileSection, ok = o.(map[string]any); !ok {
			return nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("invalid 'helmfile' section in the stack manifest '%s'", relativeFilePath)
		}

		if i, ok := globalHelmfileSection[cfg.OverridesSectionName]; ok {
			if helmfileOverrides, ok = i.(map[string]any); !ok {
				return nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("invalid 'terraform.overrides' section in the stack manifest '%s'", relativeFilePath)
			}
		}
	}

	parentTerraformOverridesInline, err = m.Merge(
		atmosConfig,
		[]map[string]any{globalOverrides, terraformOverrides, parentTerraformOverridesInline},
	)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, err
	}

	parentHelmfileOverridesInline, err = m.Merge(
		atmosConfig,
		[]map[string]any{globalOverrides, helmfileOverrides, parentHelmfileOverridesInline},
	)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, err
	}

	// Find and process all imports
	importStructs, err := ProcessImportSection(stackConfigMap, relativeFilePath)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, err
	}

	for _, importStruct := range importStructs {
		imp := importStruct.Path

		if imp == "" {
			return nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("invalid empty import in the manifest '%s'", relativeFilePath)
		}

		// If the import file is specified without extension, use `.yaml` as default
		impWithExt := imp
		ext := filepath.Ext(imp)
		if ext == "" {
			extensions := []string{
				u.YamlFileExtension,
				u.YmlFileExtension,
				u.YamlTemplateExtension,
				u.YmlTemplateExtension,
			}

			found := false
			for _, extension := range extensions {
				testPath := filepath.Join(basePath, imp+extension)
				if _, err := os.Stat(testPath); err == nil {
					impWithExt = imp + extension
					found = true
					break
				}
			}

			if !found {
				// Default to .yaml if no file is found
				impWithExt = imp + u.DefaultStackConfigFileExtension
			}
		} else if ext == u.YamlFileExtension || ext == u.YmlFileExtension {
			// Check if there's a template version of this file
			templatePath := impWithExt + u.TemplateExtension
			if _, err := os.Stat(filepath.Join(basePath, templatePath)); err == nil {
				impWithExt = templatePath
			}
		}

		impWithExtPath := filepath.Join(basePath, impWithExt)

		if impWithExtPath == filePath {
			errorMessage := fmt.Sprintf("invalid import in the manifest '%s'\nThe file imports itself in '%s'",
				relativeFilePath,
				imp)
			return nil, nil, nil, nil, nil, nil, nil, errors.New(errorMessage)
		}

		// Find all import matches in the glob
		importMatches, err := u.GetGlobMatches(impWithExtPath)
		if err != nil || len(importMatches) == 0 {
			// Retry (b/c we are using `doublestar` library and it sometimes has issues reading many files in a Docker container)
			// TODO: review `doublestar` library

			importMatches, err = u.GetGlobMatches(impWithExtPath)
			if err != nil || len(importMatches) == 0 {
				// The import was not found -> check if the import is a Go template; if not, return the error
				isGolangTemplate, err2 := IsGolangTemplate(imp)
				if err2 != nil {
					return nil, nil, nil, nil, nil, nil, nil, err2
				}

				// If the import is not a Go template and SkipIfMissing is false, return the error
				if !isGolangTemplate && !importStruct.SkipIfMissing {
					if err != nil {
						errorMessage := fmt.Sprintf("no matches found for the import '%s' in the file '%s'\nError: %s",
							imp,
							relativeFilePath,
							err,
						)
						return nil, nil, nil, nil, nil, nil, nil, errors.New(errorMessage)
					} else if importMatches == nil {
						errorMessage := fmt.Sprintf("no matches found for the import '%s' in the file '%s'",
							imp,
							relativeFilePath,
						)
						return nil, nil, nil, nil, nil, nil, nil, errors.New(errorMessage)
					}
				}
			}
		}

		// Process `context` in hierarchical imports.
		// Deep-merge the parent `context` with the current `context` and propagate the result to the entire chain of imports.
		// The parent `context` takes precedence over the current (imported) `context` and will override items with the same keys.
		// TODO: instead of calling the conversion functions, we need to switch to generics and update everything to support it
		listOfMaps := []map[string]any{importStruct.Context, context}
		mergedContext, err := m.Merge(atmosConfig, listOfMaps)
		if err != nil {
			return nil, nil, nil, nil, nil, nil, nil, err
		}

		// Process the imports in the current manifest
		for _, importFile := range importMatches {
			yamlConfig, _, yamlConfigRaw, _, terraformOverridesImports, _, helmfileOverridesImports, err2 := ProcessYAMLConfigFile(
				atmosConfig,
				basePath,
				importFile,
				importsConfig,
				mergedContext,
				ignoreMissingFiles,
				importStruct.SkipTemplatesProcessing,
				true, // importStruct.IgnoreMissingTemplateValues,
				importStruct.SkipIfMissing,
				parentTerraformOverridesInline,
				parentTerraformOverridesImports,
				parentHelmfileOverridesInline,
				parentHelmfileOverridesImports,
				"",
			)
			if err2 != nil {
				return nil, nil, nil, nil, nil, nil, nil, err2
			}

			parentTerraformOverridesImports, err = m.Merge(
				atmosConfig,
				[]map[string]any{parentTerraformOverridesImports, terraformOverridesImports},
			)
			if err != nil {
				return nil, nil, nil, nil, nil, nil, nil, err
			}

			parentHelmfileOverridesImports, err = m.Merge(
				atmosConfig,
				[]map[string]any{parentHelmfileOverridesImports, helmfileOverridesImports},
			)
			if err != nil {
				return nil, nil, nil, nil, nil, nil, nil, err
			}

			stackConfigs = append(stackConfigs, yamlConfig)

			importRelativePathWithExt := strings.Replace(filepath.ToSlash(importFile), filepath.ToSlash(basePath)+"/", "", 1)
			ext2 := filepath.Ext(importRelativePathWithExt)
			if ext2 == "" {
				ext2 = u.DefaultStackConfigFileExtension
			}

			importRelativePathWithoutExt := strings.TrimSuffix(importRelativePathWithExt, ext2)
			importsConfig[importRelativePathWithoutExt] = yamlConfigRaw
		}
	}

	// Terraform `overrides`
	finalTerraformOverrides, err = m.Merge(
		atmosConfig,
		[]map[string]any{parentTerraformOverridesImports, parentTerraformOverridesInline},
	)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, err
	}

	// Helmfile `overrides`
	finalHelmfileOverrides, err = m.Merge(
		atmosConfig,
		[]map[string]any{parentHelmfileOverridesImports, parentHelmfileOverridesInline},
	)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, err
	}

	// Add the `overrides` section to all components in this stack manifest
	if len(finalTerraformOverrides) > 0 || len(finalHelmfileOverrides) > 0 {
		if componentsSection, ok := stackConfigMap[cfg.ComponentsSectionName].(map[string]any); ok {
			// Terraform
			if len(finalTerraformOverrides) > 0 {
				if terraformSection, ok := componentsSection[cfg.TerraformSectionName].(map[string]any); ok {
					for _, compSection := range terraformSection {
						if componentSection, ok := compSection.(map[string]any); ok {
							componentSection[cfg.OverridesSectionName] = finalTerraformOverrides
						}
					}
				}
			}

			// Helmfile
			if len(finalHelmfileOverrides) > 0 {
				if helmfileSection, ok := componentsSection[cfg.HelmfileSectionName].(map[string]any); ok {
					for _, compSection := range helmfileSection {
						if componentSection, ok := compSection.(map[string]any); ok {
							componentSection[cfg.OverridesSectionName] = finalHelmfileOverrides
						}
					}
				}
			}
		}
	}

	if len(stackConfigMap) > 0 {
		stackConfigs = append(stackConfigs, stackConfigMap)
	}

	// Deep-merge the stack manifest and all the imports
	stackConfigsDeepMerged, err := m.Merge(atmosConfig, stackConfigs)
	if err != nil {
		err2 := fmt.Errorf("ProcessYAMLConfigFile: Merge: Deep-merge the stack manifest and all the imports: Error: %v", err)
		return nil, nil, nil, nil, nil, nil, nil, err2
	}

	return stackConfigsDeepMerged, importsConfig, stackConfigMap,
		parentTerraformOverridesInline, parentTerraformOverridesImports,
		parentHelmfileOverridesInline, parentHelmfileOverridesImports,
		nil
}

// ProcessStackConfig takes a stack manifest, deep-merges all variables, settings, environments and backends,
// and returns the final stack configuration for all Terraform and helmfile components
func ProcessStackConfig(
	atmosConfig schema.AtmosConfiguration,
	stacksBasePath string,
	terraformComponentsBasePath string,
	helmfileComponentsBasePath string,
	stack string,
	config map[string]any,
	processStackDeps bool,
	processComponentDeps bool,
	componentTypeFilter string,
	componentStackMap map[string]map[string][]string,
	importsConfig map[string]map[string]any,
	checkBaseComponentExists bool,
) (map[string]any, error) {
	stackName := strings.TrimSuffix(
		strings.TrimSuffix(
			u.TrimBasePathFromPath(stacksBasePath+"/", stack),
			u.DefaultStackConfigFileExtension),
		".yml",
	)

	globalVarsSection := map[string]any{}
	globalHooksSection := map[string]any{}
	globalSettingsSection := map[string]any{}
	globalEnvSection := map[string]any{}
	globalTerraformSection := map[string]any{}
	globalHelmfileSection := map[string]any{}
	globalComponentsSection := map[string]any{}

	terraformVars := map[string]any{}
	terraformSettings := map[string]any{}
	terraformEnv := map[string]any{}
	terraformCommand := ""
	terraformProviders := map[string]any{}
	terraformHooks := map[string]any{}

	helmfileVars := map[string]any{}
	helmfileSettings := map[string]any{}
	helmfileEnv := map[string]any{}
	helmfileCommand := ""

	terraformComponents := map[string]any{}
	helmfileComponents := map[string]any{}
	allComponents := map[string]any{}

	// Global sections
	if i, ok := config["vars"]; ok {
		globalVarsSection, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'vars' section in the file '%s'", stackName)
		}
	}

	if i, ok := config["hooks"]; ok {
		globalHooksSection, ok = i.(map[string]any)
		if !ok {
			return nil, errors.Wrapf(ErrInvalidHooksSection, " '%s'", stackName)
		}
	}

	if i, ok := config["settings"]; ok {
		globalSettingsSection, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'settings' section in the file '%s'", stackName)
		}
	}

	if i, ok := config["env"]; ok {
		globalEnvSection, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'env' section in the file '%s'", stackName)
		}
	}

	if i, ok := config["terraform"]; ok {
		globalTerraformSection, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'terraform' section in the file '%s'", stackName)
		}
	}

	if i, ok := config["helmfile"]; ok {
		globalHelmfileSection, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'helmfile' section in the file '%s'", stackName)
		}
	}

	if i, ok := config["components"]; ok {
		globalComponentsSection, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'components' section in the file '%s'", stackName)
		}
	}

	// Terraform section
	if i, ok := globalTerraformSection[cfg.CommandSectionName]; ok {
		terraformCommand, ok = i.(string)
		if !ok {
			return nil, fmt.Errorf("invalid 'terraform.command' section in the file '%s'", stackName)
		}
	}

	if i, ok := globalTerraformSection["vars"]; ok {
		terraformVars, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'terraform.vars' section in the file '%s'", stackName)
		}
	}

	if i, ok := globalTerraformSection["hooks"]; ok {
		terraformHooks, ok = i.(map[string]any)
		if !ok {
			return nil, errors.Wrapf(ErrInvalidTerraformHooksSection, "in file '%s'", stackName)
		}
	}

	globalAndTerraformVars, err := m.Merge(atmosConfig, []map[string]any{globalVarsSection, terraformVars})
	if err != nil {
		return nil, err
	}

	globalAndTerraformHooks, err := m.Merge(atmosConfig, []map[string]any{globalHooksSection, terraformHooks})
	if err != nil {
		return nil, err
	}

	if i, ok := globalTerraformSection["settings"]; ok {
		terraformSettings, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'terraform.settings' section in the file '%s'", stackName)
		}
	}

	globalAndTerraformSettings, err := m.Merge(atmosConfig, []map[string]any{globalSettingsSection, terraformSettings})
	if err != nil {
		return nil, err
	}

	if i, ok := globalTerraformSection["env"]; ok {
		terraformEnv, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'terraform.env' section in the file '%s'", stackName)
		}
	}

	globalAndTerraformEnv, err := m.Merge(atmosConfig, []map[string]any{globalEnvSection, terraformEnv})
	if err != nil {
		return nil, err
	}

	if i, ok := globalTerraformSection[cfg.ProvidersSectionName]; ok {
		terraformProviders, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'terraform.providers' section in the file '%s'", stackName)
		}
	}

	if i, ok := globalTerraformSection[cfg.HooksSectionName]; ok {
		terraformHooks, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'terraform.hooks' section in the file '%s'", stackName)
		}
	}

	// Global backend
	globalBackendType := ""
	globalBackendSection := map[string]any{}

	if i, ok := globalTerraformSection["backend_type"]; ok {
		globalBackendType, ok = i.(string)
		if !ok {
			return nil, fmt.Errorf("invalid 'terraform.backend_type' section in the file '%s'", stackName)
		}
	}

	if i, ok := globalTerraformSection["backend"]; ok {
		globalBackendSection, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'terraform.backend' section in the file '%s'", stackName)
		}
	}

	// Global remote state backend
	globalRemoteStateBackendType := ""
	globalRemoteStateBackendSection := map[string]any{}

	if i, ok := globalTerraformSection["remote_state_backend_type"]; ok {
		globalRemoteStateBackendType, ok = i.(string)
		if !ok {
			return nil, fmt.Errorf("invalid 'terraform.remote_state_backend_type' section in the file '%s'", stackName)
		}
	}

	if i, ok := globalTerraformSection["remote_state_backend"]; ok {
		globalRemoteStateBackendSection, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'terraform.remote_state_backend' section in the file '%s'", stackName)
		}
	}

	// Helmfile section
	if i, ok := globalHelmfileSection[cfg.CommandSectionName]; ok {
		helmfileCommand, ok = i.(string)
		if !ok {
			return nil, fmt.Errorf("invalid 'helmfile.command' section in the file '%s'", stackName)
		}
	}

	if i, ok := globalHelmfileSection["vars"]; ok {
		helmfileVars, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'helmfile.vars' section in the file '%s'", stackName)
		}
	}

	globalAndHelmfileVars, err := m.Merge(atmosConfig, []map[string]any{globalVarsSection, helmfileVars})
	if err != nil {
		return nil, err
	}

	if i, ok := globalHelmfileSection["settings"]; ok {
		helmfileSettings, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'helmfile.settings' section in the file '%s'", stackName)
		}
	}

	globalAndHelmfileSettings, err := m.Merge(atmosConfig, []map[string]any{globalSettingsSection, helmfileSettings})
	if err != nil {
		return nil, err
	}

	if i, ok := globalHelmfileSection["env"]; ok {
		helmfileEnv, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'helmfile.env' section in the file '%s'", stackName)
		}
	}

	globalAndHelmfileEnv, err := m.Merge(atmosConfig, []map[string]any{globalEnvSection, helmfileEnv})
	if err != nil {
		return nil, err
	}

	// Process all Terraform components
	if componentTypeFilter == "" || componentTypeFilter == "terraform" {
		if allTerraformComponents, ok := globalComponentsSection["terraform"]; ok {

			allTerraformComponentsMap, ok := allTerraformComponents.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid 'components.terraform' section in the file '%s'", stackName)
			}

			for cmp, v := range allTerraformComponentsMap {
				component := cmp

				componentMap, ok := v.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("invalid 'components.terraform.%s' section in the file '%s'", component, stackName)
				}

				componentVars := map[string]any{}
				if i, ok := componentMap[cfg.VarsSectionName]; ok {
					componentVars, ok = i.(map[string]any)
					if !ok {
						return nil, fmt.Errorf("invalid 'components.terraform.%s.vars' section in the file '%s'", component, stackName)
					}
				}

				componentSettings := map[string]any{}
				if i, ok := componentMap[cfg.SettingsSectionName]; ok {
					componentSettings, ok = i.(map[string]any)
					if !ok {
						return nil, fmt.Errorf("invalid 'components.terraform.%s.settings' section in the file '%s'", component, stackName)
					}

					if i, ok := componentSettings["spacelift"]; ok {
						_, ok = i.(map[string]any)
						if !ok {
							return nil, fmt.Errorf("invalid 'components.terraform.%s.settings.spacelift' section in the file '%s'", component, stackName)
						}
					}
				}

				componentEnv := map[string]any{}
				if i, ok := componentMap[cfg.EnvSectionName]; ok {
					componentEnv, ok = i.(map[string]any)
					if !ok {
						return nil, fmt.Errorf("invalid 'components.terraform.%s.env' section in the file '%s'", component, stackName)
					}
				}

				componentProviders := map[string]any{}
				if i, ok := componentMap[cfg.ProvidersSectionName]; ok {
					componentProviders, ok = i.(map[string]any)
					if !ok {
						return nil, fmt.Errorf("invalid 'components.terraform.%s.providers' section in the file '%s'", component, stackName)
					}
				}

				componentHooks := map[string]any{}
				if i, ok := componentMap[cfg.HooksSectionName]; ok {
					componentHooks, ok = i.(map[string]any)
					if !ok {
						return nil, fmt.Errorf("invalid 'components.terraform.%s.hooks' section in the file '%s'", component, stackName)
					}
				}

				// Component metadata.
				// This is per component, not deep-merged and not inherited from base components and globals.
				componentMetadata := map[string]any{}
				if i, ok := componentMap[cfg.MetadataSectionName]; ok {
					componentMetadata, ok = i.(map[string]any)
					if !ok {
						return nil, fmt.Errorf("invalid 'components.terraform.%s.metadata' section in the file '%s'", component, stackName)
					}
				}

				// Component backend
				componentBackendType := ""
				componentBackendSection := map[string]any{}

				if i, ok := componentMap[cfg.BackendTypeSectionName]; ok {
					componentBackendType, ok = i.(string)
					if !ok {
						return nil, fmt.Errorf("invalid 'components.terraform.%s.backend_type' attribute in the file '%s'", component, stackName)
					}
				}

				if i, ok := componentMap[cfg.BackendSectionName]; ok {
					componentBackendSection, ok = i.(map[string]any)
					if !ok {
						return nil, fmt.Errorf("invalid 'components.terraform.%s.backend' section in the file '%s'", component, stackName)
					}
				}

				// Component remote state backend
				componentRemoteStateBackendType := ""
				componentRemoteStateBackendSection := map[string]any{}

				if i, ok := componentMap["remote_state_backend_type"]; ok {
					componentRemoteStateBackendType, ok = i.(string)
					if !ok {
						return nil, fmt.Errorf("invalid 'components.terraform.%s.remote_state_backend_type' attribute in the file '%s'", component, stackName)
					}
				}

				if i, ok := componentMap["remote_state_backend"]; ok {
					componentRemoteStateBackendSection, ok = i.(map[string]any)
					if !ok {
						return nil, fmt.Errorf("invalid 'components.terraform.%s.remote_state_backend' section in the file '%s'", component, stackName)
					}
				}

				componentTerraformCommand := ""
				if i, ok := componentMap[cfg.CommandSectionName]; ok {
					componentTerraformCommand, ok = i.(string)
					if !ok {
						return nil, fmt.Errorf("invalid 'components.terraform.%s.command' attribute in the file '%s'", component, stackName)
					}
				}

				// Process overrides
				componentOverrides := map[string]any{}
				componentOverridesVars := map[string]any{}
				componentOverridesSettings := map[string]any{}
				componentOverridesEnv := map[string]any{}
				componentOverridesProviders := map[string]any{}
				componentOverridesHooks := map[string]any{}
				componentOverridesTerraformCommand := ""

				if i, ok := componentMap[cfg.OverridesSectionName]; ok {
					if componentOverrides, ok = i.(map[string]any); !ok {
						return nil, fmt.Errorf("invalid 'components.terraform.%s.overrides' in the manifest '%s'", component, stackName)
					}

					if i, ok = componentOverrides[cfg.VarsSectionName]; ok {
						if componentOverridesVars, ok = i.(map[string]any); !ok {
							return nil, fmt.Errorf("invalid 'components.terraform.%s.overrides.vars' in the manifest '%s'", component, stackName)
						}
					}

					if i, ok = componentOverrides[cfg.SettingsSectionName]; ok {
						if componentOverridesSettings, ok = i.(map[string]any); !ok {
							return nil, fmt.Errorf("invalid 'components.terraform.%s.overrides.settings' in the manifest '%s'", component, stackName)
						}
					}

					if i, ok = componentOverrides[cfg.EnvSectionName]; ok {
						if componentOverridesEnv, ok = i.(map[string]any); !ok {
							return nil, fmt.Errorf("invalid 'components.terraform.%s.overrides.env' in the manifest '%s'", component, stackName)
						}
					}

					if i, ok = componentOverrides[cfg.CommandSectionName]; ok {
						if componentOverridesTerraformCommand, ok = i.(string); !ok {
							return nil, fmt.Errorf("invalid 'components.terraform.%s.overrides.command' in the manifest '%s'", component, stackName)
						}
					}

					if i, ok = componentOverrides[cfg.ProvidersSectionName]; ok {
						if componentOverridesProviders, ok = i.(map[string]any); !ok {
							return nil, fmt.Errorf("invalid 'components.terraform.%s.overrides.providers' in the manifest '%s'", component, stackName)
						}
					}

					if i, ok = componentOverrides[cfg.HooksSectionName]; ok {
						if componentOverridesHooks, ok = i.(map[string]any); !ok {
							return nil, fmt.Errorf("invalid 'components.terraform.%s.overrides.hooks' in the manifest '%s'", component, stackName)
						}
					}
				}

				// Process base component(s)
				baseComponentName := ""
				baseComponentVars := map[string]any{}
				baseComponentSettings := map[string]any{}
				baseComponentEnv := map[string]any{}
				baseComponentProviders := map[string]any{}
				baseComponentHooks := map[string]any{}
				baseComponentTerraformCommand := ""
				baseComponentBackendType := ""
				baseComponentBackendSection := map[string]any{}
				baseComponentRemoteStateBackendType := ""
				baseComponentRemoteStateBackendSection := map[string]any{}
				var baseComponentConfig schema.BaseComponentConfig
				var componentInheritanceChain []string
				var baseComponents []string

				// Inheritance using the top-level `component` attribute
				if baseComponent, baseComponentExist := componentMap[cfg.ComponentSectionName]; baseComponentExist {
					baseComponentName, ok = baseComponent.(string)
					if !ok {
						return nil, fmt.Errorf("invalid 'components.terraform.%s.component' attribute in the file '%s'", component, stackName)
					}

					// Process the base components recursively to find `componentInheritanceChain`
					err = ProcessBaseComponentConfig(
						atmosConfig,
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
					baseComponentProviders = baseComponentConfig.BaseComponentProviders
					baseComponentHooks = baseComponentConfig.BaseComponentHooks
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
				if baseComponentFromMetadata, baseComponentFromMetadataExist := componentMetadata[cfg.ComponentSectionName]; baseComponentFromMetadataExist {
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
								errorMessage := fmt.Sprintf("The component '%[1]s' in the stack manifest '%[2]s' inherits from '%[3]s' "+
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
							atmosConfig,
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
				finalComponentVars, err := m.Merge(
					atmosConfig,
					[]map[string]any{
						globalAndTerraformVars,
						baseComponentVars,
						componentVars,
						componentOverridesVars,
					})
				if err != nil {
					return nil, err
				}

				finalComponentSettings, err := m.Merge(
					atmosConfig,
					[]map[string]any{
						globalAndTerraformSettings,
						baseComponentSettings,
						componentSettings,
						componentOverridesSettings,
					})
				if err != nil {
					return nil, err
				}

				finalComponentEnv, err := m.Merge(
					atmosConfig,
					[]map[string]any{
						globalAndTerraformEnv,
						baseComponentEnv,
						componentEnv,
						componentOverridesEnv,
					})
				if err != nil {
					return nil, err
				}

				finalComponentProviders, err := m.Merge(
					atmosConfig,
					[]map[string]any{
						terraformProviders,
						baseComponentProviders,
						componentProviders,
						componentOverridesProviders,
					})
				if err != nil {
					return nil, err
				}

				finalComponentHooks, err := m.Merge(
					atmosConfig,
					[]map[string]any{
						globalAndTerraformHooks,
						baseComponentHooks,
						componentHooks,
						componentOverridesHooks,
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

				finalComponentBackendSection, err := m.Merge(
					atmosConfig,
					[]map[string]any{
						globalBackendSection,
						baseComponentBackendSection,
						componentBackendSection,
					})
				if err != nil {
					return nil, err
				}

				finalComponentBackend := map[string]any{}
				if i, ok := finalComponentBackendSection[finalComponentBackendType]; ok {
					finalComponentBackend, ok = i.(map[string]any)
					if !ok {
						return nil, fmt.Errorf("invalid 'terraform.backend' section for the component '%s'", component)
					}
				}

				// AWS S3 backend
				// Check if `backend` section has `workspace_key_prefix` for `s3` backend type
				// If it does not, use the component name instead
				// It will also be propagated to `remote_state_backend` section of `s3` type
				if finalComponentBackendType == "s3" {
					if p, ok := finalComponentBackend["workspace_key_prefix"].(string); !ok || p == "" {
						workspaceKeyPrefix := component
						if baseComponentName != "" {
							workspaceKeyPrefix = baseComponentName
						}
						finalComponentBackend["workspace_key_prefix"] = strings.Replace(workspaceKeyPrefix, "/", "-", -1)
					}
				}

				// Google GSC backend
				// Check if `backend` section has `prefix` for `gcs` backend type
				// If it does not, use the component name instead
				// https://developer.hashicorp.com/terraform/language/settings/backends/gcs
				// https://developer.hashicorp.com/terraform/language/settings/backends/gcs#prefix
				if finalComponentBackendType == "gcs" {
					if p, ok := finalComponentBackend["prefix"].(string); !ok || p == "" {
						prefix := component
						if baseComponentName != "" {
							prefix = baseComponentName
						}
						finalComponentBackend["prefix"] = strings.Replace(prefix, "/", "-", -1)
					}
				}

				// Azure backend
				// Check if component `backend` section has `key` for `azurerm` backend type
				// If it does not, use the component name instead and format it with the global backend key name to auto generate a unique Terraform state key
				// The backend state file will be formatted like so: {global key name}/{component name}.terraform.tfstate
				if finalComponentBackendType == "azurerm" {
					if componentAzurerm, componentAzurermExists := componentBackendSection["azurerm"].(map[string]any); !componentAzurermExists {
						if _, componentAzurermKeyExists := componentAzurerm["key"].(string); !componentAzurermKeyExists {
							azureKeyPrefixComponent := component
							var keyName []string
							if baseComponentName != "" {
								azureKeyPrefixComponent = baseComponentName
							}
							if globalAzurerm, globalAzurermExists := globalBackendSection["azurerm"].(map[string]any); globalAzurermExists {
								if _, globalAzurermKeyExists := globalAzurerm["key"].(string); globalAzurermKeyExists {
									keyName = append(keyName, globalAzurerm["key"].(string))
								}
							}
							componentKeyName := strings.Replace(azureKeyPrefixComponent, "/", "-", -1)
							keyName = append(keyName, fmt.Sprintf("%s.terraform.tfstate", componentKeyName))
							finalComponentBackend["key"] = strings.Join(keyName, "/")
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

				finalComponentRemoteStateBackendSection, err := m.Merge(
					atmosConfig,
					[]map[string]any{
						globalRemoteStateBackendSection,
						baseComponentRemoteStateBackendSection,
						componentRemoteStateBackendSection,
					})
				if err != nil {
					return nil, err
				}

				// Merge `backend` and `remote_state_backend` sections
				// This will allow keeping `remote_state_backend` section DRY
				finalComponentRemoteStateBackendSectionMerged, err := m.Merge(
					atmosConfig,
					[]map[string]any{
						finalComponentBackendSection,
						finalComponentRemoteStateBackendSection,
					})
				if err != nil {
					return nil, err
				}

				finalComponentRemoteStateBackend := map[string]any{}
				if i, ok := finalComponentRemoteStateBackendSectionMerged[finalComponentRemoteStateBackendType]; ok {
					finalComponentRemoteStateBackend, ok = i.(map[string]any)
					if !ok {
						return nil, fmt.Errorf("invalid 'terraform.remote_state_backend' section for the component '%s'", component)
					}
				}

				// Final binary to execute
				// Check for the binary in the following order:
				// - `components.terraform.command` section in `atmos.yaml` CLI config file
				// - global `terraform.command` section
				// - base component(s) `command` section
				// - component `command` section
				// - `overrides.command` section
				finalComponentTerraformCommand := "terraform"
				if atmosConfig.Components.Terraform.Command != "" {
					finalComponentTerraformCommand = atmosConfig.Components.Terraform.Command
				}
				if terraformCommand != "" {
					finalComponentTerraformCommand = terraformCommand
				}
				if baseComponentTerraformCommand != "" {
					finalComponentTerraformCommand = baseComponentTerraformCommand
				}
				if componentTerraformCommand != "" {
					finalComponentTerraformCommand = componentTerraformCommand
				}
				if componentOverridesTerraformCommand != "" {
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
						spaceliftSettings, ok := i.(map[string]any)
						if !ok {
							return nil, fmt.Errorf("invalid 'components.terraform.%s.settings.spacelift' section in the file '%s'", component, stackName)
						}
						delete(spaceliftSettings, "workspace_enabled")
					}
				}

				finalSettings, err := processSettingsIntegrationsGithub(atmosConfig, finalComponentSettings)
				if err != nil {
					return nil, err
				}

				comp := map[string]any{}
				comp[cfg.VarsSectionName] = finalComponentVars
				comp[cfg.SettingsSectionName] = finalSettings
				comp[cfg.EnvSectionName] = finalComponentEnv
				comp[cfg.BackendTypeSectionName] = finalComponentBackendType
				comp[cfg.BackendSectionName] = finalComponentBackend
				comp[cfg.RemoteStateBackendTypeSectionName] = finalComponentRemoteStateBackendType
				comp[cfg.RemoteStateBackendSectionName] = finalComponentRemoteStateBackend
				comp[cfg.CommandSectionName] = finalComponentTerraformCommand
				comp[cfg.InheritanceSectionName] = componentInheritanceChain
				comp[cfg.MetadataSectionName] = componentMetadata
				comp[cfg.OverridesSectionName] = componentOverrides
				comp[cfg.ProvidersSectionName] = finalComponentProviders
				comp[cfg.HooksSectionName] = finalComponentHooks

				if baseComponentName != "" {
					comp[cfg.ComponentSectionName] = baseComponentName
				}

				terraformComponents[component] = comp
			}
		}
	}

	// Process all helmfile components
	if componentTypeFilter == "" || componentTypeFilter == "helmfile" {
		if allHelmfileComponents, ok := globalComponentsSection["helmfile"]; ok {

			allHelmfileComponentsMap, ok := allHelmfileComponents.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid 'components.helmfile' section in the file '%s'", stackName)
			}

			for cmp, v := range allHelmfileComponentsMap {
				component := cmp

				componentMap, ok := v.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("invalid 'components.helmfile.%s' section in the file '%s'", component, stackName)
				}

				componentVars := map[string]any{}
				if i2, ok := componentMap[cfg.VarsSectionName]; ok {
					componentVars, ok = i2.(map[string]any)
					if !ok {
						return nil, fmt.Errorf("invalid 'components.helmfile.%s.vars' section in the file '%s'", component, stackName)
					}
				}

				componentSettings := map[string]any{}
				if i, ok := componentMap[cfg.SettingsSectionName]; ok {
					componentSettings, ok = i.(map[string]any)
					if !ok {
						return nil, fmt.Errorf("invalid 'components.helmfile.%s.settings' section in the file '%s'", component, stackName)
					}
				}

				componentEnv := map[string]any{}
				if i, ok := componentMap[cfg.EnvSectionName]; ok {
					componentEnv, ok = i.(map[string]any)
					if !ok {
						return nil, fmt.Errorf("invalid 'components.helmfile.%s.env' section in the file '%s'", component, stackName)
					}
				}

				// Component metadata.
				// This is per component, not deep-merged and not inherited from base components and globals.
				componentMetadata := map[string]any{}
				if i, ok := componentMap[cfg.MetadataSectionName]; ok {
					componentMetadata, ok = i.(map[string]any)
					if !ok {
						return nil, fmt.Errorf("invalid 'components.helmfile.%s.metadata' section in the file '%s'", component, stackName)
					}
				}

				componentHelmfileCommand := ""
				if i, ok := componentMap[cfg.CommandSectionName]; ok {
					componentHelmfileCommand, ok = i.(string)
					if !ok {
						return nil, fmt.Errorf("invalid 'components.helmfile.%s.command' attribute in the file '%s'", component, stackName)
					}
				}

				// Process overrides
				componentOverrides := map[string]any{}
				componentOverridesVars := map[string]any{}
				componentOverridesSettings := map[string]any{}
				componentOverridesEnv := map[string]any{}
				componentOverridesHelmfileCommand := ""

				if i, ok := componentMap[cfg.OverridesSectionName]; ok {
					if componentOverrides, ok = i.(map[string]any); !ok {
						return nil, fmt.Errorf("invalid 'components.helmfile.%s.overrides' in the manifest '%s'", component, stackName)
					}

					if i, ok = componentOverrides[cfg.VarsSectionName]; ok {
						if componentOverridesVars, ok = i.(map[string]any); !ok {
							return nil, fmt.Errorf("invalid 'components.helmfile.%s.overrides.vars' in the manifest '%s'", component, stackName)
						}
					}

					if i, ok = componentOverrides[cfg.SettingsSectionName]; ok {
						if componentOverridesSettings, ok = i.(map[string]any); !ok {
							return nil, fmt.Errorf("invalid 'components.helmfile.%s.overrides.settings' in the manifest '%s'", component, stackName)
						}
					}

					if i, ok = componentOverrides[cfg.EnvSectionName]; ok {
						if componentOverridesEnv, ok = i.(map[string]any); !ok {
							return nil, fmt.Errorf("invalid 'components.helmfile.%s.overrides.env' in the manifest '%s'", component, stackName)
						}
					}

					if i, ok = componentOverrides[cfg.CommandSectionName]; ok {
						if componentOverridesHelmfileCommand, ok = i.(string); !ok {
							return nil, fmt.Errorf("invalid 'components.helmfile.%s.overrides.command' in the manifest '%s'", component, stackName)
						}
					}
				}

				// Process base component(s)
				baseComponentVars := map[string]any{}
				baseComponentSettings := map[string]any{}
				baseComponentEnv := map[string]any{}
				baseComponentName := ""
				baseComponentHelmfileCommand := ""
				var baseComponentConfig schema.BaseComponentConfig
				var componentInheritanceChain []string
				var baseComponents []string

				// Inheritance using the top-level `component` attribute
				if baseComponent, baseComponentExist := componentMap[cfg.ComponentSectionName]; baseComponentExist {
					baseComponentName, ok = baseComponent.(string)
					if !ok {
						return nil, fmt.Errorf("invalid 'components.helmfile.%s.component' attribute in the file '%s'", component, stackName)
					}

					// Process the base components recursively to find `componentInheritanceChain`
					err = ProcessBaseComponentConfig(
						atmosConfig,
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
				if baseComponentFromMetadata, baseComponentFromMetadataExist := componentMetadata[cfg.ComponentSectionName]; baseComponentFromMetadataExist {
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
								errorMessage := fmt.Sprintf("The component '%[1]s' in the stack manifest '%[2]s' inherits from '%[3]s' "+
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
							atmosConfig,
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
				finalComponentVars, err := m.Merge(
					atmosConfig,
					[]map[string]any{
						globalAndHelmfileVars,
						baseComponentVars,
						componentVars,
						componentOverridesVars,
					})
				if err != nil {
					return nil, err
				}

				finalComponentSettings, err := m.Merge(
					atmosConfig,
					[]map[string]any{
						globalAndHelmfileSettings,
						baseComponentSettings,
						componentSettings,
						componentOverridesSettings,
					})
				if err != nil {
					return nil, err
				}

				finalComponentEnv, err := m.Merge(
					atmosConfig,
					[]map[string]any{
						globalAndHelmfileEnv,
						baseComponentEnv,
						componentEnv,
						componentOverridesEnv,
					})
				if err != nil {
					return nil, err
				}

				// Final binary to execute
				// Check for the binary in the following order:
				// - `components.helmfile.command` section in `atmos.yaml` CLI config file
				// - global `helmfile.command` section
				// - base component(s) `command` section
				// - component `command` section
				// - `overrides.command` section
				finalComponentHelmfileCommand := "helmfile"
				if atmosConfig.Components.Helmfile.Command != "" {
					finalComponentHelmfileCommand = atmosConfig.Components.Helmfile.Command
				}
				if helmfileCommand != "" {
					finalComponentHelmfileCommand = helmfileCommand
				}
				if baseComponentHelmfileCommand != "" {
					finalComponentHelmfileCommand = baseComponentHelmfileCommand
				}
				if componentHelmfileCommand != "" {
					finalComponentHelmfileCommand = componentHelmfileCommand
				}
				if componentOverridesHelmfileCommand != "" {
					finalComponentHelmfileCommand = componentOverridesHelmfileCommand
				}

				finalSettings, err := processSettingsIntegrationsGithub(atmosConfig, finalComponentSettings)
				if err != nil {
					return nil, err
				}

				comp := map[string]any{}
				comp[cfg.VarsSectionName] = finalComponentVars
				comp[cfg.SettingsSectionName] = finalSettings
				comp[cfg.EnvSectionName] = finalComponentEnv
				comp[cfg.CommandSectionName] = finalComponentHelmfileCommand
				comp["inheritance"] = componentInheritanceChain
				comp[cfg.MetadataSectionName] = componentMetadata
				comp[cfg.OverridesSectionName] = componentOverrides

				if baseComponentName != "" {
					comp[cfg.ComponentSectionName] = baseComponentName
				}

				helmfileComponents[component] = comp
			}
		}
	}

	allComponents["terraform"] = terraformComponents
	allComponents["helmfile"] = helmfileComponents

	result := map[string]any{
		"components": allComponents,
	}

	return result, nil
}

// processSettingsIntegrationsGithub deep-merges the `settings.integrations.github` section from stack manifests with
// the `integrations.github` section from `atmos.yaml`
func processSettingsIntegrationsGithub(atmosConfig schema.AtmosConfiguration, settings map[string]any) (map[string]any, error) {
	settingsIntegrationsSection := make(map[string]any)
	settingsIntegrationsGithubSection := make(map[string]any)

	// Find `settings.integrations.github` section from stack manifests
	if settingsIntegrations, ok := settings[cfg.IntegrationsSectionName]; ok {
		if settingsIntegrationsMap, ok := settingsIntegrations.(map[string]any); ok {
			settingsIntegrationsSection = settingsIntegrationsMap
			if settingsIntegrationsGithub, ok := settingsIntegrationsMap[cfg.GithubSectionName]; ok {
				if settingsIntegrationsGithubMap, ok := settingsIntegrationsGithub.(map[string]any); ok {
					settingsIntegrationsGithubSection = settingsIntegrationsGithubMap
				}
			}
		}
	}

	// deep-merge the `settings.integrations.github` section from stack manifests with  the `integrations.github` section from `atmos.yaml`
	settingsIntegrationsGithubMerged, err := m.Merge(
		atmosConfig,
		[]map[string]any{
			atmosConfig.Integrations.GitHub,
			settingsIntegrationsGithubSection,
		})
	if err != nil {
		return nil, err
	}

	// Update the `settings.integrations.github` section
	if len(settingsIntegrationsGithubMerged) > 0 {
		settingsIntegrationsSection[cfg.GithubSectionName] = settingsIntegrationsGithubMerged
		settings[cfg.IntegrationsSectionName] = settingsIntegrationsSection
	}

	return settings, nil
}

// FindComponentStacks finds all infrastructure stack manifests where the component or the base component is defined
func FindComponentStacks(
	componentType string,
	component string,
	baseComponent string,
	componentStackMap map[string]map[string][]string,
) ([]string, error) {
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
	stackImports map[string]map[string]any,
) ([]string, error) {
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
			if sectionContainsAnyNotEmptySections(stackImportMap[componentType].(map[string]any), sectionsToCheck) {
				deps = append(deps, stackImportName)
				continue
			}
		}

		stackImportMapComponentsSection, ok := stackImportMap["components"].(map[string]any)
		if !ok {
			continue
		}

		stackImportMapComponentTypeSection, ok := stackImportMapComponentsSection[componentType].(map[string]any)
		if !ok {
			continue
		}

		if stackImportMapComponentSection, ok := stackImportMapComponentTypeSection[component].(map[string]any); ok {
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
			baseComponentSection, ok := stackImportMapComponentTypeSection[baseComponent].(map[string]any)

			if !ok || len(baseComponentSection) == 0 {
				continue
			}

			importOfStackImportStructs, err := ProcessImportSection(stackImportMap, stack)
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

				importOfStackImportComponentsSection, ok := importOfStackImportMap["components"].(map[string]any)
				if !ok {
					continue
				}

				importOfStackImportComponentTypeSection, ok := importOfStackImportComponentsSection[componentType].(map[string]any)
				if !ok {
					continue
				}

				importOfStackImportBaseComponentSection, ok := importOfStackImportComponentTypeSection[baseComponent].(map[string]any)
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

// ProcessImportSection processes the `import` section in stack manifests
// The `import` section can contain:
// 1. Project-relative paths (e.g. "mixins/region/us-east-2")
// 2. Paths relative to the current stack file (e.g. "./_defaults")
// 3. StackImport structs containing either of the above path types (e.g. "path: mixins/region/us-east-2")
func ProcessImportSection(stackMap map[string]any, filePath string) ([]schema.StackImport, error) {
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
			importObj.Path = u.ResolveRelativePath(importObj.Path, filePath)
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

		s = u.ResolveRelativePath(s, filePath)
		result = append(result, schema.StackImport{Path: s})
	}

	return result, nil
}

// sectionContainsAnyNotEmptySections checks if a section contains any of the provided low-level sections, and it's not empty
func sectionContainsAnyNotEmptySections(section map[string]any, sectionsToCheck []string) bool {
	for _, s := range sectionsToCheck {
		if len(s) > 0 {
			if v, ok := section[s]; ok {
				if v2, ok2 := v.(map[string]any); ok2 && len(v2) > 0 {
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
	atmosConfig schema.AtmosConfiguration,
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

	dir := filepath.Dir(filePath)

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
				config, _, _, _, _, _, _, err := ProcessYAMLConfigFile(
					atmosConfig,
					stacksBasePath,
					p,
					map[string]map[string]any{},
					nil,
					false,
					false,
					false,
					false,
					map[string]any{},
					map[string]any{},
					map[string]any{},
					map[string]any{},
					"",
				)
				if err != nil {
					return err
				}

				finalConfig, err := ProcessStackConfig(
					atmosConfig,
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
			componentStackMap["terraform"][component] = append(componentStackMap["terraform"][component], strings.Replace(stack, u.DefaultStackConfigFileExtension, "", 1))
		}
	}

	for stack, components := range stackComponentMap["helmfile"] {
		for _, component := range components {
			componentStackMap["helmfile"][component] = append(componentStackMap["helmfile"][component], strings.Replace(stack, u.DefaultStackConfigFileExtension, "", 1))
		}
	}

	return componentStackMap, nil
}

// GetFileContent tries to read and return the file content from the sync map if it exists in the map,
// otherwise it reads the file, stores its content in the map and returns the content
func GetFileContent(filePath string) (string, error) {
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
	atmosConfig schema.AtmosConfiguration,
	baseComponentConfig *schema.BaseComponentConfig,
	allComponentsMap map[string]any,
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

	var baseComponentVars map[string]any
	var baseComponentSettings map[string]any
	var baseComponentEnv map[string]any
	var baseComponentProviders map[string]any
	var baseComponentHooks map[string]any
	var baseComponentCommand string
	var baseComponentBackendType string
	var baseComponentBackendSection map[string]any
	var baseComponentRemoteStateBackendType string
	var baseComponentRemoteStateBackendSection map[string]any
	var baseComponentMap map[string]any
	var ok bool

	*baseComponents = append(*baseComponents, baseComponent)

	if baseComponentSection, baseComponentSectionExist := allComponentsMap[baseComponent]; baseComponentSectionExist {
		baseComponentMap, ok = baseComponentSection.(map[string]any)
		if !ok {
			// Depending on the code and libraries, the section can have different map types: map[string]any or map[string]any
			// We try to convert to both
			baseComponentMapOfStrings, ok := baseComponentSection.(map[string]any)
			if !ok {
				return fmt.Errorf("invalid config for the base component '%s' of the component '%s' in the stack '%s'",
					baseComponent, component, stack)
			}
			baseComponentMap = baseComponentMapOfStrings
		}

		// First, process the base component(s) of this base component
		if baseComponentOfBaseComponent, baseComponentOfBaseComponentExist := baseComponentMap["component"]; baseComponentOfBaseComponentExist {
			baseComponentOfBaseComponentString, ok := baseComponentOfBaseComponent.(string)
			if !ok {
				return fmt.Errorf("invalid 'component:' section of the component '%s' in the stack '%s'",
					baseComponent, stack)
			}

			err := ProcessBaseComponentConfig(
				atmosConfig,
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
		componentMetadata := map[string]any{}
		if i, ok := baseComponentMap["metadata"]; ok {
			componentMetadata, ok = i.(map[string]any)
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
						atmosConfig,
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
			baseComponentVars, ok = baseComponentVarsSection.(map[string]any)
			if !ok {
				return fmt.Errorf("invalid '%s.vars' section in the stack '%s'", baseComponent, stack)
			}
		}

		if baseComponentSettingsSection, baseComponentSettingsSectionExist := baseComponentMap["settings"]; baseComponentSettingsSectionExist {
			baseComponentSettings, ok = baseComponentSettingsSection.(map[string]any)
			if !ok {
				return fmt.Errorf("invalid '%s.settings' section in the stack '%s'", baseComponent, stack)
			}
		}

		if baseComponentEnvSection, baseComponentEnvSectionExist := baseComponentMap["env"]; baseComponentEnvSectionExist {
			baseComponentEnv, ok = baseComponentEnvSection.(map[string]any)
			if !ok {
				return fmt.Errorf("invalid '%s.env' section in the stack '%s'", baseComponent, stack)
			}
		}

		if baseComponentProvidersSection, baseComponentProvidersSectionExist := baseComponentMap[cfg.ProvidersSectionName]; baseComponentProvidersSectionExist {
			baseComponentProviders, ok = baseComponentProvidersSection.(map[string]any)
			if !ok {
				return fmt.Errorf("invalid '%s.providers' section in the stack '%s'", baseComponent, stack)
			}
		}

		if baseComponentHooksSection, baseComponentHooksSectionExist := baseComponentMap[cfg.HooksSectionName]; baseComponentHooksSectionExist {
			baseComponentHooks, ok = baseComponentHooksSection.(map[string]any)
			if !ok {
				return fmt.Errorf("invalid '%s.hooks' section in the stack '%s'", baseComponent, stack)
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
			baseComponentBackendSection, ok = i.(map[string]any)
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
			baseComponentRemoteStateBackendSection, ok = i.(map[string]any)
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
		merged, err := m.Merge(atmosConfig, []map[string]any{baseComponentConfig.BaseComponentVars, baseComponentVars})
		if err != nil {
			return err
		}
		baseComponentConfig.BaseComponentVars = merged

		// Base component `settings`
		merged, err = m.Merge(atmosConfig, []map[string]any{baseComponentConfig.BaseComponentSettings, baseComponentSettings})
		if err != nil {
			return err
		}
		baseComponentConfig.BaseComponentSettings = merged

		// Base component `env`
		merged, err = m.Merge(atmosConfig, []map[string]any{baseComponentConfig.BaseComponentEnv, baseComponentEnv})
		if err != nil {
			return err
		}
		baseComponentConfig.BaseComponentEnv = merged

		// Base component `providers`
		merged, err = m.Merge(atmosConfig, []map[string]any{baseComponentConfig.BaseComponentProviders, baseComponentProviders})
		if err != nil {
			return err
		}
		baseComponentConfig.BaseComponentProviders = merged

		// Base component `hooks`
		merged, err = m.Merge(atmosConfig, []map[string]any{baseComponentConfig.BaseComponentHooks, baseComponentHooks})
		if err != nil {
			return err
		}
		baseComponentConfig.BaseComponentHooks = merged

		// Base component `command`
		baseComponentConfig.BaseComponentCommand = baseComponentCommand

		// Base component `backend_type`
		baseComponentConfig.BaseComponentBackendType = baseComponentBackendType

		// Base component `backend`
		merged, err = m.Merge(atmosConfig, []map[string]any{baseComponentConfig.BaseComponentBackendSection, baseComponentBackendSection})
		if err != nil {
			return err
		}
		baseComponentConfig.BaseComponentBackendSection = merged

		// Base component `remote_state_backend_type`
		baseComponentConfig.BaseComponentRemoteStateBackendType = baseComponentRemoteStateBackendType

		// Base component `remote_state_backend`
		merged, err = m.Merge(atmosConfig, []map[string]any{baseComponentConfig.BaseComponentRemoteStateBackendSection, baseComponentRemoteStateBackendSection})
		if err != nil {
			return err
		}
		baseComponentConfig.BaseComponentRemoteStateBackendSection = merged

		baseComponentConfig.ComponentInheritanceChain = u.UniqueStrings(append([]string{baseComponent}, baseComponentConfig.ComponentInheritanceChain...))
	} else {
		if checkBaseComponentExists {
			// Check if the base component exists as Terraform/Helmfile component
			// If it does exist, don't throw errors if it is not defined in YAML config
			componentPath := filepath.Join(componentBasePath, baseComponent)
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
