package exec

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	tea "github.com/charmbracelet/bubbletea"
	cp "github.com/otiai10/copy"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/internal/tui/templates/term"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ExecuteVendorPullCommand executes `atmos vendor` commands
func ExecuteVendorPullCommand(cmd *cobra.Command, args []string) error {
	info, err := ProcessCommandLineArgs("terraform", cmd, args, nil)
	if err != nil {
		return err
	}

	flags := cmd.Flags()

	// Check if the `stack` flag is set
	// If it's set, process stacks
	processStacks := flags.Changed("stack")

	// InitCliConfig finds and merges CLI configurations in the following order:
	// system dir, home dir, current dir, ENV vars, command-line arguments
	atmosConfig, err := cfg.InitCliConfig(info, processStacks)
	if err != nil {
		return fmt.Errorf("failed to initialize CLI config: %w", err)
	}

	dryRun, err := flags.GetBool("dry-run")
	if err != nil {
		return err
	}

	component, err := flags.GetString("component")
	if err != nil {
		return err
	}

	stack, err := flags.GetString("stack")
	if err != nil {
		return err
	}

	tagsCsv, err := flags.GetString("tags")
	if err != nil {
		return err
	}

	var tags []string
	if tagsCsv != "" {
		tags = strings.Split(tagsCsv, ",")
	}

	if component != "" && stack != "" {
		return fmt.Errorf("either '--component' or '--stack' flag can be provided, but not both")
	}

	if component != "" && len(tags) > 0 {
		return fmt.Errorf("either '--component' or '--tags' flag can be provided, but not both")
	}

	// Retrieve the 'everything' flag and set default behavior if no other flags are set
	everything, err := flags.GetBool("everything")
	if err != nil {
		return err
	}

	// If neither `everything`, `component`, `stack`, nor `tags` flags are set, default to `everything = true`
	if !everything && !flags.Changed("everything") && component == "" && stack == "" && len(tags) == 0 {
		everything = true
	}

	// Validate that only one of `--everything`, `--component`, `--stack`, or `--tags` is provided
	if everything && (component != "" || stack != "" || len(tags) > 0) {
		return fmt.Errorf("'--everything' flag cannot be combined with '--component', '--stack', or '--tags' flags")
	}

	if stack != "" {
		// Process stack vendoring
		return ExecuteStackVendorInternal(stack, dryRun)
	}

	// Check `vendor.yaml`
	vendorConfig, vendorConfigExists, foundVendorConfigFile, err := ReadAndProcessVendorConfigFile(atmosConfig, cfg.AtmosVendorConfigFileName, true)
	if err != nil {
		return err
	}
	if !vendorConfigExists && everything {
		return fmt.Errorf("the '--everything' flag is set, but the vendor config file '%s' does not exist", cfg.AtmosVendorConfigFileName)
	}
	if vendorConfigExists {
		// Process `vendor.yaml`
		return ExecuteAtmosVendorInternal(atmosConfig, foundVendorConfigFile, vendorConfig.Spec, component, tags, dryRun)
	} else {
		// Check and process `component.yaml`
		if component != "" {
			// Process component vendoring
			componentType, err := flags.GetString("type")
			if err != nil {
				return err
			}

			if componentType == "" {
				componentType = "terraform"
			}

			componentConfig, componentPath, err := ReadAndProcessComponentVendorConfigFile(atmosConfig, component, componentType)
			if err != nil {
				return err
			}

			return ExecuteComponentVendorInternal(atmosConfig, componentConfig.Spec, component, componentPath, dryRun)
		}
	}

	q := ""
	if len(args) > 0 {
		q = fmt.Sprintf("Did you mean 'atmos vendor pull -c %s'?", args[0])
	}

	return fmt.Errorf("to vendor a component, the '--component' (shorthand '-c') flag needs to be specified.\n" +
		"Example: atmos vendor pull -c <component>\n" +
		q)
}

