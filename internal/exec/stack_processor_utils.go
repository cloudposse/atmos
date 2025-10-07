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

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/santhosh-tekuri/jsonschema/v5"

	cfg "github.com/cloudposse/atmos/pkg/config"
	m "github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

var (
	// File content sync map.
	getFileContentSyncMap = sync.Map{}

	// Mutex to serialize updates of the result map of ProcessYAMLConfigFiles function.
	processYAMLConfigFilesLock = &sync.Mutex{}

	// The mergeContexts stores MergeContexts keyed by stack file path when provenance tracking is enabled.
	// This is used to capture provenance data for the describe component command.
	mergeContexts   = make(map[string]*m.MergeContext)
	mergeContextsMu sync.RWMutex

	// Deprecated: Use SetMergeContextForStack/GetMergeContextForStack instead.
	lastMergeContext   *m.MergeContext
	lastMergeContextMu sync.RWMutex
)

// SetMergeContextForStack stores the merge context for a specific stack file.
func SetMergeContextForStack(stackFile string, ctx *m.MergeContext) {
	defer perf.Track(nil, "exec.SetMergeContextForStack")()

	mergeContextsMu.Lock()
	defer mergeContextsMu.Unlock()
	mergeContexts[stackFile] = ctx
}

// GetMergeContextForStack retrieves the merge context for a specific stack file.
func GetMergeContextForStack(stackFile string) *m.MergeContext {
	defer perf.Track(nil, "exec.GetMergeContextForStack")()

	mergeContextsMu.RLock()
	defer mergeContextsMu.RUnlock()
	return mergeContexts[stackFile]
}

// ClearMergeContexts clears all stored merge contexts.
func ClearMergeContexts() {
	defer perf.Track(nil, "exec.ClearMergeContexts")()

	mergeContextsMu.Lock()
	defer mergeContextsMu.Unlock()
	mergeContexts = make(map[string]*m.MergeContext)
}

// SetLastMergeContext stores the merge context for later retrieval.
// Deprecated: Use SetMergeContextForStack instead.
func SetLastMergeContext(ctx *m.MergeContext) {
	defer perf.Track(nil, "exec.SetLastMergeContext")()

	lastMergeContextMu.Lock()
	defer lastMergeContextMu.Unlock()
	lastMergeContext = ctx
}

// GetLastMergeContext retrieves the last stored merge context.
// Deprecated: Use GetMergeContextForStack instead.
func GetLastMergeContext() *m.MergeContext {
	defer perf.Track(nil, "exec.GetLastMergeContext")()

	lastMergeContextMu.RLock()
	defer lastMergeContextMu.RUnlock()
	return lastMergeContext
}

// ClearLastMergeContext clears the stored merge context.
// Deprecated: Use ClearMergeContexts instead.
func ClearLastMergeContext() {
	defer perf.Track(nil, "exec.ClearLastMergeContext")()

	lastMergeContextMu.Lock()
	defer lastMergeContextMu.Unlock()
	lastMergeContext = nil
}

