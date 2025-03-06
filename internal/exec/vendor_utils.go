package exec

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	cp "github.com/otiai10/copy"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

var ErrVendorComponents = errors.New("failed to vendor components")

// ReadAndProcessVendorConfigFile reads and processes the Atmos vendoring config file `vendor.yaml`.
func ReadAndProcessVendorConfigFile(
	atmosConfig schema.AtmosConfiguration,
	vendorConfigFile string,
	checkGlobalConfig bool,
) (schema.AtmosVendorConfig, bool, string, error) {
	var vendorConfig schema.AtmosVendorConfig
	vendorConfig.Spec.Sources = []schema.AtmosVendorSource{} // Initialize empty sources slice

	// Determine the vendor config file path
	foundVendorConfigFile, err := resolveVendorConfigFilePath(atmosConfig, vendorConfigFile, checkGlobalConfig)
	if err != nil {
		return vendorConfig, false, "", err
	}
	if foundVendorConfigFile == "" {
		u.LogWarning(fmt.Sprintf("Vendor config file '%s' does not exist. Proceeding without vendor configurations", vendorConfigFile))
		return vendorConfig, false, "", nil
	}

	// Validate and process the vendor config file or directory
	configFiles, err := getConfigFiles(foundVendorConfigFile)
	if err != nil {
		return vendorConfig, false, "", err
	}

	// Merge all config files into a single vendor configuration
	vendorConfig, err = mergeVendorConfigFiles(configFiles)
	if err != nil {
		return vendorConfig, false, "", err
	}

	return vendorConfig, true, foundVendorConfigFile, nil
}

// Helper function to resolve the vendor config file path
func resolveVendorConfigFilePath(atmosConfig schema.AtmosConfiguration, vendorConfigFile string, checkGlobalConfig bool) (string, error) {
	if checkGlobalConfig && atmosConfig.Vendor.BasePath != "" {
		if !filepath.IsAbs(atmosConfig.Vendor.BasePath) {
			return filepath.Join(atmosConfig.BasePath, atmosConfig.Vendor.BasePath), nil
		}
		return atmosConfig.Vendor.BasePath, nil
	}

	// Search for the vendor config file
	foundVendorConfigFile, fileExists := u.SearchConfigFile(vendorConfigFile)
	if !fileExists {
		pathToVendorConfig := filepath.Join(atmosConfig.BasePath, vendorConfigFile)
		foundVendorConfigFile, fileExists = u.SearchConfigFile(pathToVendorConfig)
		if !fileExists {
			return "", nil // File does not exist, but this is not an error
		}
	}
	return foundVendorConfigFile, nil
}

// Helper function to get config files from a path (file or directory)
func getConfigFiles(path string) ([]string, error) {
	fileInfo, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("vendoring is not configured. To set up vendoring, please see https://atmos.tools/core-concepts/vendor/")
		}
		if os.IsPermission(err) {
			return nil, fmt.Errorf("permission denied when accessing '%s'. Please check the file permissions", path)
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
			return nil, fmt.Errorf("no YAML configuration files found in directory '%s'", path)
		}
		for i, match := range matches {
			matches[i] = filepath.Join(path, match)
		}
		sort.Strings(matches)
		return matches, nil
	}
	return []string{path}, nil
}

