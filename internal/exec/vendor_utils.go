package exec

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/go-getter"
	cp "github.com/otiai10/copy"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"

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

	// InitCliConfig finds and merges CLI configurations in the following order:
	// system dir, home dir, current dir, ENV vars, command-line arguments
	cliConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		return err
	}

	flags := cmd.Flags()

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

	if component != "" && stack != "" {
		return fmt.Errorf("either '--component' or '--stack' flag needs to be provided, but not both")
	}

	if stack != "" {
		// Process stack vendoring
		return ExecuteStackVendorInternal(stack, dryRun)
	}

	// Check and process `vendor.yaml`
	vendorConfig, vendorConfigExists, foundVendorConfigFile, err := ReadAndProcessVendorConfigFile(cliConfig, cfg.AtmosVendorConfigFileName)
	if vendorConfigExists && err != nil {
		return err
	}

	if vendorConfigExists {
		return ExecuteAtmosVendorInternal(cliConfig, foundVendorConfigFile, vendorConfig.Spec, component, dryRun)
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

	// If the vendoring manifest is specified without an extension, use the default extension
	if filepath.Ext(vendorConfigFile) == "" {
		vendorConfigFile = vendorConfigFile + cfg.DefaultVendoringManifestFileExtension
	}

	foundVendorConfigFile := vendorConfigFile

	// Look for the vendoring manifest in the current directory
	if !u.FileExists(vendorConfigFile) {
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

	if err = yaml.Unmarshal(vendorConfigFileContent, &vendorConfig); err != nil {
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
	dryRun bool,
) error {

	var tempDir string
	var err error
	var uri string
	vendorConfigFilePath := path.Dir(vendorConfigFileName)

	u.LogInfo(cliConfig, fmt.Sprintf("Processing vendor config file '%s'", vendorConfigFileName))

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

	components := lo.FilterMap(sources, func(s schema.AtmosVendorSource, index int) (string, bool) {
		if s.Component != "" {
			return s.Component, true
		}
		return "", false
	})

	duplicateComponents := lo.FindDuplicates(components)

	if len(duplicateComponents) > 0 {
		return fmt.Errorf("dublicate component names %v in the vendor config file '%s' and the imports",
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
	//	return fmt.Errorf("dublicate targets %v in the vendor config file '%s' and the imports",
	//		duplicateTargets,
	//		vendorConfigFileName,
	//	)
	//}

	// Process sources
	for indexSource, s := range sources {
		if component != "" && s.Component != component {
			continue
		}

		if s.File == "" {
			s.File = vendorConfigFileName
		}

		if s.Source == "" {
			return fmt.Errorf("'source' must be specified in 'sources' in the vendor config file '%s'",
				s.File,
			)
		}

		if len(s.Targets) == 0 {
			return fmt.Errorf("'targets' must be specified for the source '%s' in the vendor config file '%s'",
				s.Source,
				s.File,
			)
		}

		// Parse 'source' template
		if s.Version != "" {
			uri, err = u.ProcessTmpl(fmt.Sprintf("source-%d-%s", indexSource, s.Version), s.Source, s, false)
			if err != nil {
				return err
			}
		} else {
			uri = s.Source
		}

		// Check if `uri` uses the `oci://` scheme (to download the sources from an OCI-compatible registry).
		useOciScheme := false
		if strings.HasPrefix(uri, "oci://") {
			useOciScheme = true
			uri = strings.TrimPrefix(uri, "oci://")
		}

		if !useOciScheme {
			if absPath, err := u.JoinAbsolutePathWithPath(vendorConfigFilePath, uri); err == nil {
				uri = absPath
			}
		}

		// Iterate over the targets
		for indexTarget, tgt := range s.Targets {
			var target string
			// Parse 'target' template
			if s.Version != "" {
				target, err = u.ProcessTmpl(fmt.Sprintf("target-%d-%d-%s", indexSource, indexTarget, s.Version), tgt, s, false)
				if err != nil {
					return err
				}
			} else {
				target = tgt
			}

			targetPath := path.Join(vendorConfigFilePath, target)

			if s.Component != "" {
				u.LogInfo(cliConfig, fmt.Sprintf("Pulling sources for the component '%s' from '%s' into '%s'",
					s.Component,
					uri,
					targetPath,
				))
			} else {
				u.LogInfo(cliConfig, fmt.Sprintf("Pulling sources from '%s' into '%s'",
					uri,
					targetPath,
				))
			}

			if dryRun {
				return nil
			}

			// Create temp folder
			// We are using a temp folder for the following reasons:
			// 1. 'git' does not clone into an existing folder (and we have the existing component folder with `component.yaml` in it)
			// 2. We have the option to skip some files we don't need and include only the files we need when copying from the temp folder to the destination folder
			tempDir, err = os.MkdirTemp("", strconv.FormatInt(time.Now().Unix(), 10))
			if err != nil {
				return err
			}

			defer removeTempDir(cliConfig, tempDir)

			// Download the source into the temp directory
			if !useOciScheme {
				client := &getter.Client{
					Ctx: context.Background(),
					// Define the destination where the files will be stored. This will create the directory if it doesn't exist
					Dst: tempDir,
					// Source
					Src:  uri,
					Mode: getter.ClientModeDir,
				}

				if err = client.Get(); err != nil {
					return err
				}
			} else {
				// Download the Image from the OCI-compatible registry, extract the layers from the tarball, and write to the destination directory
				err = processOciImage(cliConfig, uri, tempDir)
				if err != nil {
					return err
				}
			}

			// Copy from the temp folder to the destination folder and skip the excluded files
			copyOptions := cp.Options{
				// Skip specifies which files should be skipped
				Skip: func(srcInfo os.FileInfo, src, dest string) (bool, error) {
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
				},

				// Preserve the atime and the mtime of the entries
				// On linux we can preserve only up to 1 millisecond accuracy
				PreserveTimes: false,

				// Preserve the uid and the gid of all entries
				PreserveOwner: false,
			}

			if err = cp.Copy(tempDir, targetPath, copyOptions); err != nil {
				return err
			}
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
