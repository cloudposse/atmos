package exec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/Masterminds/sprig/v3"
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

	// Check and process `vendor.yaml`
	vendorConfig, vendorConfigExists, err := ReadAndProcessVendorConfigFile(cliConfig)
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
		} else if stack != "" {
			// Process stack vendoring
			return ExecuteStackVendorInternal(stack, dryRun)
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

// ReadAndProcessComponentVendorConfigFile reads and processes the component vendoring config file `component.yaml`
func ReadAndProcessComponentVendorConfigFile(
	cliConfig schema.CliConfiguration,
	component string,
	componentType string,
) (schema.VendorComponentConfig, string, error) {
	var componentBasePath string
	var componentConfig schema.VendorComponentConfig

	if componentType == "terraform" {
		componentBasePath = cliConfig.Components.Terraform.BasePath
	} else if componentType == "helmfile" {
		componentBasePath = cliConfig.Components.Helmfile.BasePath
	} else {
		return componentConfig, "", fmt.Errorf("type '%s' is not supported. Valid types are 'terraform' and 'helmfile'", componentType)
	}

	componentPath := path.Join(cliConfig.BasePath, componentBasePath, component)

	dirExists, err := u.IsDirectory(componentPath)
	if err != nil {
		return componentConfig, "", err
	}

	if !dirExists {
		return componentConfig, "", fmt.Errorf("folder '%s' does not exist", componentPath)
	}

	componentConfigFile := path.Join(componentPath, cfg.ComponentVendorConfigFileName)
	if !u.FileExists(componentConfigFile) {
		return componentConfig, "", fmt.Errorf("component vendoring config file '%s' does not exist in the '%s' folder",
			cfg.ComponentVendorConfigFileName,
			componentPath,
		)
	}

	componentConfigFileContent, err := os.ReadFile(componentConfigFile)
	if err != nil {
		return componentConfig, "", err
	}

	if err = yaml.Unmarshal(componentConfigFileContent, &componentConfig); err != nil {
		return componentConfig, "", err
	}

	if componentConfig.Kind != "ComponentVendorConfig" {
		return componentConfig, "", fmt.Errorf("invalid 'kind: %s' in the component vendoring config file '%s'. Supported kinds: 'ComponentVendorConfig'",
			componentConfig.Kind,
			cfg.ComponentVendorConfigFileName)
	}

	return componentConfig, componentPath, nil
}

// ReadAndProcessVendorConfigFile reads and processes the Atmos vendoring config file `vendor.yaml`
func ReadAndProcessVendorConfigFile(
	cliConfig schema.CliConfiguration,
) (schema.AtmosVendorConfig, bool, error) {
	var vendorConfig schema.AtmosVendorConfig
	vendorConfigFileExists := true

	vendorConfigFile := cfg.AtmosVendorConfigFileName
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
				cfg.ComponentVendorConfigFileName,
			)
	}

	return vendorConfig, vendorConfigFileExists, nil
}

