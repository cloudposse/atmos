package vendor

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/samber/lo"
	"go.yaml.in/yaml/v3"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// VendorConfigResult holds the result of reading a vendor config file.
type VendorConfigResult struct {
	Config   schema.AtmosVendorConfig
	Found    bool
	FilePath string
}

// ReadAndProcessVendorConfigFile reads and processes the Atmos vendoring config file `vendor.yaml`.
func ReadAndProcessVendorConfigFile(
	atmosConfig *schema.AtmosConfiguration,
	vendorConfigFile string,
	checkGlobalConfig bool,
) (VendorConfigResult, error) {
	result := VendorConfigResult{}
	result.Config.Spec.Sources = []schema.AtmosVendorSource{} // Initialize empty sources slice.

	// Determine the vendor config file path.
	result.FilePath = resolveVendorConfigFilePath(atmosConfig, vendorConfigFile, checkGlobalConfig)
	if result.FilePath == "" {
		log.Debug("Vendor config file not found", "file", vendorConfigFile)
		return result, nil
	}

	// Validate and process the vendor config file or directory.
	configFiles, err := getConfigFiles(result.FilePath)
	if err != nil {
		return result, err
	}

	// Merge all config files into a single vendor configuration.
	result.Config, err = mergeVendorConfigFiles(configFiles)
	if err != nil {
		return result, err
	}

	result.Found = true
	return result, nil
}

// resolveVendorConfigFilePath resolves the vendor config file path.
func resolveVendorConfigFilePath(atmosConfig *schema.AtmosConfiguration, vendorConfigFile string, checkGlobalConfig bool) string {
	if checkGlobalConfig && atmosConfig.Vendor.BasePath != "" {
		if !filepath.IsAbs(atmosConfig.Vendor.BasePath) {
			return filepath.Join(atmosConfig.BasePath, atmosConfig.Vendor.BasePath)
		}
		return atmosConfig.Vendor.BasePath
	}

	// Search for the vendor config file
	foundVendorConfigFile, fileExists := u.SearchConfigFile(vendorConfigFile)
	if !fileExists {
		pathToVendorConfig := filepath.Join(atmosConfig.BasePath, vendorConfigFile)
		foundVendorConfigFile, fileExists = u.SearchConfigFile(pathToVendorConfig)
		if !fileExists {
			return "" // File does not exist, but this is not an error
		}
	}
	return foundVendorConfigFile
}

// getConfigFiles returns config files from a path (file or directory).
func getConfigFiles(path string) ([]string, error) {
	fileInfo, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errUtils.Build(errUtils.ErrVendoringNotConfigured).
				WithHint("To set up vendoring, please see https://atmos.tools/core-concepts/vendor/").
				Err()
		}
		if os.IsPermission(err) {
			return nil, fmt.Errorf("%w '%s'. Please check the file permissions", ErrPermissionDenied, path)
		}
		return nil, fmt.Errorf("an error occurred while accessing the vendoring configuration: %w", err)
	}

	if fileInfo.IsDir() {
		path = filepath.ToSlash(path)
		matches, err := doublestar.Glob(os.DirFS(path), "*.{yaml,yml}")
		if err != nil {
			return nil, err
		}

		if len(matches) == 0 {
			return nil, fmt.Errorf("%w '%s'", ErrNoYAMLConfigFiles, path)
		}
		for i, match := range matches {
			matches[i] = filepath.Join(path, match)
		}
		sort.Strings(matches)
		return matches, nil
	}
	return []string{path}, nil
}