// ProcessYAMLConfigFiles takes a list of paths to stack manifests, processes and deep-merges all imports, and returns a list of stack configs.
func ProcessYAMLConfigFiles(
	atmosConfig *schema.AtmosConfiguration,
	stacksBasePath string,
	terraformComponentsBasePath string,
	helmfileComponentsBasePath string,
	packerComponentsBasePath string,
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
	defer perf.Track(atmosConfig, "exec.ProcessYAMLConfigFiles")()

	count := len(filePaths)
	listResult := make([]string, count)
	mapResult := map[string]any{}
	rawStackConfigs := map[string]map[string]any{}
	var errorResult error
	var errorLock sync.Mutex
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

			// Each goroutine gets its own merge context to avoid data races.
			// For single-file operations (like describe component), use the
			// SetLastMergeContext/GetLastMergeContext mechanism instead.
			mergeContext := m.NewMergeContext()
			if atmosConfig != nil && atmosConfig.TrackProvenance {
				mergeContext.EnableProvenance()
			}

			deepMergedStackConfig, importsConfig, stackConfig, _, _, _, _, err := ProcessYAMLConfigFileWithContext(
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
				mergeContext,
			)
			if err != nil {
				errorLock.Lock()
				errorResult = err
				errorLock.Unlock()
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
				packerComponentsBasePath,
				p,
				deepMergedStackConfig,
				processStackDeps,
				processComponentDeps,
				"",
				componentStackMap,
				importsConfig,
				true)
			if err != nil {
				errorLock.Lock()
				errorResult = err
				errorLock.Unlock()
				return
			}

			finalConfig["imports"] = uniqueImports

			// Store merge context for this stack file if provenance tracking is enabled.
			if atmosConfig != nil && atmosConfig.TrackProvenance && mergeContext != nil && mergeContext.IsProvenanceEnabled() {
				SetMergeContextForStack(stackFileName, mergeContext)
				// Also set as last merge context for backward compatibility (note: may be overwritten by other goroutines)
				SetLastMergeContext(mergeContext)
			}

			yamlConfig, err := u.ConvertToYAML(finalConfig)
			if err != nil {
				errorLock.Lock()
				errorResult = err
				errorLock.Unlock()
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
// recursively processes and deep-merges all the imports,
// and returns the final stack config.
func ProcessYAMLConfigFile(
	atmosConfig *schema.AtmosConfiguration,
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
	defer perf.Track(atmosConfig, "exec.ProcessYAMLConfigFile")()

	// Create merge context for single-file operations
	var mergeContext *m.MergeContext
	if atmosConfig != nil && atmosConfig.TrackProvenance {
		mergeContext = m.NewMergeContext()
		mergeContext.EnableProvenance()
	}

	// Call the context-aware version
	deepMerged, imports, stackConfig, terraformInline, terraformImports, helmfileInline, helmfileImports, err := ProcessYAMLConfigFileWithContext(
		atmosConfig,
		basePath,
		filePath,
		importsConfig,
		context,
		ignoreMissingFiles,
		skipTemplatesProcessingInImports,
		ignoreMissingTemplateValues,
		skipIfMissing,
		parentTerraformOverridesInline,
		parentTerraformOverridesImports,
		parentHelmfileOverridesInline,
		parentHelmfileOverridesImports,
		atmosManifestJsonSchemaFilePath,
		mergeContext,
	)

	// Store merge context if provenance tracking is enabled (for single-file operations like describe component)
	if atmosConfig != nil && atmosConfig.TrackProvenance && mergeContext != nil && mergeContext.IsProvenanceEnabled() {
		SetLastMergeContext(mergeContext)
	}

	return deepMerged, imports, stackConfig, terraformInline, terraformImports, helmfileInline, helmfileImports, err
}

// ProcessYAMLConfigFileWithContext takes a path to a YAML stack manifest,
// recursively processes and deep-merges all the imports with context tracking,
// and returns the final stack config.
//
//nolint:gocognit,revive,cyclop,funlen
func ProcessYAMLConfigFileWithContext(
	atmosConfig *schema.AtmosConfiguration,
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
	mergeContext *m.MergeContext,
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
	defer perf.Track(atmosConfig, "exec.ProcessYAMLConfigFileWithContext")()

	var stackConfigs []map[string]any
	relativeFilePath := u.TrimBasePathFromPath(basePath+"/", filePath)

	// Initialize or update merge context with current file.
	if mergeContext == nil {
		mergeContext = m.NewMergeContext()
		// Enable provenance if configured.
		if atmosConfig != nil && atmosConfig.TrackProvenance {
			mergeContext.EnableProvenance()
		}
	}
	mergeContext = mergeContext.WithFile(relativeFilePath)

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

	// Process `Go` templates in the imported stack manifest if it has a template extension
	// Files with .yaml.tmpl or .yml.tmpl extensions are always processed as templates
	// Other .tmpl files are processed only when context is provided (backward compatibility)
	// https://atmos.tools/core-concepts/stacks/imports#go-templates-in-imports
	if !skipTemplatesProcessingInImports && (u.IsTemplateFile(filePath) || len(context) > 0) { //nolint:nestif // Template processing error handling requires conditional formatting based on context
		var tmplErr error
		stackManifestTemplatesProcessed, tmplErr = ProcessTmpl(atmosConfig, relativeFilePath, stackYamlConfig, context, ignoreMissingTemplateValues)
		if tmplErr != nil {
			if atmosConfig.Logs.Level == u.LogLevelTrace || atmosConfig.Logs.Level == u.LogLevelDebug {
				stackManifestTemplatesErrorMessage = fmt.Sprintf("\n\n%s", stackYamlConfig)
			}
			wrappedErr := fmt.Errorf("%w: %v", errUtils.ErrInvalidStackManifest, tmplErr)
			if mergeContext != nil {
				return nil, nil, nil, nil, nil, nil, nil, mergeContext.FormatError(wrappedErr, fmt.Sprintf("stack manifest '%s'%s", relativeFilePath, stackManifestTemplatesErrorMessage))
			}
			return nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("%w: stack manifest '%s'\n%v%s", errUtils.ErrInvalidStackManifest, relativeFilePath, tmplErr, stackManifestTemplatesErrorMessage)
		}
	}

	stackConfigMap, positions, err := u.UnmarshalYAMLFromFileWithPositions[schema.AtmosSectionMapType](atmosConfig, stackManifestTemplatesProcessed, filePath)
	if err != nil {
		if atmosConfig.Logs.Level == u.LogLevelTrace || atmosConfig.Logs.Level == u.LogLevelDebug {
			stackManifestTemplatesErrorMessage = fmt.Sprintf("\n\n%s", stackYamlConfig)
		}
		// Check if we have merge context to provide enhanced error formatting
		if mergeContext != nil {
			// Wrap the error with the sentinel first to preserve it
			wrappedErr := fmt.Errorf("%w: %v", errUtils.ErrInvalidStackManifest, err)
			// Then format it with context information
			e := mergeContext.FormatError(wrappedErr, fmt.Sprintf("stack manifest '%s'%s", relativeFilePath, stackManifestTemplatesErrorMessage))
			return nil, nil, nil, nil, nil, nil, nil, e
		} else {
			e := fmt.Errorf("%w: stack manifest '%s'\n%v%s", errUtils.ErrInvalidStackManifest, relativeFilePath, err, stackManifestTemplatesErrorMessage)
			return nil, nil, nil, nil, nil, nil, nil, e
		}
	}

	// Enable provenance tracking in merge context if tracking is enabled
	if atmosConfig.TrackProvenance && mergeContext != nil && len(positions) > 0 {
		mergeContext.EnableProvenance()
		mergeContext.Positions = positions // Store positions for merge operations
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

	parentTerraformOverridesInline, err = m.MergeWithContext(
		atmosConfig,
		[]map[string]any{globalOverrides, terraformOverrides, parentTerraformOverridesInline},
		mergeContext,
	)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, err
	}

	parentHelmfileOverridesInline, err = m.MergeWithContext(
		atmosConfig,
		[]map[string]any{globalOverrides, helmfileOverrides, parentHelmfileOverridesInline},
		mergeContext,
	)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, err
	}

	// Find and process all imports
	importStructs, err := ProcessImportSection(stackConfigMap, relativeFilePath)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, err
	}

	// Record provenance for each import if provenance tracking is enabled.
	// Use the import path as the key so we can look it up later when building the final array.
	if atmosConfig.TrackProvenance && mergeContext != nil && mergeContext.IsProvenanceEnabled() && len(importStructs) > 0 {
		for i, importStruct := range importStructs {
			// Look up position for this import array element.
			arrayPath := fmt.Sprintf("import[%d]", i)
			if pos, exists := positions[arrayPath]; exists {
				// Get depth from merge context using the dedicated method.
				depth := mergeContext.GetImportDepth()

				entry := m.ProvenanceEntry{
					File:   relativeFilePath,
					Line:   pos.Line,
					Column: pos.Column,
					Type:   mergeContext.GetProvenanceType(),
					Depth:  depth,
				}

				// Store provenance using a special key format that includes the import path.
				// This allows us to look it up later when building the final flattened array.
				// Format: "__import__:<import-path>" (e.g., "__import__:mixins/region/us-east-2")
				importKey := fmt.Sprintf("__import__:%s", importStruct.Path)

				// Only record if not already recorded (first occurrence wins).
				if !mergeContext.HasProvenance(importKey) {
					mergeContext.RecordProvenance(importKey, entry)
				}
			}
		}
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
				isGolangTemplate, err2 := IsGolangTemplate(atmosConfig, imp)
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
		mergedContext, err := m.MergeWithContext(atmosConfig, listOfMaps, mergeContext)
		if err != nil {
			return nil, nil, nil, nil, nil, nil, nil, err
		}

		// Process the imports in the current manifest
		for _, importFile := range importMatches {
			yamlConfig,
				_,
				yamlConfigRaw,
				terraformOverridesInline,
				terraformOverridesImports,
				helmfileOverridesInline,
				helmfileOverridesImports, err2 := ProcessYAMLConfigFileWithContext(
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
				mergeContext,
			)
			if err2 != nil {
				return nil, nil, nil, nil, nil, nil, nil, err2
			}

			// From the imported manifest, get the `overrides` sections and merge them with the parent `overrides` section.
			// The inline `overrides` section takes precedence over the imported `overrides` section inside the imported manifest.
			parentTerraformOverridesImports, err = m.MergeWithContext(
				atmosConfig,
				[]map[string]any{parentTerraformOverridesImports, terraformOverridesImports, terraformOverridesInline},
				mergeContext,
			)
			if err != nil {
				return nil, nil, nil, nil, nil, nil, nil, err
			}

			// From the imported manifest, get the `overrides` sections and merge them with the parent `overrides` section.
			// The inline `overrides` section takes precedence over the imported `overrides` section inside the imported manifest.
			parentHelmfileOverridesImports, err = m.MergeWithContext(
				atmosConfig,
				[]map[string]any{parentHelmfileOverridesImports, helmfileOverridesImports, helmfileOverridesInline},
				mergeContext,
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

			// Record metadata for this import.
			// We record every time we encounter an import to track all files that import it,
			// but we use the path as a unique key so only the first entry is kept per import path.
			if atmosConfig.TrackProvenance && mergeContext != nil && mergeContext.IsProvenanceEnabled() {
				// Get depth from merge context using the dedicated method.
				depth := mergeContext.GetImportDepth()

				// Store metadata using special key format: "__import_meta__:<import-path>".
				// Note: We don't have line number info here since this is during recursive processing,
				// not YAML parsing. We'll use line 1 as a placeholder.
				metaKey := fmt.Sprintf("__import_meta__:%s", importRelativePathWithoutExt)

				// Only record if not already recorded (first occurrence wins for the metadata)
				if !mergeContext.HasProvenance(metaKey) {
					mergeContext.RecordProvenance(metaKey, m.ProvenanceEntry{
						File:   mergeContext.CurrentFile, // The file that's importing this file
						Line:   1,                        // Placeholder - we don't have exact line info here
						Column: 1,
						Type:   mergeContext.GetProvenanceType(),
						Depth:  depth,
					})
				}
			}

			importsConfig[importRelativePathWithoutExt] = yamlConfigRaw
		}
	}

	// Terraform `overrides`
	finalTerraformOverrides, err = m.MergeWithContext(
		atmosConfig,
		[]map[string]any{parentTerraformOverridesImports, parentTerraformOverridesInline},
		mergeContext,
	)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, err
	}

	// Helmfile `overrides`
	finalHelmfileOverrides, err = m.MergeWithContext(
		atmosConfig,
		[]map[string]any{parentHelmfileOverridesImports, parentHelmfileOverridesInline},
		mergeContext,
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
	stackConfigsDeepMerged, err := m.MergeWithContext(atmosConfig, stackConfigs, mergeContext)
	if err != nil {
		// The error already contains context information from MergeWithContext
		return nil, nil, nil, nil, nil, nil, nil, err
	}

	// NOTE: We don't store merge context here because ProcessYAMLConfigFileWithContext
	// can be called from parallel goroutines in ProcessYAMLConfigFiles, which would create
	// a race condition. Instead, the caller should store the merge context if needed.

	return stackConfigsDeepMerged,
		importsConfig,
		stackConfigMap,
		parentTerraformOverridesInline,
		parentTerraformOverridesImports,
		parentHelmfileOverridesInline,
		parentHelmfileOverridesImports,
		nil
}

// ProcessStackConfig takes a stack manifest, deep-merges all variables, settings, environments and backends, and returns the final stack configuration for all Terraform/Helmfile/Packer components.
func ProcessStackConfig(
	atmosConfig *schema.AtmosConfiguration,
	stacksBasePath string,
	terraformComponentsBasePath string,
	helmfileComponentsBasePath string,
	packerComponentsBasePath string,
	stack string,
	config map[string]any,
	processStackDeps bool,
	processComponentDeps bool,
	componentTypeFilter string,
	componentStackMap map[string]map[string][]string,
	importsConfig map[string]map[string]any,
	checkBaseComponentExists bool,
) (map[string]any, error) {
	defer perf.Track(atmosConfig, "exec.ProcessStackConfig")()

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
	globalPackerSection := map[string]any{}
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

	packerVars := map[string]any{}
	packerSettings := map[string]any{}
	packerEnv := map[string]any{}
	packerCommand := ""

	terraformComponents := map[string]any{}
	helmfileComponents := map[string]any{}
	packerComponents := map[string]any{}
	allComponents := map[string]any{}

	// Global sections
	if i, ok := config[cfg.VarsSectionName]; ok {
		globalVarsSection, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'vars' section in the file '%s'", stackName)
		}
	}

	if i, ok := config[cfg.HooksSectionName]; ok {
		globalHooksSection, ok = i.(map[string]any)
		if !ok {
			return nil, errors.Wrapf(errUtils.ErrInvalidHooksSection, " '%s'", stackName)
		}
	}

	if i, ok := config[cfg.SettingsSectionName]; ok {
		globalSettingsSection, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'settings' section in the file '%s'", stackName)
		}
	}

	if i, ok := config[cfg.EnvSectionName]; ok {
		globalEnvSection, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'env' section in the file '%s'", stackName)
		}
	}

	if i, ok := config[cfg.TerraformSectionName]; ok {
		globalTerraformSection, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'terraform' section in the file '%s'", stackName)
		}
	}

	if i, ok := config[cfg.HelmfileSectionName]; ok {
		globalHelmfileSection, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'helmfile' section in the file '%s'", stackName)
		}
	}

	if i, ok := config[cfg.PackerSectionName]; ok {
		globalPackerSection, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'packer' section in the file '%s'", stackName)
		}
	}

	if i, ok := config[cfg.ComponentsSectionName]; ok {
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

	if i, ok := globalTerraformSection[cfg.VarsSectionName]; ok {
		terraformVars, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'terraform.vars' section in the file '%s'", stackName)
		}
	}

	if i, ok := globalTerraformSection[cfg.HooksSectionName]; ok {
		terraformHooks, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%w '%s'", errUtils.ErrInvalidTerraformHooksSection, stackName)
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

	if i, ok := globalTerraformSection[cfg.SettingsSectionName]; ok {
		terraformSettings, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'terraform.settings' section in the file '%s'", stackName)
		}
	}

	globalAndTerraformSettings, err := m.Merge(atmosConfig, []map[string]any{globalSettingsSection, terraformSettings})
	if err != nil {
		return nil, err
	}

	if i, ok := globalTerraformSection[cfg.EnvSectionName]; ok {
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

	if i, ok := globalTerraformSection[cfg.BackendTypeSectionName]; ok {
		globalBackendType, ok = i.(string)
		if !ok {
			return nil, fmt.Errorf("invalid 'terraform.backend_type' section in the file '%s'", stackName)
		}
	}

	if i, ok := globalTerraformSection[cfg.BackendSectionName]; ok {
		globalBackendSection, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'terraform.backend' section in the file '%s'", stackName)
		}
	}

	// Global remote state backend
	globalRemoteStateBackendType := ""
	globalRemoteStateBackendSection := map[string]any{}

	if i, ok := globalTerraformSection[cfg.RemoteStateBackendTypeSectionName]; ok {
		globalRemoteStateBackendType, ok = i.(string)
		if !ok {
			return nil, fmt.Errorf("invalid 'terraform.remote_state_backend_type' section in the file '%s'", stackName)
		}
	}

	if i, ok := globalTerraformSection[cfg.RemoteStateBackendSectionName]; ok {
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

	if i, ok := globalHelmfileSection[cfg.VarsSectionName]; ok {
		helmfileVars, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'helmfile.vars' section in the file '%s'", stackName)
		}
	}

	globalAndHelmfileVars, err := m.Merge(atmosConfig, []map[string]any{globalVarsSection, helmfileVars})
	if err != nil {
		return nil, err
	}

	if i, ok := globalHelmfileSection[cfg.SettingsSectionName]; ok {
		helmfileSettings, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'helmfile.settings' section in the file '%s'", stackName)
		}
	}

	globalAndHelmfileSettings, err := m.Merge(atmosConfig, []map[string]any{globalSettingsSection, helmfileSettings})
	if err != nil {
		return nil, err
	}

	if i, ok := globalHelmfileSection[cfg.EnvSectionName]; ok {
		helmfileEnv, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'helmfile.env' section in the file '%s'", stackName)
		}
	}

	globalAndHelmfileEnv, err := m.Merge(atmosConfig, []map[string]any{globalEnvSection, helmfileEnv})
	if err != nil {
		return nil, err
	}

	// Packer section
	if i, ok := globalPackerSection[cfg.CommandSectionName]; ok {
		packerCommand, ok = i.(string)
		if !ok {
			return nil, fmt.Errorf("invalid 'packer.command' section in the file '%s'", stackName)
		}
	}

	if i, ok := globalPackerSection[cfg.VarsSectionName]; ok {
		packerVars, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'packer.vars' section in the file '%s'", stackName)
		}
	}

	globalAndPackerVars, err := m.Merge(atmosConfig, []map[string]any{globalVarsSection, packerVars})
	if err != nil {
		return nil, err
	}

	if i, ok := globalPackerSection[cfg.SettingsSectionName]; ok {
		packerSettings, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'packer.settings' section in the file '%s'", stackName)
		}
	}

	globalAndPackerSettings, err := m.Merge(atmosConfig, []map[string]any{globalSettingsSection, packerSettings})
	if err != nil {
		return nil, err
	}

	if i, ok := globalPackerSection[cfg.EnvSectionName]; ok {
		packerEnv, ok = i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'packer.env' section in the file '%s'", stackName)
		}
	}

	globalAndPackerEnv, err := m.Merge(atmosConfig, []map[string]any{globalEnvSection, packerEnv})
	if err != nil {
		return nil, err
	}

	// Process all Terraform components
	if componentTypeFilter == "" || componentTypeFilter == cfg.TerraformComponentType {
		if allTerraformComponents, ok := globalComponentsSection[cfg.TerraformComponentType]; ok {

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

				// Process component using helper function.
				opts := ComponentProcessorOptions{
					ComponentType:                   cfg.TerraformComponentType,
					Component:                       component,
					Stack:                           stack,
					StackName:                       stackName,
					ComponentMap:                    componentMap,
					AllComponentsMap:                allTerraformComponentsMap,
					ComponentsBasePath:              terraformComponentsBasePath,
					CheckBaseComponentExists:        checkBaseComponentExists,
					GlobalVars:                      globalAndTerraformVars,
					GlobalSettings:                  globalAndTerraformSettings,
					GlobalEnv:                       globalAndTerraformEnv,
					GlobalCommand:                   terraformCommand,
					TerraformProviders:              terraformProviders,
					GlobalAndTerraformHooks:         globalAndTerraformHooks,
					GlobalBackendType:               globalBackendType,
					GlobalBackendSection:            globalBackendSection,
					GlobalRemoteStateBackendType:    globalRemoteStateBackendType,
					GlobalRemoteStateBackendSection: globalRemoteStateBackendSection,
					AtmosConfig:                     atmosConfig,
				}

				result, err := processComponent(opts)
				if err != nil {
					return nil, err
				}

				// Merge component configurations.
				comp, err := mergeComponentConfigurations(atmosConfig, opts, result)
				if err != nil {
					return nil, err
				}

				terraformComponents[component] = comp
			}
		}
	}

	// Process all Helmfile components
	if componentTypeFilter == "" || componentTypeFilter == cfg.HelmfileComponentType {
		if allHelmfileComponents, ok := globalComponentsSection[cfg.HelmfileComponentType]; ok {

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

				// Process component using helper function.
				opts := ComponentProcessorOptions{
					ComponentType:            cfg.HelmfileComponentType,
					Component:                component,
					Stack:                    stack,
					StackName:                stackName,
					ComponentMap:             componentMap,
					AllComponentsMap:         allHelmfileComponentsMap,
					ComponentsBasePath:       helmfileComponentsBasePath,
					CheckBaseComponentExists: checkBaseComponentExists,
					GlobalVars:               globalAndHelmfileVars,
					GlobalSettings:           globalAndHelmfileSettings,
					GlobalEnv:                globalAndHelmfileEnv,
					GlobalCommand:            helmfileCommand,
					AtmosConfig:              atmosConfig,
				}

				result, err := processComponent(opts)
				if err != nil {
					return nil, err
				}

				// Merge component configurations.
				comp, err := mergeComponentConfigurations(atmosConfig, opts, result)
				if err != nil {
					return nil, err
				}

				helmfileComponents[component] = comp
			}
		}
	}

	// Process all Packer components
	if componentTypeFilter == "" || componentTypeFilter == cfg.PackerComponentType {
		if allPackerComponents, ok := globalComponentsSection[cfg.PackerComponentType]; ok {

			allPackerComponentsMap, ok := allPackerComponents.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid 'components.packer' section in the file '%s'", stackName)
			}

			for cmp, v := range allPackerComponentsMap {
				component := cmp

				componentMap, ok := v.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("invalid 'components.packer.%s' section in the file '%s'", component, stackName)
				}

				// Process component using helper function.
				opts := ComponentProcessorOptions{
					ComponentType:            cfg.PackerComponentType,
					Component:                component,
					Stack:                    stack,
					StackName:                stackName,
					ComponentMap:             componentMap,
					AllComponentsMap:         allPackerComponentsMap,
					ComponentsBasePath:       packerComponentsBasePath,
					CheckBaseComponentExists: checkBaseComponentExists,
					GlobalVars:               globalAndPackerVars,
					GlobalSettings:           globalAndPackerSettings,
					GlobalEnv:                globalAndPackerEnv,
					GlobalCommand:            packerCommand,
					AtmosConfig:              atmosConfig,
				}

				result, err := processComponent(opts)
				if err != nil {
					return nil, err
				}

				// Merge component configurations.
				comp, err := mergeComponentConfigurations(atmosConfig, opts, result)
				if err != nil {
					return nil, err
				}

				packerComponents[component] = comp
			}
		}
	}

	allComponents[cfg.TerraformComponentType] = terraformComponents
	allComponents[cfg.HelmfileComponentType] = helmfileComponents
	allComponents[cfg.PackerComponentType] = packerComponents

	result := map[string]any{
		cfg.ComponentsSectionName: allComponents,
	}

	return result, nil
}

