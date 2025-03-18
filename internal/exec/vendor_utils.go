package exec

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	log "github.com/charmbracelet/log"
	cp "github.com/otiai10/copy"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

var (
	ErrVendorComponents              = errors.New("failed to vendor components")
	ErrSourceMissing                 = errors.New("'source' must be specified in 'sources' in the vendor config file")
	ErrTargetsMissing                = errors.New("'targets' must be specified for the source in the vendor config file")
	ErrVendorConfigSelfImport        = errors.New("vendor config file imports itself in 'spec.imports'")
	ErrMissingVendorConfigDefinition = errors.New("either 'spec.sources' or 'spec.imports' (or both) must be defined in the vendor config file")
	ErrVendoringNotConfigured        = errors.New("Vendoring is not configured. To set up vendoring, please see https://atmos.tools/core-concepts/vendor/")
	ErrPermissionDenied              = errors.New("Permission denied when accessing")
	ErrEmptySources                  = errors.New("'spec.sources' is empty in the vendor config file and the imports")
	ErrNoComponentsWithTags          = errors.New("there are no components in the vendor config file")
	ErrNoYAMLConfigFiles             = errors.New("no YAML configuration files found in directory")
	ErrDuplicateComponents           = errors.New("duplicate component names")
	ErrDuplicateImport               = errors.New("Duplicate import")
	ErrDuplicateComponentsFound      = errors.New("duplicate component")
	ErrComponentNotDefined           = errors.New("the flag '--component' is passed, but the component is not defined in any of the 'sources' in the vendor config file and the imports")
)

type processTargetsParams struct {
	IndexSource          int
	Source               *schema.AtmosVendorSource
	TemplateData         struct{ Component, Version string }
	VendorConfigFilePath string
	URI                  string
	PkgType              pkgType
	SourceIsLocalFile    bool
}
type executeVendorOptions struct {
	atmosConfig          *schema.AtmosConfiguration
	vendorConfigFileName string
	atmosVendorSpec      schema.AtmosVendorSpec
	component            string
	tags                 []string
	dryRun               bool
}

