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

	"github.com/hashicorp/go-getter"
	cp "github.com/otiai10/copy"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ExecuteVendorCommand executes `atmos vendor` commands
func ExecuteVendorCommand(cmd *cobra.Command, args []string, vendorCommand string) error {
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
		return fmt.Errorf("either '--component' or '--stack' parameter needs to be provided, but not both")
	}

	if component != "" {
		// Process component vendoring
		componentType, err := flags.GetString("type")
		if err != nil {
			return err
		}

		if componentType == "" {
			componentType = "terraform"
		}

		componentConfig, componentPath, err := ReadAndProcessComponentConfigFile(cliConfig, component, componentType)
		if err != nil {
			return err
		}

		return ExecuteComponentVendorCommandInternal(cliConfig, componentConfig.Spec, component, componentPath, dryRun, vendorCommand)
	} else {
		// Process stack vendoring
		return ExecuteStackVendorCommandInternal(stack, dryRun, vendorCommand)
	}
}

// ReadAndProcessComponentConfigFile reads and processes `component.yaml` vendor config file
func ReadAndProcessComponentConfigFile(cliConfig schema.CliConfiguration, component string, componentType string) (schema.VendorComponentConfig, string, error) {
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

	componentConfigFile := path.Join(componentPath, cfg.ComponentConfigFileName)
	if !u.FileExists(componentConfigFile) {
		return componentConfig, "", fmt.Errorf("vendor config file '%s' does not exist in the '%s' folder", cfg.ComponentConfigFileName, componentPath)
	}

	componentConfigFileContent, err := os.ReadFile(componentConfigFile)
	if err != nil {
		return componentConfig, "", err
	}

	if err = yaml.Unmarshal(componentConfigFileContent, &componentConfig); err != nil {
		return componentConfig, "", err
	}

	if componentConfig.Kind != "ComponentVendorConfig" {
		return componentConfig, "", fmt.Errorf("invalid 'kind: %s' in the vendor config file '%s'. Supported kinds: 'ComponentVendorConfig'",
			componentConfig.Kind,
			cfg.ComponentConfigFileName)
	}

	return componentConfig, componentPath, nil
}

// ExecuteComponentVendorCommandInternal executes a component vendor command
// Supports all protocols (local files, Git, Mercurial, HTTP, HTTPS, Amazon S3, Google GCP),
// URL and archive formats described in https://github.com/hashicorp/go-getter
// https://www.allee.xyz/en/posts/getting-started-with-go-getter
// https://github.com/otiai10/copy
func ExecuteComponentVendorCommandInternal(
	cliConfig schema.CliConfiguration,
	vendorComponentSpec schema.VendorComponentSpec,
	component string,
	componentPath string,
	dryRun bool,
	vendorCommand string,
) error {

	var tempDir string
	var err error
	var t *template.Template
	var uri string

	if vendorCommand == "pull" {
		if vendorComponentSpec.Source.Uri == "" {
			return errors.New("'uri' must be specified in 'source.uri' in the 'component.yaml' file")
		}

		// Parse 'uri' template
		if vendorComponentSpec.Source.Version != "" {
			t, err = template.New(fmt.Sprintf("source-uri-%s", vendorComponentSpec.Source.Version)).Parse(vendorComponentSpec.Source.Uri)
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

		// Check if `uri` is a file path.
		// If it's a file path, check if it's an absolute path.
		// If it's not absolute path, join it with the base path (component dir) and convert to absolute path.
		if absPath, err := u.JoinAbsolutePathWithPath(componentPath, uri); err == nil {
			uri = absPath
		}

		u.LogInfo(fmt.Sprintf("Pulling sources for the component '%s' from '%s' and writing to '%s'\n",
			component,
			uri,
			componentPath,
		))

		if !dryRun {
			// Create temp folder
			// We are using a temp folder for the following reasons:
			// 1. 'git' does not clone into an existing folder (and we have the existing component folder with `component.yaml` in it)
			// 2. We have the option to skip some files we don't need and include only the files we need when copying from the temp folder to the destination folder
			// ioutil.TempDir is deprecated. As of Go 1.17, this function simply calls os.MkdirTemp
			tempDir, err = os.MkdirTemp("", strconv.FormatInt(time.Now().Unix(), 10))
			if err != nil {
				return err
			}

			defer removeTempDir(cliConfig, tempDir)

			// Download the source into the temp folder
			client := &getter.Client{
				Ctx: context.Background(),
				// Define the destination to where the files will be stored. This will create the directory if it doesn't exist
				Dst: tempDir,
				// Source
				Src:  uri,
				Mode: getter.ClientModeDir,
			}

			if err = client.Get(); err != nil {
				return err
			}

			// Copy from the temp folder to the destination folder with skipping of some files
			copyOptions := cp.Options{
				// Skip specifies which files should be skipped
				Skip: func(srcinfo os.FileInfo, src, dest string) (bool, error) {
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
							u.LogMessage(fmt.Sprintf("Excluding the file '%s' since it matches the '%s' pattern from 'excluded_paths'\n",
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
								u.LogMessage(fmt.Sprintf("Including '%s' since it matches the '%s' pattern from 'included_paths'\n",
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
							u.LogMessage(fmt.Sprintf("Excluding '%s' since it does not match any pattern from 'included_paths'\n", trimmedSrc))
							return true, nil
						}
					}

					// If 'included_paths' is not provided, include all files that were not excluded
					u.LogMessage(fmt.Sprintf("Including '%s'\n", u.TrimBasePathFromPath(tempDir+"/", src)))
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
					t, err = template.New(fmt.Sprintf("mixin-uri-%s", mixin.Version)).Parse(mixin.Uri)
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

				// Check if `uri` is a file path.
				// If it's a file path, check if it's an absolute path.
				// If it's not absolute path, join it with the base path (component dir) and convert to absolute path.
				if absPath, err := u.JoinAbsolutePathWithPath(componentPath, uri); err == nil {
					uri = absPath
				}

				u.LogInfo(fmt.Sprintf("Pulling the mixin '%s' for the component '%s' and writing to '%s'\n",
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
					client := &getter.Client{
						Ctx:  context.Background(),
						Dst:  path.Join(tempDir, mixin.Filename),
						Src:  uri,
						Mode: getter.ClientModeFile,
					}

					if err = client.Get(); err != nil {
						return err
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
	}

	return nil
}

// ExecuteStackVendorCommandInternal executes a stack vendor command
// TODO: implement this
func ExecuteStackVendorCommandInternal(
	stack string,
	dryRun bool,
	vendorCommand string,
) error {
	return fmt.Errorf("command 'atmos vendor %s --stack <stack>' is not implemented yet", vendorCommand)
}