// processSettingsIntegrationsGithub deep-merges the `settings.integrations.github` section from stack manifests with the `integrations.github` section from `atmos.yaml`.
func processSettingsIntegrationsGithub(atmosConfig *schema.AtmosConfiguration, settings map[string]any) (map[string]any, error) {
	defer perf.Track(atmosConfig, "exec.processSettingsIntegrationsGithub")()

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

// processAuthConfig merges the component `auth` section with global `auth` from atmos.yaml.
// Component-level config takes precedence over global config.
func processAuthConfig(atmosConfig *schema.AtmosConfiguration, authConfig map[string]any) (map[string]any, error) {
	// Convert the global auth config struct to map[string]any for merging.
	var globalAuthConfig map[string]any
	if err := mapstructure.Decode(atmosConfig.Auth, &globalAuthConfig); err != nil {
		return nil, fmt.Errorf("%w: failed to convert global auth config to map: %v", errUtils.ErrInvalidAuthConfig, err)
	}

	mergedAuthConfig, err := m.Merge(
		atmosConfig,
		[]map[string]any{
			globalAuthConfig,
			authConfig,
		})
	if err != nil {
		return nil, fmt.Errorf("%w: merge auth config: %v", errUtils.ErrInvalidAuthConfig, err)
	}

	return mergedAuthConfig, nil
}

// FindComponentStacks finds all infrastructure stack manifests where the component or the base component is defined.
func FindComponentStacks(
	componentType string,
	component string,
	baseComponent string,
	componentStackMap map[string]map[string][]string,
) ([]string, error) {
	defer perf.Track(nil, "exec.FindComponentStacks")()

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

// FindComponentDependenciesLegacy finds all imports where the component or the base component(s) are defined.
// Component depends on the imported config file if any of the following conditions is true:
//  1. The imported config file has any of the global `backend`, `backend_type`, `env`, `remote_state_backend`, `remote_state_backend_type`,
//     `settings` or `vars` sections which are not empty.
//  2. The imported config file has the component type section, which has any of the `backend`, `backend_type`, `env`, `remote_state_backend`,
//     `remote_state_backend_type`, `settings` or `vars` sections which are not empty.
//  3. The imported config file has the cfg.ComponentsSectionName section, which has the component type section, which has the component section.
//  4. The imported config file has the cfg.ComponentsSectionName section, which has the component type section, which has the base component(s) section,
//     and the base component section is defined inline (not imported).
func FindComponentDependenciesLegacy(
	stack string,
	componentType string,
	component string,
	baseComponents []string,
	stackImports map[string]map[string]any,
) ([]string, error) {
	defer perf.Track(nil, "exec.FindComponentDependenciesLegacy")()

	var deps []string

	sectionsToCheck := []string{
		cfg.BackendSectionName,
		cfg.BackendTypeSectionName,
		cfg.EnvSectionName,
		cfg.RemoteStateBackendSectionName,
		cfg.RemoteStateBackendTypeSectionName,
		cfg.SettingsSectionName,
		cfg.VarsSectionName,
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

		stackImportMapComponentsSection, ok := stackImportMap[cfg.ComponentsSectionName].(map[string]any)
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

				importOfStackImportComponentsSection, ok := importOfStackImportMap[cfg.ComponentsSectionName].(map[string]any)
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

// ProcessImportSection processes the `import` section in stack manifests.
// The `import` section can contain:
// 1. Project-relative paths (e.g. "mixins/region/us-east-2")
// 2. Paths relative to the current stack file (e.g. "./_defaults")
// 3. StackImport structs containing either of the above path types (e.g. "path: mixins/region/us-east-2").
func ProcessImportSection(stackMap map[string]any, filePath string) ([]schema.StackImport, error) {
	defer perf.Track(nil, "exec.ProcessImportSection")()

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

// GetFileContent tries to read and return the file content from the sync map if it exists in the map,
// otherwise it reads the file, stores its content in the map and returns the content.
func GetFileContent(filePath string) (string, error) {
	defer perf.Track(nil, "exec.GetFileContent")()

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

// ProcessBaseComponentConfig processes base component(s) config.
func ProcessBaseComponentConfig(
	atmosConfig *schema.AtmosConfiguration,
	baseComponentConfig *schema.BaseComponentConfig,
	allComponentsMap map[string]any,
	component string,
	stack string,
	baseComponent string,
	componentBasePath string,
	checkBaseComponentExists bool,
	baseComponents *[]string,
) error {
	defer perf.Track(atmosConfig, "exec.ProcessBaseComponentConfig")()

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

			if inheritList, inheritListExist := componentMetadata[cfg.InheritsSectionName].([]any); inheritListExist {
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

		if baseComponentVarsSection, baseComponentVarsSectionExist := baseComponentMap[cfg.VarsSectionName]; baseComponentVarsSectionExist {
			baseComponentVars, ok = baseComponentVarsSection.(map[string]any)
			if !ok {
				return fmt.Errorf("invalid '%s.vars' section in the stack '%s'", baseComponent, stack)
			}
		}

		if baseComponentSettingsSection, baseComponentSettingsSectionExist := baseComponentMap[cfg.SettingsSectionName]; baseComponentSettingsSectionExist {
			baseComponentSettings, ok = baseComponentSettingsSection.(map[string]any)
			if !ok {
				return fmt.Errorf("invalid '%s.settings' section in the stack '%s'", baseComponent, stack)
			}
		}

		if baseComponentEnvSection, baseComponentEnvSectionExist := baseComponentMap[cfg.EnvSectionName]; baseComponentEnvSectionExist {
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
		if i, ok2 := baseComponentMap[cfg.BackendTypeSectionName]; ok2 {
			baseComponentBackendType, ok = i.(string)
			if !ok {
				return fmt.Errorf("invalid '%s.backend_type' section in the stack '%s'", baseComponent, stack)
			}
		}

		if i, ok2 := baseComponentMap[cfg.BackendSectionName]; ok2 {
			baseComponentBackendSection, ok = i.(map[string]any)
			if !ok {
				return fmt.Errorf("invalid '%s.backend' section in the stack '%s'", baseComponent, stack)
			}
		}

		// Base component remote state backend
		if i, ok2 := baseComponentMap[cfg.RemoteStateBackendTypeSectionName]; ok2 {
			baseComponentRemoteStateBackendType, ok = i.(string)
			if !ok {
				return fmt.Errorf("invalid '%s.remote_state_backend_type' section in the stack '%s'", baseComponent, stack)
			}
		}

		if i, ok2 := baseComponentMap[cfg.RemoteStateBackendSectionName]; ok2 {
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

// FindComponentsDerivedFromBaseComponents finds all components that derive from the given base components.
func FindComponentsDerivedFromBaseComponents(
	stack string,
	allComponents map[string]any,
	baseComponents []string,
) ([]string, error) {
	defer perf.Track(nil, "exec.FindComponentsDerivedFromBaseComponents")()

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