// mergeVendorConfigFiles merges multiple config files into a single vendor configuration.
func mergeVendorConfigFiles(configFiles []string) (schema.AtmosVendorConfig, error) {
	var vendorConfig schema.AtmosVendorConfig
	sourceMap := make(map[string]bool) // Track unique sources by component name
	importMap := make(map[string]bool) // Track unique imports

	for _, configFile := range configFiles {
		var currentConfig schema.AtmosVendorConfig
		yamlFile, err := os.ReadFile(configFile)
		if err != nil {
			return vendorConfig, err
		}
		if err := yaml.Unmarshal(yamlFile, &currentConfig); err != nil {
			return vendorConfig, err
		}

		// Merge sources, checking for duplicates
		for i := range currentConfig.Spec.Sources {
			source := currentConfig.Spec.Sources[i]
			if source.Component != "" {
				if sourceMap[source.Component] {
					return vendorConfig, fmt.Errorf("%w '%s' found in config file '%s'", ErrDuplicateComponentsFound, source.Component, configFile)
				}
				sourceMap[source.Component] = true
			}
			vendorConfig.Spec.Sources = append(vendorConfig.Spec.Sources, source)
		}

		// Merge imports, checking for duplicates
		for _, imp := range currentConfig.Spec.Imports {
			if !importMap[imp] {
				importMap[imp] = true
				vendorConfig.Spec.Imports = append(vendorConfig.Spec.Imports, imp)
			}
		}
	}
	return vendorConfig, nil
}

// processVendorImports processes all imports recursively and returns a list of sources.
func processVendorImports(
	atmosConfig *schema.AtmosConfiguration,
	vendorConfigFile string,
	imports []string,
	sources []schema.AtmosVendorSource,
	allImports []string,
) ([]schema.AtmosVendorSource, []string, error) {
	var mergedSources []schema.AtmosVendorSource
	for _, imp := range imports {
		if u.SliceContainsString(allImports, imp) {
			return nil, nil, fmt.Errorf("%w '%s' in the vendor config file '%s'. It was already imported in the import chain",
				ErrDuplicateImport,
				imp,
				vendorConfigFile,
			)
		}

		allImports = append(allImports, imp)

		vendorResult, err := ReadAndProcessVendorConfigFile(atmosConfig, imp, false)
		if err != nil {
			return nil, nil, err
		}

		if u.SliceContainsString(vendorResult.Config.Spec.Imports, imp) {
			return nil, nil, fmt.Errorf("%w file '%s'", ErrVendorConfigSelfImport, imp)
		}

		if len(vendorResult.Config.Spec.Sources) == 0 && len(vendorResult.Config.Spec.Imports) == 0 {
			return nil, nil, fmt.Errorf("%w '%s'", ErrMissingVendorConfigDefinition, imp)
		}

		mergedSources, allImports, err = processVendorImports(atmosConfig, imp, vendorResult.Config.Spec.Imports, mergedSources, allImports)
		if err != nil {
			return nil, nil, err
		}

		for i := range vendorResult.Config.Spec.Sources {
			vendorResult.Config.Spec.Sources[i].File = imp
		}

		mergedSources = append(mergedSources, vendorResult.Config.Spec.Sources...)
	}

	return append(mergedSources, sources...), allImports, nil
}

// logInitialMessage logs the initial message for vendoring.
func logInitialMessage(vendorConfigFileName string, tags []string) {
	logMessage := fmt.Sprintf("Vendoring from '%s'", vendorConfigFileName)
	if len(tags) > 0 {
		logMessage = fmt.Sprintf("%s for tags {%s}", logMessage, strings.Join(tags, ", "))
	}
	log.Info(logMessage)
}

// validateSourceFields validates required fields in a vendor source.
func validateSourceFields(s *schema.AtmosVendorSource, vendorConfigFileName string) error {
	// Ensure necessary fields are present
	if s.File == "" {
		s.File = vendorConfigFileName
	}
	if s.Source == "" {
		return fmt.Errorf("%w `%s`", ErrSourceMissing, s.File)
	}
	if len(s.Targets) == 0 {
		return fmt.Errorf("%w for source '%s' in file '%s'", ErrTargetsMissing, s.Source, s.File)
	}
	return nil
}

// shouldSkipSource determines if a source should be skipped based on component and tags filters.
func shouldSkipSource(s *schema.AtmosVendorSource, component string, tags []string) bool {
	// Skip if component or tags do not match
	// If `--component` is specified, and it's not equal to this component, skip this component
	// If `--tags` list is specified, and it does not contain any tags defined in this component, skip this component.
	return (component != "" && s.Component != component) || (len(tags) > 0 && len(lo.Intersect(tags, s.Tags)) == 0)
}