// ReadAndProcessVendorConfigFile reads and processes the Atmos vendoring config file `vendor.yaml`.
func ReadAndProcessVendorConfigFile(
	atmosConfig *schema.AtmosConfiguration,
	vendorConfigFile string,
	checkGlobalConfig bool,
) (schema.AtmosVendorConfig, bool, string, error) {
	var vendorConfig schema.AtmosVendorConfig
	vendorConfig.Spec.Sources = []schema.AtmosVendorSource{} // Initialize empty sources slice

	// Determine the vendor config file path
	foundVendorConfigFile := resolveVendorConfigFilePath(atmosConfig, vendorConfigFile, checkGlobalConfig)
	if foundVendorConfigFile == "" {
		log.Warn("Vendor config file does not exist. Proceeding without vendor configurations", "path", vendorConfig)
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

// Helper function to resolve the vendor config file path.
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

// Helper function to get config files from a path (file or directory).
func getConfigFiles(path string) ([]string, error) {
	fileInfo, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrVendoringNotConfigured
		}
		if os.IsPermission(err) {
			return nil, fmt.Errorf("%w '%s'. Please check the file permissions", ErrPermissionDenied, path)
		}
		return nil, fmt.Errorf("An error occurred while accessing the vendoring configuration: %w", err)
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

// ExecuteAtmosVendorInternal downloads the artifacts from the sources and writes them to the targets.
func ExecuteAtmosVendorInternal(params *executeVendorOptions) error {
	var err error
	vendorConfigFilePath := filepath.Dir(params.vendorConfigFileName)

	logInitialMessage(params.vendorConfigFileName, params.tags)
	if len(params.atmosVendorSpec.Sources) == 0 && len(params.atmosVendorSpec.Imports) == 0 {
		return fmt.Errorf("%w '%s'", ErrMissingVendorConfigDefinition, params.vendorConfigFileName)
	}
	// Process imports and return all sources from all the imports and from `vendor.yaml`.
	sources, _, err := processVendorImports(
		params.atmosConfig,
		params.vendorConfigFileName,
		params.atmosVendorSpec.Imports,
		params.atmosVendorSpec.Sources,
		[]string{params.vendorConfigFileName},
	)
	if err != nil {
		return err
	}

	if len(sources) == 0 {
		return fmt.Errorf("%w %s", ErrEmptySources, params.vendorConfigFileName)
	}

	if err := validateTagsAndComponents(sources, params.vendorConfigFileName, params.component, params.tags); err != nil {
		return err
	}

	packages, err := processAtmosVendorSource(sources, params.component, params.tags, params.vendorConfigFileName, vendorConfigFilePath)
	if err != nil {
		return err
	}
	if len(packages) > 0 {
		return executeVendorModel(packages, params.dryRun, params.atmosConfig)
	}
	return nil
}

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

func processAtmosVendorSource(sources []schema.AtmosVendorSource, component string, tags []string, vendorConfigFileName, vendorConfigFilePath string) ([]pkgAtmosVendor, error) {
	var packages []pkgAtmosVendor
	for indexSource := range sources {
		if shouldSkipSource(&sources[indexSource], component, tags) {
			continue
		}

		if err := validateSourceFields(&sources[indexSource], vendorConfigFileName); err != nil {
			return nil, err
		}

		tmplData := struct {
			Component string
			Version   string
		}{sources[indexSource].Component, sources[indexSource].Version}

		// Parse 'source' template
		uri, err := ProcessTmpl(fmt.Sprintf("source-%d", indexSource), sources[indexSource].Source, tmplData, false)
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
					return nil, fmt.Errorf("invalid URI for component %s: %w: Please ensure the source is a valid local path", sources[indexSource].Component, err)
				}
				return nil, fmt.Errorf("invalid URI for component %s: %w", sources[indexSource].Component, err)
			}
		}

		// Determine package type
		pType := determinePackageType(useOciScheme, useLocalFileSystem)

		// Process each target within the source
		pkgs, err := processTargets(&processTargetsParams{
			IndexSource:          indexSource,
			Source:               &sources[indexSource],
			TemplateData:         tmplData,
			VendorConfigFilePath: vendorConfigFilePath,
			URI:                  uri,
			PkgType:              pType,
			SourceIsLocalFile:    sourceIsLocalFile,
		})
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

func processTargets(params *processTargetsParams) ([]pkgAtmosVendor, error) {
	var packages []pkgAtmosVendor
	for indexTarget, tgt := range params.Source.Targets {
		target, err := ProcessTmpl(fmt.Sprintf("target-%d-%d", params.IndexSource, indexTarget), tgt, params.TemplateData, false)
		if err != nil {
			return nil, err
		}
		targetPath := filepath.Join(filepath.ToSlash(params.VendorConfigFilePath), filepath.ToSlash(target))
		pkgName := params.Source.Component
		if pkgName == "" {
			pkgName = params.URI
		}
		// Create package struct
		p := pkgAtmosVendor{
			uri:               params.URI,
			name:              pkgName,
			targetPath:        targetPath,
			sourceIsLocalFile: params.SourceIsLocalFile,
			pkgType:           params.PkgType,
			version:           params.Source.Version,
			atmosVendorSource: *params.Source,
		}
		packages = append(packages, p)
	}
	return packages, nil
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

		vendorConfig, _, _, err := ReadAndProcessVendorConfigFile(atmosConfig, imp, false)
		if err != nil {
			return nil, nil, err
		}

		if u.SliceContainsString(vendorConfig.Spec.Imports, imp) {
			return nil, nil, fmt.Errorf("%w file '%s'", ErrVendorConfigSelfImport, imp)
		}

		if len(vendorConfig.Spec.Sources) == 0 && len(vendorConfig.Spec.Imports) == 0 {
			return nil, nil, fmt.Errorf("%w '%s'", ErrMissingVendorConfigDefinition, imp)
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

func logInitialMessage(vendorConfigFileName string, tags []string) {
	logMessage := fmt.Sprintf("Vendoring from '%s'", vendorConfigFileName)
	if len(tags) > 0 {
		logMessage = fmt.Sprintf("%s for tags {%s}", logMessage, strings.Join(tags, ", "))
	}
	log.Info(logMessage)
}

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

func shouldSkipSource(s *schema.AtmosVendorSource, component string, tags []string) bool {
	// Skip if component or tags do not match
	// If `--component` is specified, and it's not equal to this component, skip this component
	// If `--tags` list is specified, and it does not contain any tags defined in this component, skip this component
	return (component != "" && s.Component != component) || (len(tags) > 0 && len(lo.Intersect(tags, s.Tags)) == 0)
}

func determineSourceType(uri *string, vendorConfigFilePath string) (bool, bool, bool, error) {
	// Determine if the URI is an OCI scheme, a local file, or remote
	useLocalFileSystem := false
	sourceIsLocalFile := false
	useOciScheme := strings.HasPrefix(*uri, "oci://")
	if useOciScheme {
		*uri = strings.TrimPrefix(*uri, "oci://")
		return useOciScheme, useLocalFileSystem, sourceIsLocalFile, nil
	}

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
	if parsedURL.Scheme != "" {
		if parsedURL.Scheme == "file" {
			trimmedPath := strings.TrimPrefix(filepath.ToSlash(parsedURL.Path), "/")
			*uri = filepath.Clean(trimmedPath)
			useLocalFileSystem = true
		}
	}

	return useOciScheme, useLocalFileSystem, sourceIsLocalFile, nil
}

func copyToTarget(tempDir, targetPath string, s *schema.AtmosVendorSource, sourceIsLocalFile bool, uri string) error {
	copyOptions := cp.Options{
		Skip:          generateSkipFunction(tempDir, s),
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

// GenerateSkipFunction creates a function that determines whether to skip files during copying.
// Based on the vendor source configuration. It uses the provided patterns in ExcludedPaths.
// And IncludedPaths to filter files during the copy operation.
//
// Parameters:
//   - atmosConfig: The CLI configuration for logging.
//   - tempDir: The temporary directory containing the files to copy.
//   - s: The vendor source configuration containing exclusion/inclusion patterns.
//
// Returns a function that determines if a file should be skipped during copying.
func generateSkipFunction(tempDir string, s *schema.AtmosVendorSource) func(os.FileInfo, string, string) (bool, error) {
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
		if len(s.ExcludedPaths) > 0 {
			return shouldExcludeFile(src, s.ExcludedPaths, trimmedSrc)
		}

		// Only include the files that match the 'included_paths' patterns (if any pattern is specified)
		if len(s.IncludedPaths) > 0 {
			return shouldIncludeFile(src, s.IncludedPaths, trimmedSrc)
		}

		// If 'included_paths' is not provided, include all files that were not excluded
		log.Debug("Including", u.TrimBasePathFromPath(tempDir+"/", src))
		return false, nil
	}
}

// Exclude the files that match the 'excluded_paths' patterns.
// It supports POSIX-style Globs for file names/paths (double-star `**` is supported).
// https://en.wikipedia.org/wiki/Glob_(programming).
// https://github.com/bmatcuk/doublestar#pattern.
func shouldExcludeFile(src string, excludedPaths []string, trimmedSrc string) (bool, error) {
	for _, excludePath := range excludedPaths {
		excludeMatch, err := u.PathMatch(excludePath, src)
		if err != nil {
			return true, err
		} else if excludeMatch {
			// If the file matches ANY of the 'excluded_paths' patterns, exclude the file
			log.Debug("Excluding file since it match any pattern from 'excluded_paths'", "excluded_paths", excludePath, "source", trimmedSrc)
			return true, nil
		}
	}
	return false, nil
}

// Helper function to check if a file should be included.
func shouldIncludeFile(src string, includedPaths []string, trimmedSrc string) (bool, error) {
	anyMatches := false
	for _, includePath := range includedPaths {
		includeMatch, err := u.PathMatch(includePath, src)
		if err != nil {
			return true, err
		} else if includeMatch {
			// If the file matches ANY of the 'included_paths' patterns, include the file
			log.Debug("Including path since it matches the '%s' pattern from 'included_paths'", "included_paths", includePath, "path", trimmedSrc)

			anyMatches = true
			break
		}
	}

	if anyMatches {
		return false, nil
	} else {
		log.Debug("Excluding path since it does not match any pattern from 'included_paths'", "path", trimmedSrc)
		return true, nil
	}
}