// ReadAndProcessVendorConfigFile reads and processes the Atmos vendoring config file `vendor.yaml`
func ReadAndProcessVendorConfigFile(
	atmosConfig schema.AtmosConfiguration,
	vendorConfigFile string,
	checkGlobalConfig bool,
) (schema.AtmosVendorConfig, bool, string, error) {
	var vendorConfig schema.AtmosVendorConfig

	// Initialize empty sources slice
	vendorConfig.Spec.Sources = []schema.AtmosVendorSource{}

	var vendorConfigFileExists bool
	var foundVendorConfigFile string

	// Check if vendor config is specified in atmos.yaml
	if checkGlobalConfig && atmosConfig.Vendor.BasePath != "" {
		if !filepath.IsAbs(atmosConfig.Vendor.BasePath) {
			foundVendorConfigFile = filepath.Join(atmosConfig.BasePath, atmosConfig.Vendor.BasePath)
		} else {
			foundVendorConfigFile = atmosConfig.Vendor.BasePath
		}
	} else {
		// Path is not defined in atmos.yaml, proceed with existing logic
		var fileExists bool
		foundVendorConfigFile, fileExists = u.SearchConfigFile(vendorConfigFile)

		if !fileExists {
			// Look for the vendoring manifest in the directory pointed to by the `base_path` setting in `atmos.yaml`
			pathToVendorConfig := filepath.Join(atmosConfig.BasePath, vendorConfigFile)
			foundVendorConfigFile, fileExists = u.SearchConfigFile(pathToVendorConfig)

			if !fileExists {
				vendorConfigFileExists = false
				u.LogWarning(fmt.Sprintf("Vendor config file '%s' does not exist. Proceeding without vendor configurations", pathToVendorConfig))
				return vendorConfig, vendorConfigFileExists, "", nil
			}
		}
	}

	// Check if it's a directory
	fileInfo, err := os.Stat(foundVendorConfigFile)
	if err != nil {
		if os.IsNotExist(err) {
			// File does not exist
			return vendorConfig, false, "", fmt.Errorf("Vendoring is not configured. To set up vendoring, please see https://atmos.tools/core-concepts/vendor/")
		}
		if os.IsPermission(err) {
			// Permission error
			return vendorConfig, false, "", fmt.Errorf("Permission denied when accessing '%s'. Please check the file permissions.", foundVendorConfigFile)
		}
		// Other errors
		return vendorConfig, false, "", fmt.Errorf("An error occurred while accessing the vendoring configuration: %w", err)
	}

	var configFiles []string
	if fileInfo.IsDir() {
		foundVendorConfigFile = filepath.ToSlash(foundVendorConfigFile)
		matches, err := doublestar.Glob(os.DirFS(foundVendorConfigFile), "*.{yaml,yml}")
		if err != nil {
			return vendorConfig, false, "", err
		}
		for _, match := range matches {
			configFiles = append(configFiles, filepath.Join(foundVendorConfigFile, match))
		}
		sort.Strings(configFiles)
		if len(configFiles) == 0 {
			return vendorConfig, false, "", fmt.Errorf("no YAML configuration files found in directory '%s'", foundVendorConfigFile)
		}
	} else {
		configFiles = []string{foundVendorConfigFile}
	}

	// Process all config files
	var mergedSources []schema.AtmosVendorSource
	var mergedImports []string
	sourceMap := make(map[string]bool) // Track unique sources by component name
	importMap := make(map[string]bool) // Track unique imports

	for _, configFile := range configFiles {
		var currentConfig schema.AtmosVendorConfig
		yamlFile, err := os.ReadFile(configFile)
		if err != nil {
			return vendorConfig, false, "", err
		}

		err = yaml.Unmarshal(yamlFile, &currentConfig)
		if err != nil {
			return vendorConfig, false, "", err
		}

		// Merge sources, checking for duplicates
		for _, source := range currentConfig.Spec.Sources {
			if source.Component != "" {
				if sourceMap[source.Component] {
					return vendorConfig, false, "", fmt.Errorf("duplicate component '%s' found in config file '%s'",
						source.Component, configFile)
				}
				sourceMap[source.Component] = true
			}
			mergedSources = append(mergedSources, source)
		}

		// Merge imports, checking for duplicates
		for _, imp := range currentConfig.Spec.Imports {
			if importMap[imp] {
				continue // Skip duplicate imports
			}
			importMap[imp] = true
			mergedImports = append(mergedImports, imp)
		}
	}

	// Create final merged config
	vendorConfig.Spec.Sources = mergedSources
	vendorConfig.Spec.Imports = mergedImports
	vendorConfigFileExists = true

	return vendorConfig, vendorConfigFileExists, foundVendorConfigFile, nil
}