// validateTagsAndComponents validates tags and components in the vendor config.
func validateTagsAndComponents(
	sources []schema.AtmosVendorSource,
	vendorConfigFileName string,
	component string,
	tags []string,
) error {
	if len(tags) > 0 {
		componentTags := lo.FlatMap(sources, func(s schema.AtmosVendorSource, _ int) []string {
			return s.Tags
		})

		if len(lo.Intersect(tags, componentTags)) == 0 {
			return fmt.Errorf("%w '%s' tagged with the tags %v",
				ErrNoComponentsWithTags, vendorConfigFileName, tags)
		}
	}

	components := lo.FilterMap(sources, func(s schema.AtmosVendorSource, _ int) (string, bool) {
		return s.Component, s.Component != ""
	})

	if duplicates := lo.FindDuplicates(components); len(duplicates) > 0 {
		return fmt.Errorf("%w %v in the vendor config file '%s' and the imports",
			ErrDuplicateComponents, duplicates, vendorConfigFileName)
	}

	if component != "" && !u.SliceContainsString(components, component) {
		return fmt.Errorf("%w component '%s', file '%s'",
			ErrComponentNotDefined, component, vendorConfigFileName)
	}

	return nil
}

// resolveComponentBasePath returns the base path for a component type.
func resolveComponentBasePath(atmosConfig *schema.AtmosConfiguration, componentType string) (string, error) {
	switch componentType {
	case cfg.TerraformComponentType:
		return atmosConfig.Components.Terraform.BasePath, nil
	case cfg.HelmfileComponentType:
		return atmosConfig.Components.Helmfile.BasePath, nil
	case cfg.PackerComponentType:
		return atmosConfig.Components.Packer.BasePath, nil
	default:
		return "", fmt.Errorf("%w: %s", errUtils.ErrUnsupportedComponentType, componentType)
	}
}

// loadComponentConfig reads and unmarshals the component config file.
func loadComponentConfig(componentConfigFile string) (schema.VendorComponentConfig, error) {
	var componentConfig schema.VendorComponentConfig
	componentConfigFileContent, err := os.ReadFile(componentConfigFile)
	if err != nil {
		return componentConfig, err
	}
	componentConfig, err = u.UnmarshalYAML[schema.VendorComponentConfig](string(componentConfigFileContent))
	if err != nil {
		return componentConfig, err
	}
	if componentConfig.Kind != "ComponentVendorConfig" {
		return componentConfig, fmt.Errorf("%w: '%s' in file '%s'", ErrInvalidComponentKind, componentConfig.Kind, cfg.ComponentVendorConfigFileName)
	}
	return componentConfig, nil
}

// ReadAndProcessComponentVendorConfigFile reads and processes the component vendoring config file `component.yaml`.
func ReadAndProcessComponentVendorConfigFile(
	atmosConfig *schema.AtmosConfiguration,
	component string,
	componentType string,
) (schema.VendorComponentConfig, string, error) {
	var componentConfig schema.VendorComponentConfig

	componentBasePath, err := resolveComponentBasePath(atmosConfig, componentType)
	if err != nil {
		return componentConfig, "", err
	}

	componentPath := filepath.Join(atmosConfig.BasePath, componentBasePath, component)
	dirExists, err := u.IsDirectory(componentPath)
	if err != nil {
		return componentConfig, "", err
	}
	if !dirExists {
		return componentConfig, "", fmt.Errorf("%w: %s", ErrFolderNotFound, componentPath)
	}

	componentConfigFile, err := findComponentConfigFile(componentPath, strings.TrimSuffix(cfg.ComponentVendorConfigFileName, ".yaml"))
	if err != nil {
		return componentConfig, "", err
	}

	componentConfig, err = loadComponentConfig(componentConfigFile)
	if err != nil {
		return componentConfig, "", err
	}

	return componentConfig, componentPath, nil
}

// findComponentConfigFile identifies the component vendoring config file (`component.yaml` or `component.yml`).
func findComponentConfigFile(basePath, fileName string) (string, error) {
	componentConfigExtensions := []string{"yaml", "yml"}

	for _, ext := range componentConfigExtensions {
		configFilePath := filepath.Join(basePath, fmt.Sprintf("%s.%s", fileName, ext))
		if u.FileExists(configFilePath) {
			return configFilePath, nil
		}
	}
	return "", fmt.Errorf("%w: %s", ErrComponentConfigFileNotFound, basePath)
}