// ExecuteComponentVendorInternal executes the 'atmos vendor pull' command for a component
// Supports all protocols (local files, Git, Mercurial, HTTP, HTTPS, Amazon S3, Google GCP),
// URL and archive formats described in https://github.com/hashicorp/go-getter
// https://www.allee.xyz/en/posts/getting-started-with-go-getter
// https://github.com/otiai10/copy
// https://opencontainers.org/
// https://github.com/google/go-containerregistry
// https://docs.aws.amazon.com/AmazonECR/latest/public/public-registries.html
func ExecuteComponentVendorInternal(
	cliConfig schema.CliConfiguration,
	vendorComponentSpec schema.VendorComponentSpec,
	component string,
	componentPath string,
	dryRun bool,
) error {

	var tempDir string
	var err error
	var t *template.Template
	var uri string

	if vendorComponentSpec.Source.Uri == "" {
		return fmt.Errorf("'uri' must be specified in 'source.uri' in the component vendoring config file '%s'", cfg.ComponentVendorConfigFileName)
	}

	// Parse 'uri' template
	if vendorComponentSpec.Source.Version != "" {
		t, err = template.New(fmt.Sprintf("source-uri-%s", vendorComponentSpec.Source.Version)).Funcs(sprig.FuncMap()).Parse(vendorComponentSpec.Source.Uri)
		if err != nil {
			return err
		}

		var tpl bytes.Buffer
		err = t.Execute(&tpl, vendorComponentSpec.Source)
		if err != nil {
			return err
		}

		uri = tpl.String()
	} else {
		uri = vendorComponentSpec.Source.Uri
	}

	// Check if `uri` uses the `oci://` scheme (to download the sources from an OCI-compatible registry).
	useOciScheme := false
	if strings.HasPrefix(uri, "oci://") {
		useOciScheme = true
		uri = strings.TrimPrefix(uri, "oci://")
	}

	// Check if `uri` is a file path.
	// If it's a file path, check if it's an absolute path.
	// If it's not absolute path, join it with the base path (component dir) and convert to absolute path.
	if !useOciScheme {
		if absPath, err := u.JoinAbsolutePathWithPath(componentPath, uri); err == nil {
			uri = absPath
		}
	}

	u.LogInfo(cliConfig, fmt.Sprintf("Pulling sources for the component '%s' from '%s' into '%s'\n",
		component,
		uri,
		componentPath,
	))

	if !dryRun {
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
				for _, excludePath := range vendorComponentSpec.Source.ExcludedPaths {
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
				if len(vendorComponentSpec.Source.IncludedPaths) > 0 {
					anyMatches := false
					for _, includePath := range vendorComponentSpec.Source.IncludedPaths {
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

		if err = cp.Copy(tempDir, componentPath, copyOptions); err != nil {
			return err
		}
	}

	// Process mixins
	if len(vendorComponentSpec.Mixins) > 0 {
		for _, mixin := range vendorComponentSpec.Mixins {
			if mixin.Uri == "" {
				return errors.New("'uri' must be specified for each 'mixin' in the 'component.yaml' file")
			}

			if mixin.Filename == "" {
				return errors.New("'filename' must be specified for each 'mixin' in the 'component.yaml' file")
			}

			// Parse 'uri' template
			if mixin.Version != "" {
				t, err = template.New(fmt.Sprintf("mixin-uri-%s", mixin.Version)).Funcs(sprig.FuncMap()).Parse(mixin.Uri)
				if err != nil {
					return err
				}

				var tpl bytes.Buffer
				err = t.Execute(&tpl, mixin)
				if err != nil {
					return err
				}

				uri = tpl.String()
			} else {
				uri = mixin.Uri
			}

			// Check if `uri` uses the `oci://` scheme (to download the sources from an OCI-compatible registry).
			useOciScheme = false
			if strings.HasPrefix(uri, "oci://") {
				useOciScheme = true
				uri = strings.TrimPrefix(uri, "oci://")
			}

			// Check if `uri` is a file path.
			// If it's a file path, check if it's an absolute path.
			// If it's not absolute path, join it with the base path (component dir) and convert to absolute path.
			if !useOciScheme {
				if absPath, err := u.JoinAbsolutePathWithPath(componentPath, uri); err == nil {
					uri = absPath
				}
			}

			u.LogInfo(cliConfig, fmt.Sprintf(
				"Pulling the mixin '%s' for the component '%s' into '%s'\n",
				uri,
				component,
				path.Join(componentPath, mixin.Filename),
			))

			if !dryRun {
				err = os.RemoveAll(tempDir)
				if err != nil {
					return err
				}

				// Download the mixin into the temp file
				if !useOciScheme {
					client := &getter.Client{
						Ctx:  context.Background(),
						Dst:  path.Join(tempDir, mixin.Filename),
						Src:  uri,
						Mode: getter.ClientModeFile,
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

				// Copy from the temp folder to the destination folder
				copyOptions := cp.Options{
					// Preserve the atime and the mtime of the entries
					PreserveTimes: false,

					// Preserve the uid and the gid of all entries
					PreserveOwner: false,

					// OnSymlink specifies what to do on symlink
					// Override the destination file if it already exists
					// Prevent the error:
					// symlink components/terraform/mixins/context.tf components/terraform/infra/vpc-flow-logs-bucket/context.tf: file exists
					OnSymlink: func(src string) cp.SymlinkAction {
						return cp.Deep
					},
				}

				if err = cp.Copy(tempDir, componentPath, copyOptions); err != nil {
					return err
				}
			}
		}
	}

	return nil
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

	if len(atmosVendorSpec.Sources) == 0 {
		return fmt.Errorf("'spec.sources' is empty in the vendor config file '%s'", cfg.AtmosVendorConfigFileName)
	}

	components := lo.FilterMap(atmosVendorSpec.Sources, func(s schema.AtmosVendorSource, index int) (string, bool) {
		if s.Component != "" {
			return s.Component, true
		}
		return "", false
	})

	if component != "" && !u.SliceContainsString(components, component) {
		return fmt.Errorf("the flag '--component %s' is passed, but the component is not defined in any of the `sources` in the vendor config file '%s'",
			component,
			cfg.AtmosVendorConfigFileName,
		)
	}

	duplicatedComponents := lo.FindDuplicates(components)

	if len(duplicatedComponents) > 0 {
		return fmt.Errorf("dublicated components %v in `sources` in the vendor config file '%s'",
			duplicatedComponents,
			cfg.AtmosVendorConfigFileName,
		)
	}

	for _, s := range atmosVendorSpec.Sources {
		if component != "" && s.Component != component {
			continue
		}

		if s.Source == "" {
			return fmt.Errorf("'source' must be specified in 'sources' in the vendor config file '%s'",
				cfg.AtmosVendorConfigFileName,
			)
		}

		if len(s.Targets) == 0 {
			return fmt.Errorf("'targets' must be specified for the source '%s' in the vendor config file '%s'",
				s.Source,
				cfg.AtmosVendorConfigFileName,
			)
		}

		// Parse 'source' template
		if s.Version != "" {
			uri, err = u.ProcessTmpl(fmt.Sprintf("source-%s", s.Version), s.Source, s.Source, false)
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
		for _, tgt := range s.Targets {
			var target string
			// Parse 'target' template
			if s.Version != "" {
				target, err = u.ProcessTmpl(fmt.Sprintf("target-%s", s.Version), tgt, tgt, false)
				if err != nil {
					return err
				}
			} else {
				target = tgt
			}

			// Check if `target` is a file path.
			// If it's a file path, check if it's an absolute path.
			if !useOciScheme {
				if absPath, err := u.JoinAbsolutePathWithPath(".", target); err == nil {
					target = absPath
				}
			}

			if component != "" {
				u.LogInfo(cliConfig, fmt.Sprintf("Pulling sources for the component '%s' from '%s' into '%s'\n",
					component,
					uri,
					target,
				))
			} else {
				u.LogInfo(cliConfig, fmt.Sprintf("Pulling sources from '%s' into '%s'\n",
					uri,
					target,
				))
			}

			if !dryRun {
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
	}
	return nil
}

// ExecuteStackVendorInternal executes the command to vendor an Atmos stack
// TODO: implement this
func ExecuteStackVendorInternal(
	stack string,
	dryRun bool,
) error {
	return fmt.Errorf("command 'atmos vendor pull --stack <stack>' is not implemented yet")
}
