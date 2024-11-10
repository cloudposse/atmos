package exec

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	cp "github.com/otiai10/copy"
	"github.com/samber/lo"
	"github.com/spf13/cobra"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ExecuteVendorPullCommand executes `atmos vendor` commands
func ExecuteVendorPullCommand(cmd *cobra.Command, args []string) error {
	info, err := processCommandLineArgs("terraform", cmd, args, nil)
	if err != nil {
		return err
	}

	flags := cmd.Flags()

	// Check if the `stack` flag is set
	// If it's set, process stacks
	processStacks := flags.Changed("stack")

	// InitCliConfig finds and merges CLI configurations in the following order:
	// system dir, home dir, current dir, ENV vars, command-line arguments
	cliConfig, err := cfg.InitCliConfig(info, processStacks)
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
		return fmt.Errorf("either '--component' or '--stack' flag can to be provided, but not both")
	}

	if component != "" && len(tags) > 0 {
		return fmt.Errorf("either '--component' or '--tags' flag can to be provided, but not both")
	}

	if stack != "" {
		// Process stack vendoring
		return ExecuteStackVendorInternal(stack, dryRun)
	}

	// Check `vendor.yaml`
	vendorConfig, vendorConfigExists, foundVendorConfigFile, err := ReadAndProcessVendorConfigFile(cliConfig, cfg.AtmosVendorConfigFileName)
	if vendorConfigExists && err != nil {
		return err
	}

	if vendorConfigExists {
		// Process `vendor.yaml`
		return ExecuteAtmosVendorInternal(cliConfig, foundVendorConfigFile, vendorConfig.Spec, component, tags, dryRun)
	} else {
		// Check and process `component.yaml`
		fmt.Println("No vendor config file found. Checking for component vendoring...")
		if component != "" {
			// Process component vendoring
			componentType, err := flags.GetString("type")
			if err != nil {
				return err
			}

			if componentType == "" {
				componentType = "terraform"
			}

			componentConfig, componentPath, err := ReadAndProcessComponentVendorConfigFile(cliConfig, component, componentType)
			if err != nil {
				return err
			}

			return ExecuteComponentVendorInternal(cliConfig, componentConfig.Spec, component, componentPath, dryRun)
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
func ReadAndProcessVendorConfigFile(cliConfig schema.CliConfiguration, vendorConfigFile string) (
	schema.AtmosVendorConfig,
	bool,
	string,
	error,
) {
	var vendorConfig schema.AtmosVendorConfig
	vendorConfigFileExists := true

	// Check if the vendoring manifest file exists
	foundVendorConfigFile, fileExists := u.SearchConfigFile(vendorConfigFile)

	if !fileExists {
		// Look for the vendoring manifest in the directory pointed to by the `base_path` setting in the `atmos.yaml`
		pathToVendorConfig := path.Join(cliConfig.BasePath, vendorConfigFile)

		if !u.FileExists(pathToVendorConfig) {
			vendorConfigFileExists = false
			return vendorConfig, vendorConfigFileExists, "", fmt.Errorf("vendor config file '%s' does not exist", pathToVendorConfig)
		}

		foundVendorConfigFile = pathToVendorConfig
	}

	vendorConfigFileContent, err := os.ReadFile(foundVendorConfigFile)
	if err != nil {
		return vendorConfig, vendorConfigFileExists, "", err
	}

	vendorConfig, err = u.UnmarshalYAML[schema.AtmosVendorConfig](string(vendorConfigFileContent))
	if err != nil {
		return vendorConfig, vendorConfigFileExists, "", err
	}

	if vendorConfig.Kind != "AtmosVendorConfig" {
		return vendorConfig, vendorConfigFileExists, "",
			fmt.Errorf("invalid 'kind: %s' in the vendor config file '%s'. Supported kinds: 'AtmosVendorConfig'",
				vendorConfig.Kind,
				foundVendorConfigFile,
			)
	}

	return vendorConfig, vendorConfigFileExists, foundVendorConfigFile, nil
}

// ExecuteAtmosVendorInternal downloads the artifacts from the sources and writes them to the targets
func ExecuteAtmosVendorInternal(
	cliConfig schema.CliConfiguration,
	vendorConfigFileName string,
	atmosVendorSpec schema.AtmosVendorSpec,
	component string,
	tags []string,
	dryRun bool,
) error {

	var err error
	vendorConfigFilePath := path.Dir(vendorConfigFileName)

	logInitialMessage(cliConfig, vendorConfigFileName, tags)

	if len(atmosVendorSpec.Sources) == 0 && len(atmosVendorSpec.Imports) == 0 {
		return fmt.Errorf("either 'spec.sources' or 'spec.imports' (or both) must be defined in the vendor config file '%s'", vendorConfigFileName)
	}

	// Process imports and return all sources from all the imports and from `vendor.yaml`
	sources, _, err := processVendorImports(
		cliConfig,
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

	// Process sources
	var packages []pkgAtmosVendor
	for indexSource, s := range sources {
		if shouldSkipSource(s, component, tags) {
			continue
		}

		if err := validateSourceFields(s, vendorConfigFileName); err != nil {
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

		useOciScheme, useLocalFileSystem, sourceIsLocalFile := determineSourceType(uri, vendorConfigFilePath)

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
			targetPath := path.Join(vendorConfigFilePath, target)
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
			opts = []tea.ProgramOption{tea.WithoutRenderer(), tea.WithInput(nil)}
			fmt.Println("TTY is not supported. Running in non-interactive mode.")
		}
		model, err := newModelAtmosVendorInternal(packages, dryRun, cliConfig)
		if err != nil {
			return fmt.Errorf("error initializing model: %v", err)
		}
		if _, err := tea.NewProgram(model, opts...).Run(); err != nil {
			return fmt.Errorf("running atmos vendor internal download error: %w", err)
		}

	}

	return nil
}

// processVendorImports processes all imports recursively and returns a list of sources
func processVendorImports(
	cliConfig schema.CliConfiguration,
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

		vendorConfig, _, _, err := ReadAndProcessVendorConfigFile(cliConfig, imp)
		if err != nil {
			return nil, nil, err
		}

		if u.SliceContainsString(vendorConfig.Spec.Imports, imp) {
			return nil, nil, fmt.Errorf("vendor config file '%s' imports itself in 'spec.imports'", imp)
		}

		if len(vendorConfig.Spec.Sources) == 0 && len(vendorConfig.Spec.Imports) == 0 {
			return nil, nil, fmt.Errorf("either 'spec.sources' or 'spec.imports' (or both) must be defined in the vendor config file '%s'", imp)
		}

		mergedSources, allImports, err = processVendorImports(cliConfig, imp, vendorConfig.Spec.Imports, mergedSources, allImports)
		if err != nil {
			return nil, nil, err
		}

		for i, _ := range vendorConfig.Spec.Sources {
			vendorConfig.Spec.Sources[i].File = imp
		}

		mergedSources = append(mergedSources, vendorConfig.Spec.Sources...)
	}

	return append(mergedSources, sources...), allImports, nil
}
func logInitialMessage(cliConfig schema.CliConfiguration, vendorConfigFileName string, tags []string) {
	logMessage := fmt.Sprintf("Processing vendor config file '%s'", vendorConfigFileName)
	if len(tags) > 0 {
		logMessage = fmt.Sprintf("%s for tags {%s}", logMessage, strings.Join(tags, ", "))
	}
	u.LogInfo(cliConfig, logMessage)

}
func validateSourceFields(s schema.AtmosVendorSource, vendorConfigFileName string) error {
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
func shouldSkipSource(s schema.AtmosVendorSource, component string, tags []string) bool {
	// Skip if component or tags do not match
	// If `--component` is specified, and it's not equal to this component, skip this component
	// If `--tags` list is specified, and it does not contain any tags defined in this component, skip this component
	return (component != "" && s.Component != component) || (len(tags) > 0 && len(lo.Intersect(tags, s.Tags)) == 0)
}

func determineSourceType(uri, vendorConfigFilePath string) (bool, bool, bool) {
	// Determine if the URI is an OCI scheme, a local file, or remote
	useOciScheme := strings.HasPrefix(uri, "oci://")
	if useOciScheme {
		uri = strings.TrimPrefix(uri, "oci://")
	}

	useLocalFileSystem := false
	sourceIsLocalFile := false
	if !useOciScheme {
		if absPath, err := u.JoinAbsolutePathWithPath(vendorConfigFilePath, uri); err == nil {
			uri = absPath
			useLocalFileSystem = true
			sourceIsLocalFile = u.FileExists(uri)
		}
	}
	return useOciScheme, useLocalFileSystem, sourceIsLocalFile
}

func copyToTarget(cliConfig schema.CliConfiguration, tempDir, targetPath string, s schema.AtmosVendorSource, sourceIsLocalFile bool, uri string) error {
	copyOptions := cp.Options{
		Skip:          generateSkipFunction(cliConfig, tempDir, s),
		PreserveTimes: false,
		PreserveOwner: false,
		OnSymlink:     func(src string) cp.SymlinkAction { return cp.Deep },
	}

	// Adjust the target path if it's a local file with no extension
	if sourceIsLocalFile && filepath.Ext(targetPath) == "" {
		targetPath = path.Join(targetPath, filepath.Base(uri))
	}

	return cp.Copy(tempDir, targetPath, copyOptions)
}

func generateSkipFunction(cliConfig schema.CliConfiguration, tempDir string, s schema.AtmosVendorSource) func(os.FileInfo, string, string) (bool, error) {
	return func(srcInfo os.FileInfo, src, dest string) (bool, error) {
		if strings.HasSuffix(src, ".git") {
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
				u.LogTrace(cliConfig, fmt.Sprintf("Excluding the file '%s' since it matches the '%s' pattern from 'excluded_paths'\n",
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
					u.LogTrace(cliConfig, fmt.Sprintf("Including '%s' since it matches the '%s' pattern from 'included_paths'\n",
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
				u.LogTrace(cliConfig, fmt.Sprintf("Excluding '%s' since it does not match any pattern from 'included_paths'\n", trimmedSrc))
				return true, nil
			}
		}

		// If 'included_paths' is not provided, include all files that were not excluded
		u.LogTrace(cliConfig, fmt.Sprintf("Including '%s'\n", u.TrimBasePathFromPath(tempDir+"/", src)))
		return false, nil
	}
}