// ExecuteAtmosVendorInternal downloads the artifacts from the sources and writes them to the targets
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

	// Process imports and return all sources from all the imports and from `vendor.yaml`
	sources, _, err := ProcessVendorImports(
		atmosConfig,
		vendorConfigFileName,
		atmosVendorSpec.Imports,
		atmosVendorSpec.Sources,
		[]string{vendorConfigFileName},
		0,  // Initial depth
		50, // Max depth for imports
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

	// Process sources
	var packages []pkgAtmosVendor
	for indexSource, s := range sources {
		if shouldSkipSource(&s, component, tags) {
			continue
		}

		if err := validateSourceFields(&s, vendorConfigFileName); err != nil {
			return err
		}

		tmplData := struct {
			Component string
			Version   string
		}{s.Component, s.Version}

		// Parse 'source' template
		uri, err := ProcessTmpl(fmt.Sprintf("source-%d", indexSource), s.Source, tmplData, false)
		if err != nil {
			return err
		}

		useOciScheme, useLocalFileSystem, sourceIsLocalFile, err := determineSourceType(&uri, vendorConfigFilePath)
		if err != nil {
			return err
		}
		if !useLocalFileSystem {
			err = ValidateURI(uri)
			if err != nil {
				if strings.Contains(uri, "..") {
					return fmt.Errorf("invalid URI for component %s: %w: Please ensure the source is a valid local path", s.Component, err)
				}
				return fmt.Errorf("invalid URI for component %s: %w", s.Component, err)
			}
		}

		// Determine package type
		var pType pkgType
		if useOciScheme {
			pType = pkgTypeOci
		} else if useLocalFileSystem {
			pType = pkgTypeLocal
		} else {
			pType = pkgTypeRemote
		}

		// Process each target within the source
		for indexTarget, tgt := range s.Targets {
			target, err := ProcessTmpl(fmt.Sprintf("target-%d-%d", indexSource, indexTarget), tgt, tmplData, false)
			if err != nil {
				return err
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
				atmosVendorSource: s,
			}

			packages = append(packages, p)

			// Log the action (handled in downloadAndInstall)
		}
	}

	// Run TUI to process packages
	if len(packages) > 0 {
		var opts []tea.ProgramOption
		if !term.IsTTYSupportForStdout() {
			// set tea.WithInput(nil) workaround tea program not run on not TTY mod issue on non TTY mode https://github.com/charmbracelet/bubbletea/issues/761
			opts = []tea.ProgramOption{tea.WithoutRenderer(), tea.WithInput(nil)}
			u.LogWarning("No TTY detected. Falling back to basic output. This can happen when no terminal is attached or when commands are pipelined.")
		}

		model, err := newModelAtmosVendorInternal(packages, dryRun, atmosConfig)
		if err != nil {
			return fmt.Errorf("failed to initialize TUI model: %v (verify terminal capabilities and permissions)", err)
		}
		if _, err := tea.NewProgram(&model, opts...).Run(); err != nil {
			return fmt.Errorf("failed to execute vendor operation in TUI mode: %w (check terminal state)", err)
		}
	}

	return nil
}