// Helper function to merge multiple config files into a single vendor configuration.
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
		for _, source := range currentConfig.Spec.Sources {
			if source.Component != "" {
				if sourceMap[source.Component] {
					return vendorConfig, fmt.Errorf("duplicate component '%s' found in config file '%s'", source.Component, configFile)
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

// ExecuteAtmosVendorInternal downloads the artifacts from the sources and writes them to the targets.
func ExecuteAtmosVendorInternal(
	atmosConfig schema.AtmosConfiguration,
	vendorConfigFileName string,
	atmosVendorSpec schema.AtmosVendorSpec,
	component string,
	tags []string,
	dryRun bool,
) error {
	var err error
	vendorConfigFilePath := filepath.Dir(vendorConfigFileName)

	logInitialMessage(atmosConfig, vendorConfigFileName, tags)

	if len(atmosVendorSpec.Sources) == 0 && len(atmosVendorSpec.Imports) == 0 {
		return fmt.Errorf("either 'spec.sources' or 'spec.imports' (or both) must be defined in the vendor config file '%s'", vendorConfigFileName)
	}

	// Process imports and return all sources from all the imports and from `vendor.yaml`.
	sources, _, err := processVendorImports(
		atmosConfig,
		vendorConfigFileName,
		atmosVendorSpec.Imports,
		atmosVendorSpec.Sources,
		[]string{vendorConfigFileName},
	)
	if err != nil {
		return err
	}

	if len(sources) == 0 {
		return fmt.Errorf("'spec.sources' is empty in the vendor config file '%s' and the imports", vendorConfigFileName)
	}

	if len(tags) > 0 {
		componentTags := lo.FlatMap(sources, func(s schema.AtmosVendorSource, index int) []string {
			return s.Tags
		})

		if len(lo.Intersect(tags, componentTags)) == 0 {
			return fmt.Errorf("there are no components in the vendor config file '%s' tagged with the tags %v", vendorConfigFileName, tags)
		}
	}

	components := lo.FilterMap(sources, func(s schema.AtmosVendorSource, index int) (string, bool) {
		if s.Component != "" {
			return s.Component, true
		}
		return "", false
	})

	duplicateComponents := lo.FindDuplicates(components)

	if len(duplicateComponents) > 0 {
		return fmt.Errorf("duplicate component names %v in the vendor config file '%s' and the imports",
			duplicateComponents,
			vendorConfigFileName,
		)
	}

	if component != "" && !u.SliceContainsString(components, component) {
		return fmt.Errorf("the flag '--component %s' is passed, but the component is not defined in any of the 'sources' in the vendor config file '%s' and the imports",
			component,
			vendorConfigFileName,
		)
	}

	// Allow having duplicate targets in different sources.
	// This can be used to vendor mixins (from local and remote sources) and write them to the same targets.
	// TODO: consider adding a flag to `atmos vendor pull` to specify if duplicate targets are allowed or not.
	//targets := lo.FlatMap(sources, func(s schema.AtmosVendorSource, index int) []string {
	//	return s.Targets
	//})
	//
	//duplicateTargets := lo.FindDuplicates(targets)
	//
	//if len(duplicateTargets) > 0 {
	//	return fmt.Errorf("duplicate targets %v in the vendor config file '%s' and the imports",
	//		duplicateTargets,
	//		vendorConfigFileName,
	//	)
	//}
	packages, err := processAtmosVendorSource(sources, component, tags, vendorConfigFileName, vendorConfigFilePath)
	if err != nil {
		return err
	}
	if len(packages) > 0 {
		return executeVendorModel(packages, dryRun, atmosConfig)
	}
	return nil
}

func processAtmosVendorSource(sources []schema.AtmosVendorSource, component string, tags []string, vendorConfigFileName, vendorConfigFilePath string) ([]pkgAtmosVendor, error) {
	var packages []pkgAtmosVendor
	for indexSource, s := range sources {
		if shouldSkipSource(&s, component, tags) {
			continue
		}

		if err := validateSourceFields(&s, vendorConfigFileName); err != nil {
			return nil, err
		}

		tmplData := struct {
			Component string
			Version   string
		}{s.Component, s.Version}

		// Parse 'source' template
		uri, err := ProcessTmpl(fmt.Sprintf("source-%d", indexSource), s.Source, tmplData, false)
		if err != nil {
			return nil, err
		}

		useOciScheme, useLocalFileSystem, sourceIsLocalFile, err := determineSourceType(&uri, vendorConfigFilePath)
		if err != nil {
			return nil, err
		}
		if !useLocalFileSystem {
			err = ValidateURI(uri)
			if err != nil {
				if strings.Contains(uri, "..") {
					return nil, fmt.Errorf("invalid URI for component %s: %w: Please ensure the source is a valid local path", s.Component, err)
				}
				return nil, fmt.Errorf("invalid URI for component %s: %w", s.Component, err)
			}
		}

		// Determine package type
		pType := determinePackageType(useOciScheme, useLocalFileSystem)

		// Process each target within the source
		pkgs, err := processTargets(indexSource, &s, tmplData, vendorConfigFilePath, uri, pType, sourceIsLocalFile)
		if err != nil {
			return nil, err
		}
		packages = append(packages, pkgs...)

	}

	return packages, nil
}

func determinePackageType(useOciScheme, useLocalFileSystem bool) pkgType {
	if useOciScheme {
		return pkgTypeOci
	} else if useLocalFileSystem {
		return pkgTypeLocal
	}
	return pkgTypeRemote
}

func processTargets(indexSource int, s *schema.AtmosVendorSource, tmplData struct{ Component, Version string }, vendorConfigFilePath, uri string, pType pkgType, sourceIsLocalFile bool) ([]pkgAtmosVendor, error) {
	var packages []pkgAtmosVendor
	for indexTarget, tgt := range s.Targets {
		target, err := ProcessTmpl(fmt.Sprintf("target-%d-%d", indexSource, indexTarget), tgt, tmplData, false)
		if err != nil {
			return nil, err
		}
		targetPath := filepath.Join(filepath.ToSlash(vendorConfigFilePath), filepath.ToSlash(target))
		pkgName := s.Component
		if pkgName == "" {
			pkgName = uri
		}
		// Create package struct
		p := pkgAtmosVendor{
			uri:               uri,
			name:              pkgName,
			targetPath:        targetPath,
			sourceIsLocalFile: sourceIsLocalFile,
			pkgType:           pType,
			version:           s.Version,
			atmosVendorSource: *s,
		}
		packages = append(packages, p)
	}
	return packages, nil
}

// processVendorImports processes all imports recursively and returns a list of sources
func processVendorImports(
	atmosConfig schema.AtmosConfiguration,
	vendorConfigFile string,
	imports []string,
	sources []schema.AtmosVendorSource,
	allImports []string,
) ([]schema.AtmosVendorSource, []string, error) {
	var mergedSources []schema.AtmosVendorSource

	for _, imp := range imports {
		if u.SliceContainsString(allImports, imp) {
			return nil, nil, fmt.Errorf("duplicate import '%s' in the vendor config file '%s'. It was already imported in the import chain",
				imp,
				vendorConfigFile,
			)
		}

		allImports = append(allImports, imp)

		vendorConfig, _, _, err := ReadAndProcessVendorConfigFile(atmosConfig, imp, false)
		if err != nil {
			return nil, nil, err
		}

		if u.SliceContainsString(vendorConfig.Spec.Imports, imp) {
			return nil, nil, fmt.Errorf("vendor config file '%s' imports itself in 'spec.imports'", imp)
		}

		if len(vendorConfig.Spec.Sources) == 0 && len(vendorConfig.Spec.Imports) == 0 {
			return nil, nil, fmt.Errorf("either 'spec.sources' or 'spec.imports' (or both) must be defined in the vendor config file '%s'", imp)
		}

		mergedSources, allImports, err = processVendorImports(atmosConfig, imp, vendorConfig.Spec.Imports, mergedSources, allImports)
		if err != nil {
			return nil, nil, err
		}

		for i := range vendorConfig.Spec.Sources {
			vendorConfig.Spec.Sources[i].File = imp
		}

		mergedSources = append(mergedSources, vendorConfig.Spec.Sources...)
	}

	return append(mergedSources, sources...), allImports, nil
}

func logInitialMessage(atmosConfig schema.AtmosConfiguration, vendorConfigFileName string, tags []string) {
	logMessage := fmt.Sprintf("Vendoring from '%s'", vendorConfigFileName)
	if len(tags) > 0 {
		logMessage = fmt.Sprintf("%s for tags {%s}", logMessage, strings.Join(tags, ", "))
	}
	u.LogInfo(logMessage)
}

func validateSourceFields(s *schema.AtmosVendorSource, vendorConfigFileName string) error {
	// Ensure necessary fields are present
	if s.File == "" {
		s.File = vendorConfigFileName
	}
	if s.Source == "" {
		return fmt.Errorf("'source' must be specified in 'sources' in the vendor config file '%s'", s.File)
	}
	if len(s.Targets) == 0 {
		return fmt.Errorf("'targets' must be specified for the source '%s' in the vendor config file '%s'", s.Source, s.File)
	}
	return nil
}

func shouldSkipSource(s *schema.AtmosVendorSource, component string, tags []string) bool {
	// Skip if component or tags do not match
	// If `--component` is specified, and it's not equal to this component, skip this component
	// If `--tags` list is specified, and it does not contain any tags defined in this component, skip this component
	return (component != "" && s.Component != component) || (len(tags) > 0 && len(lo.Intersect(tags, s.Tags)) == 0)
}

func determineSourceType(uri *string, vendorConfigFilePath string) (bool, bool, bool, error) {
	// Determine if the URI is an OCI scheme, a local file, or remote
	useOciScheme := strings.HasPrefix(*uri, "oci://")
	if useOciScheme {
		*uri = strings.TrimPrefix(*uri, "oci://")
	}

	useLocalFileSystem := false
	sourceIsLocalFile := false
	if !useOciScheme {
		absPath, err := u.JoinAbsolutePathWithPath(filepath.ToSlash(vendorConfigFilePath), *uri)
		// if URI contain path traversal is path should be resolved
		if err != nil && strings.Contains(*uri, "..") && !strings.HasPrefix(*uri, "file://") {
			return useOciScheme, useLocalFileSystem, sourceIsLocalFile, fmt.Errorf("invalid source path '%s': %w", *uri, err)
		}
		if err == nil {
			uri = &absPath
			useLocalFileSystem = true
			sourceIsLocalFile = u.FileExists(*uri)
		}

		parsedURL, err := url.Parse(*uri)
		if err != nil {
			return useOciScheme, useLocalFileSystem, sourceIsLocalFile, err
		}
		if err == nil && parsedURL.Scheme != "" {
			if parsedURL.Scheme == "file" {
				trimmedPath := strings.TrimPrefix(filepath.ToSlash(parsedURL.Path), "/")
				*uri = filepath.Clean(trimmedPath)
				useLocalFileSystem = true
			}
		}
	}
	return useOciScheme, useLocalFileSystem, sourceIsLocalFile, nil
}

func copyToTarget(atmosConfig schema.AtmosConfiguration, tempDir, targetPath string, s *schema.AtmosVendorSource, sourceIsLocalFile bool, uri string) error {
	copyOptions := cp.Options{
		Skip:          generateSkipFunction(atmosConfig, tempDir, s),
		PreserveTimes: false,
		PreserveOwner: false,
		OnSymlink:     func(src string) cp.SymlinkAction { return cp.Deep },
	}

	// Adjust the target path if it's a local file with no extension
	if sourceIsLocalFile && filepath.Ext(targetPath) == "" {
		// Sanitize the URI for safe filenames, especially on Windows
		sanitizedBase := SanitizeFileName(uri)
		targetPath = filepath.Join(targetPath, sanitizedBase)
	}

	return cp.Copy(tempDir, targetPath, copyOptions)
}

// generateSkipFunction creates a function that determines whether to skip files during copying.
// based on the vendor source configuration. It uses the provided patterns in ExcludedPaths.
// and IncludedPaths to filter files during the copy operation.
//
// Parameters:
//   - atmosConfig: The CLI configuration for logging.
//   - tempDir: The temporary directory containing the files to copy.
//   - s: The vendor source configuration containing exclusion/inclusion patterns.
//
// Returns a function that determines if a file should be skipped during copying.
func generateSkipFunction(atmosConfig schema.AtmosConfiguration, tempDir string, s *schema.AtmosVendorSource) func(os.FileInfo, string, string) (bool, error) {
	return func(srcInfo os.FileInfo, src, dest string) (bool, error) {
		// Skip .git directories
		if filepath.Base(src) == ".git" {
			return true, nil
		}

		// Normalize paths
		tempDir = filepath.ToSlash(tempDir)
		src = filepath.ToSlash(src)
		trimmedSrc := u.TrimBasePathFromPath(tempDir+"/", src)

		// Check if the file should be excluded
		if shouldExclude, err := shouldExcludeFile(src, s.ExcludedPaths, trimmedSrc); shouldExclude || err != nil {
			return shouldExclude, err
		}

		// Check if the file should be included
		if shouldInclude, err := shouldIncludeFile(src, s.IncludedPaths, trimmedSrc); len(s.IncludedPaths) > 0 && (shouldInclude || err != nil) {
			return !shouldInclude, err
		}

		// If no inclusion rules are specified, include the file
		u.LogTrace(fmt.Sprintf("Including '%s'\n", trimmedSrc))
		return false, nil
	}
}

// Helper function to check if a file should be excluded
func shouldExcludeFile(src string, excludedPaths []string, trimmedSrc string) (bool, error) {
	for _, excludePath := range excludedPaths {
		excludeMatch, err := u.PathMatch(excludePath, src)
		if err != nil {
			return true, err
		}
		if excludeMatch {
			u.LogTrace(fmt.Sprintf("Excluding the file '%s' since it matches the '%s' pattern from 'excluded_paths'\n", trimmedSrc, excludePath))
			return true, nil
		}
	}
	return false, nil
}

// Helper function to check if a file should be included
func shouldIncludeFile(src string, includedPaths []string, trimmedSrc string) (bool, error) {
	for _, includePath := range includedPaths {
		includeMatch, err := u.PathMatch(includePath, src)
		if err != nil {
			return false, err
		}
		if includeMatch {
			u.LogTrace(fmt.Sprintf("Including '%s' since it matches the '%s' pattern from 'included_paths'\n", trimmedSrc, includePath))
			return true, nil
		}
	}
	u.LogTrace(fmt.Sprintf("Excluding '%s' since it does not match any pattern from 'included_paths'\n", trimmedSrc))
	return false, nil
}
