package exec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/bmatcuk/doublestar/v4"
	c "github.com/cloudposse/atmos/pkg/config"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/hashicorp/go-getter"
	cp "github.com/otiai10/copy"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"path"
	"strconv"
	"strings"
	"text/template"
	"time"
)

const (
	componentConfigFileName = "component.yaml"
)

// ExecuteVendorCommand executes `atmos vendor` commands
func ExecuteVendorCommand(cmd *cobra.Command, args []string, vendorCommand string) error {
	err := c.InitConfig()
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

	componentType, err := flags.GetString("type")
	if err != nil {
		return err
	}

	if componentType == "" {
		componentType = "terraform"
	}

	var componentBasePath string

	if componentType == "terraform" {
		componentBasePath = c.Config.Components.Terraform.BasePath
	} else if componentType == "helmfile" {
		componentBasePath = c.Config.Components.Helmfile.BasePath
	} else {
		return errors.New(fmt.Sprintf("type '%s' is not supported. Valid types are 'terraform' and 'helmfile'", componentType))
	}

	componentPath := path.Join(c.Config.BasePath, componentBasePath, component)

	dirExists, err := u.IsDirectory(componentPath)
	if err != nil {
		return err
	}

	if !dirExists {
		return errors.New(fmt.Sprintf("Folder '%s' does not exist", componentPath))
	}

	componentConfigFile := path.Join(componentPath, componentConfigFileName)
	if !u.FileExists(componentConfigFile) {
		return errors.New(fmt.Sprintf("Vendor config file '%s' does not exist in the '%s' folder", componentConfigFileName, componentPath))
	}

	componentConfigFileContent, err := ioutil.ReadFile(componentConfigFile)
	if err != nil {
		return err
	}

	var componentConfig c.VendorComponentConfig
	if err = yaml.Unmarshal(componentConfigFileContent, &componentConfig); err != nil {
		return err
	}

	return executeVendorCommandInternal(componentConfig, component, componentPath, dryRun, vendorCommand)
}

// Supports all protocols (local files, Git, Mercurial, HTTP, HTTPS, Amazon S3, Google GCP),
// URL and archive formats described in https://github.com/hashicorp/go-getter
// https://www.allee.xyz/en/posts/getting-started-with-go-getter
// https://github.com/otiai10/copy
func executeVendorCommandInternal(
	componentConfig c.VendorComponentConfig,
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
		if componentConfig.Source.Uri == "" {
			return errors.New("'uri' must be specified in 'source.uri' in the 'component.yaml' file")
		}

		// Parse 'uri' template
		if componentConfig.Source.Version != "" {
			t, err = template.New(fmt.Sprintf("source-uri-%s", componentConfig.Source.Version)).Parse(componentConfig.Source.Uri)
			if err != nil {
				return err
			}

			var tpl bytes.Buffer
			err = t.Execute(&tpl, componentConfig.Source)
			if err != nil {
				return err
			}

			uri = tpl.String()
		} else {
			uri = componentConfig.Source.Uri
		}

		u.PrintInfo(fmt.Sprintf("Pulling sources for the component '%s' from '%s' and writing to '%s'\n",
			component,
			uri,
			componentPath,
		))

		if !dryRun {
			// Create temp folder
			// We are using a temp folder for the following reasons:
			// 1. 'git' does not clone into an existing folder (and we have the existing component folder with `component.yaml` in it)
			// 2. We have the option to skip some files we don't need and include only the files we need when copying from the temp folder to the destination folder
			tempDir, err = ioutil.TempDir("", strconv.FormatInt(time.Now().Unix(), 10))
			if err != nil {
				return err
			}

			defer func(path string) {
				err := os.RemoveAll(path)
				if err != nil {
					u.PrintError(err)
				}
			}(tempDir)

			// Download the source into the temp folder
			client := &getter.Client{
				Ctx: context.Background(),
				// Define the destination to where the files will be stored. This will create the directory if it doesn't exist
				Dst: tempDir,
				Dir: true,
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
				Skip: func(src string) (bool, error) {
					if strings.HasSuffix(src, ".git") {
						return true, nil
					}

					// Exclude the files that match the 'excluded_paths' patterns
					// It supports POSIX-style Globs for file names/paths (double-star `**` is supported)
					// https://en.wikipedia.org/wiki/Glob_(programming)
					// https://github.com/bmatcuk/doublestar#patterns
					for _, excludePath := range componentConfig.Source.ExcludedPaths {
						excludeMatch, err := doublestar.PathMatch(excludePath, src)
						if err != nil {
							return true, err
						} else if excludeMatch {
							// If the file matches ANY of the 'excluded_paths' patterns, exclude the file
							fmt.Println(fmt.Sprintf("Excluding the file '%s' since it matches the '%s' pattern from 'excluded_paths'",
								u.TrimBasePathFromPath(tempDir+"/", src),
								excludePath,
							))
							return true, nil
						}
					}

					// Only include the files that match the 'included_paths' patterns
					if len(componentConfig.Source.IncludedPaths) > 0 {
						anyMatches := false
						for _, includePath := range componentConfig.Source.IncludedPaths {
							includeMatch, err := doublestar.PathMatch(includePath, src)
							if err != nil {
								return true, err
							} else if includeMatch {
								// If the file matches ANY of the 'included_paths' patterns, include the file
								fmt.Println(fmt.Sprintf("Including the file '%s' since it matches the '%s' pattern from 'included_paths'",
									u.TrimBasePathFromPath(tempDir+"/", src),
									includePath,
								))
								anyMatches = true
								continue
							}
						}

						if anyMatches {
							return false, nil
						} else {
							fmt.Println(fmt.Sprintf("Excluding the file '%s' since it does not match any pattern from 'included_paths'",
								u.TrimBasePathFromPath(tempDir+"/", src),
							))
							return true, nil
						}
					}

					// If 'included_paths' is not provided, include all files that were not excluded
					fmt.Println(fmt.Sprintf("Including the file '%s'",
						u.TrimBasePathFromPath(tempDir+"/", src),
					))
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
		if len(componentConfig.Mixins) > 0 {
			fmt.Println()

			for _, mixing := range componentConfig.Mixins {
				if mixing.Uri == "" {
					return errors.New("'uri' must be specified for each 'mixin' in the 'component.yaml' file")
				}

				if mixing.Filename == "" {
					return errors.New("'filename' must be specified for each 'mixin' in the 'component.yaml' file")
				}

				// Parse 'uri' template
				if mixing.Version != "" {
					t, err = template.New(fmt.Sprintf("mixing-uri-%s", mixing.Version)).Parse(mixing.Uri)
					if err != nil {
						return err
					}

					var tpl bytes.Buffer
					err = t.Execute(&tpl, mixing)
					if err != nil {
						return err
					}

					uri = tpl.String()
				} else {
					uri = mixing.Uri
				}

				u.PrintInfo(fmt.Sprintf("Pulling the mixing '%s' for the component '%s' and writing to '%s'\n",
					uri,
					component,
					componentPath,
				))

				if !dryRun {
					err = os.RemoveAll(tempDir)
					if err != nil {
						return err
					}

					// Download the mixing into the temp file
					client := &getter.Client{
						Ctx:  context.Background(),
						Dst:  path.Join(tempDir, mixing.Filename),
						Dir:  false,
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
