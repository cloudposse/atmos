package exec

import (
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	tea "github.com/charmbracelet/bubbletea"
	cp "github.com/otiai10/copy"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

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
			pathToVendorConfig := path.Join(atmosConfig.BasePath, vendorConfigFile)
			foundVendorConfigFile, fileExists = u.SearchConfigFile(pathToVendorConfig)

			if !fileExists {
				vendorConfigFileExists = false
				u.LogWarning(atmosConfig, fmt.Sprintf("Vendor config file '%s' does not exist. Proceeding without vendor configurations", pathToVendorConfig))
				return vendorConfig, vendorConfigFileExists, "", nil
			}
		}
	}

	// Check if it's a directory
	fileInfo, err := os.Stat(foundVendorConfigFile)
	if err != nil {
		return vendorConfig, false, "", err
	}

	var configFiles []string
	if fileInfo.IsDir() {
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

	var tempDir string

	// Process imports and return all sources from all the imports and from `vendor.yaml`
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

	tempDir, err = os.MkdirTemp("", "atmos_vendor_")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			u.LogWarning(atmosConfig, fmt.Sprintf("failed to clean up temp directory %s: %v", tempDir, err))
		}
	}()

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
		err = validateURI(uri)
		if err != nil {
			return err
		}

		useOciScheme, useLocalFileSystem, sourceIsLocalFile, isGitHubSource := determineSourceType(&uri, vendorConfigFilePath)

		// Handle GitHub source
		if isGitHubSource {
			if dryRun {
				u.LogInfo(atmosConfig, fmt.Sprintf("Dry run: Fetching GitHub source: %s", uri))
			} else {
				u.LogDebug(atmosConfig, fmt.Sprintf("Fetching GitHub source: %s", uri))
				fileContents, err := u.DownloadFileFromGitHub(uri)
				if err != nil {
					return fmt.Errorf("failed to download GitHub file: %w", err)
				}
				// Save the downloaded file to the existing tempDir
				tempGitHubFile := filepath.Join(tempDir, filepath.Base(uri))
				err = os.WriteFile(tempGitHubFile, fileContents, os.ModePerm)
				if err != nil {
					return fmt.Errorf("failed to save GitHub file to temp location: %w", err)
				}
				// Update the URI to point to the saved file in the temp directory
				uri = tempGitHubFile
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
			targetPath := filepath.Join(vendorConfigFilePath, target)
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
		if !CheckTTYSupport() {
			// set tea.WithInput(nil) workaround tea program not run on not TTY mod issue on non TTY mode https://github.com/charmbracelet/bubbletea/issues/761
			opts = []tea.ProgramOption{tea.WithoutRenderer(), tea.WithInput(nil)}
			u.LogWarning(atmosConfig, "No TTY detected. Falling back to basic output. This can happen when no terminal is attached or when commands are pipelined.")
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
	u.LogInfo(atmosConfig, logMessage)
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

func determineSourceType(uri *string, vendorConfigFilePath string) (bool, bool, bool, bool) {
	// Determine if the URI is an OCI scheme, a local file, a remote GitHub source, or a generic remote
	useOciScheme := strings.HasPrefix(*uri, "oci://")
	if useOciScheme {
		*uri = strings.TrimPrefix(*uri, "oci://")
	}

	useLocalFileSystem := false
	sourceIsLocalFile := false
	isGitHubSource := false

	// If not OCI, we proceed with checks
	if !useOciScheme {
		parsedURL, err := url.Parse(*uri)
		if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
			// Not a valid URL or no host: consider local filesystem
			if absPath, err := u.JoinAbsolutePathWithPath(vendorConfigFilePath, *uri); err == nil {
				uri = &absPath
				useLocalFileSystem = true
				sourceIsLocalFile = u.FileExists(*uri)
			}
		} else {
			if parsedURL.Host == "github.com" && parsedURL.Scheme == "https" {
				isGitHubSource = true
			}
		}
	}

	return useOciScheme, useLocalFileSystem, sourceIsLocalFile, isGitHubSource
}

// sanitizeFileName replaces invalid characters and query strings with underscores for Windows.
func sanitizeFileName(uri string) string {

	// Parse the URI to handle paths and query strings properly
	parsed, err := url.Parse(uri)
	if err != nil {
		// Fallback to basic filepath.Base if URI parsing fails
		return filepath.Base(uri)
	}

	// Extract the path component of the URI
	base := filepath.Base(parsed.Path)

	// This logic applies only to Windows
	if runtime.GOOS != "windows" {
		return base
	}

	// Replace invalid characters for Windows
	base = strings.Map(func(r rune) rune {
		switch r {
		case '\\', '/', ':', '*', '?', '"', '<', '>', '|':
			return '_'
		default:
			return r
		}
	}, base)

	return base
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
		sanitizedBase := sanitizeFileName(uri)
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
				u.LogTrace(atmosConfig, fmt.Sprintf("Excluding the file '%s' since it matches the '%s' pattern from 'excluded_paths'\n",
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
					u.LogTrace(atmosConfig, fmt.Sprintf("Including '%s' since it matches the '%s' pattern from 'included_paths'\n",
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
				u.LogTrace(atmosConfig, fmt.Sprintf("Excluding '%s' since it does not match any pattern from 'included_paths'\n", trimmedSrc))
				return true, nil
			}
		}

		// If 'included_paths' is not provided, include all files that were not excluded
		u.LogTrace(atmosConfig, fmt.Sprintf("Including '%s'\n", u.TrimBasePathFromPath(tempDir+"/", src)))
		return false, nil
	}
}

func validateURI(uri string) error {
	if uri == "" {
		return fmt.Errorf("URI cannot be empty")
	}
	// Maximum length check
	if len(uri) > 2048 {
		return fmt.Errorf("URI exceeds maximum length of 2048 characters")
	}
	// Add more validation as needed
	// Validate URI format
	if strings.Contains(uri, "..") {
		return fmt.Errorf("URI cannot contain path traversal sequences")
	}
	if strings.Contains(uri, " ") {
		return fmt.Errorf("URI cannot contain spaces")
	}
	// Validate characters
	if strings.ContainsAny(uri, "<>|&;$") {
		return fmt.Errorf("URI contains invalid characters")
	}
	// Validate scheme-specific format
	if strings.HasPrefix(uri, "oci://") {
		if !strings.Contains(uri[6:], "/") {
			return fmt.Errorf("invalid OCI URI format")
		}
	} else if strings.Contains(uri, "://") {
		scheme := strings.Split(uri, "://")[0]
		if !isValidScheme(scheme) {
			return fmt.Errorf("unsupported URI scheme: %s", scheme)
		}
	}
	return nil
}
func isValidScheme(scheme string) bool {
	validSchemes := map[string]bool{
		"http":  true,
		"https": true,
		"git":   true,
		"ssh":   true,
	}
	return validSchemes[scheme]
}
