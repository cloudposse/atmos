package exec

import (
	"context"
	"fmt"
	"os"
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
	info, err := processCommandLineArgs("terraform", cmd, args)
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
	vendorConfig, vendorConfigExists, err := ReadAndProcessVendorConfigFile(cfg.AtmosVendorConfigFileName)
	if vendorConfigExists && err != nil {
		return err
	}

	if vendorConfigExists {
		return ExecuteAtmosVendorInternal(cliConfig, vendorConfig.Spec, component, dryRun)
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
func ReadAndProcessVendorConfigFile(vendorConfigFile string) (schema.AtmosVendorConfig, bool, error) {
	var vendorConfig schema.AtmosVendorConfig
	vendorConfigFileExists := true

	if !u.FileExists(vendorConfigFile) {
		vendorConfigFileExists = false
		return vendorConfig, vendorConfigFileExists, fmt.Errorf("vendor config file '%s' does not exist", vendorConfigFile)
	}

	vendorConfigFileContent, err := os.ReadFile(vendorConfigFile)
	if err != nil {
		return vendorConfig, vendorConfigFileExists, err
	}

	if err = yaml.Unmarshal(vendorConfigFileContent, &vendorConfig); err != nil {
		return vendorConfig, vendorConfigFileExists, err
	}

	if vendorConfig.Kind != "AtmosVendorConfig" {
		return vendorConfig, vendorConfigFileExists,
			fmt.Errorf("invalid 'kind: %s' in the vendor config file '%s'. Supported kinds: 'AtmosVendorConfig'",
				vendorConfig.Kind,
				vendorConfigFile,
			)
	}

	return vendorConfig, vendorConfigFileExists, nil
}

// ExecuteAtmosVendorInternal downloads the artifacts from the sources and writes them to the targets
func ExecuteAtmosVendorInternal(
	cliConfig schema.CliConfiguration,
	atmosVendorSpec schema.AtmosVendorSpec,
	component string,
	dryRun bool,
) error {

	var tempDir string
	var err error
	var uri string

	u.LogInfo(cliConfig, fmt.Sprintf("Processing vendor config file '%s'",
		cfg.AtmosVendorConfigFileName,
	))

	// Process imports and return all sources from all the imports and from `vendor.yaml`
	sources, err := processVendorImports(atmosVendorSpec.Imports, atmosVendorSpec.Sources)
	if err != nil {
		return err
	}

	if len(sources) == 0 {
		return fmt.Errorf("'spec.sources' is empty in the vendor config file '%s' and the imports", cfg.AtmosVendorConfigFileName)
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
			cfg.AtmosVendorConfigFileName,
		)
	}

	if component != "" && !u.SliceContainsString(components, component) {
		return fmt.Errorf("the flag '--component %s' is passed, but the component is not defined in any of the `sources` in the vendor config file '%s' and the imports",
			component,
			cfg.AtmosVendorConfigFileName,
		)
	}

	targets := lo.FlatMap(sources, func(s schema.AtmosVendorSource, index int) []string {
		return s.Targets
	})

	duplicateTargets := lo.FindDuplicates(targets)

	if len(duplicateTargets) > 0 {
		return fmt.Errorf("dublicate targets %v in the vendor config file '%s' and the imports",
			duplicateTargets,
			cfg.AtmosVendorConfigFileName,
		)
	}

	// Process sources
	for indexSource, s := range sources {
		if component != "" && s.Component != component {
			continue
		}

		if s.File == "" {
			s.File = cfg.AtmosVendorConfigFileName
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

			if s.Component != "" {
				u.LogInfo(cliConfig, fmt.Sprintf("Pulling sources for the component '%s' from '%s' into '%s'",
					s.Component,
					uri,
					target,
				))
			} else {
				u.LogInfo(cliConfig, fmt.Sprintf("Pulling sources from '%s' into '%s'",
					uri,
					target,
				))
			}

			if dryRun {
				return nil
			}

			// Check if `target` is a file path.
			// If it's a file path, check if it's an absolute path.
			if !useOciScheme {
				if absPath, err := u.JoinAbsolutePathWithPath(".", target); err == nil {
					target = absPath
				}
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

			if err = cp.Copy(tempDir, target, copyOptions); err != nil {
				return err
			}
		}
	}
	return nil
}

// processVendorImports processes all imports recursively and returns a list of sources
func processVendorImports(imports []string, sources []schema.AtmosVendorSource) ([]schema.AtmosVendorSource, error) {
	var mergedSources []schema.AtmosVendorSource

	for _, imp := range imports {
		vendorConfig, _, err := ReadAndProcessVendorConfigFile(imp)
		if err != nil {
			return mergedSources, err
		}

		mergedSources = append(mergedSources, vendorConfig.Spec.Sources...)

		_, err = processVendorImports(vendorConfig.Spec.Imports, mergedSources)
		if err != nil {
			return nil, err
		}
	}

	return append(mergedSources, sources...), nil
}