// ProcessVendorImports processes all imports recursively and returns a list of sources
// maxDepth limits the recursion depth to prevent stack overflow from deeply nested imports
func ProcessVendorImports(
	atmosConfig schema.AtmosConfiguration,
	vendorConfigFile string,
	imports []string,
	sources []schema.AtmosVendorSource,
	allImports []string,
	depth int,
	maxDepth int,
) ([]schema.AtmosVendorSource, []string, error) {
	// Prevent stack overflow from deeply nested imports
	if depth > maxDepth {
		return nil, nil, fmt.Errorf("maximum import depth of %d exceeded in vendor config file '%s'", maxDepth, vendorConfigFile)
	}

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

		// Pass depth + 1 to track recursion depth
		mergedSources, allImports, err = ProcessVendorImports(atmosConfig, imp, vendorConfig.Spec.Imports, mergedSources, allImports, depth+1, maxDepth)
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
		if err != nil && strings.Contains(*uri, "..") {
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

// generateSkipFunction creates a function that determines whether to skip files during copying
// based on the vendor source configuration. It uses the provided patterns in ExcludedPaths
// and IncludedPaths to filter files during the copy operation.
//
// Parameters:
//   - atmosConfig: The CLI configuration for logging
//   - tempDir: The temporary directory containing the files to copy
//   - s: The vendor source configuration containing exclusion/inclusion patterns
//
// Returns a function that determines if a file should be skipped during copying
func generateSkipFunction(atmosConfig schema.AtmosConfiguration, tempDir string, s *schema.AtmosVendorSource) func(os.FileInfo, string, string) (bool, error) {
	return func(srcInfo os.FileInfo, src, dest string) (bool, error) {
		if filepath.Base(src) == ".git" {
			return true, nil
		}
		tempDir = filepath.ToSlash(tempDir)
		src = filepath.ToSlash(src)

		trimmedSrc := u.TrimBasePathFromPath(tempDir+"/", src)

		// Exclude the files that match the 'excluded_paths' patterns
		// It supports POSIX-style Globs for file names/paths (double-star `**` is supported)
		// https://en.wikipedia.org/wiki/Glob_(programming)
		// https://github.com/bmatcuk/doublestar#patterns
		for _, excludePath := range s.ExcludedPaths {
			excludeMatch, err := u.PathMatch(excludePath, src)
			if err != nil {
				return true, err
			} else if excludeMatch {
				// If the file matches ANY of the 'excluded_paths' patterns, exclude the file
				u.LogTrace(fmt.Sprintf("Excluding the file '%s' since it matches the '%s' pattern from 'excluded_paths'\n",
					trimmedSrc,
					excludePath,
				))
				return true, nil
			}
		}

		// Only include the files that match the 'included_paths' patterns (if any pattern is specified)
		if len(s.IncludedPaths) > 0 {
			anyMatches := false
			for _, includePath := range s.IncludedPaths {
				includeMatch, err := u.PathMatch(includePath, src)
				if err != nil {
					return true, err
				} else if includeMatch {
					// If the file matches ANY of the 'included_paths' patterns, include the file
					u.LogTrace(fmt.Sprintf("Including '%s' since it matches the '%s' pattern from 'included_paths'\n",
						trimmedSrc,
						includePath,
					))
					anyMatches = true
					break
				}
			}

			if anyMatches {
				return false, nil
			} else {
				u.LogTrace(fmt.Sprintf("Excluding '%s' since it does not match any pattern from 'included_paths'\n", trimmedSrc))
				return true, nil
			}
		}

		// If 'included_paths' is not provided, include all files that were not excluded
		u.LogTrace(fmt.Sprintf("Including '%s'\n", u.TrimBasePathFromPath(tempDir+"/", src)))
		return false, nil
	}
}
