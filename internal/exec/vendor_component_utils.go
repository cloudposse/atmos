package exec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/Masterminds/sprig/v3"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/hairyhenderson/gomplate/v3"
	"github.com/hashicorp/go-getter"
	cp "github.com/otiai10/copy"
)

// findComponentConfigFile identifies the component vendoring config file (`component.yaml` or `component.yml`)
func findComponentConfigFile(basePath, fileName string) (string, error) {
	componentConfigExtensions := []string{"yaml", "yml"}

	for _, ext := range componentConfigExtensions {
		configFilePath := filepath.Join(basePath, fmt.Sprintf("%s.%s", fileName, ext))
		if u.FileExists(configFilePath) {
			return configFilePath, nil
		}
	}
	return "", fmt.Errorf("component vendoring config file does not exist in the '%s' folder", basePath)
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

	componentPath := filepath.Join(cliConfig.BasePath, componentBasePath, component)

	dirExists, err := u.IsDirectory(componentPath)
	if err != nil {
		return componentConfig, "", err
	}

	if !dirExists {
		return componentConfig, "", fmt.Errorf("folder '%s' does not exist", componentPath)
	}

	componentConfigFile, err := findComponentConfigFile(componentPath, strings.TrimSuffix(cfg.ComponentVendorConfigFileName, ".yaml"))
	if err != nil {
		return componentConfig, "", err
	}

	componentConfigFileContent, err := os.ReadFile(componentConfigFile)
	if err != nil {
		return componentConfig, "", err
	}

	componentConfig, err = u.UnmarshalYAML[schema.VendorComponentConfig](string(componentConfigFileContent))
	if err != nil {
		return componentConfig, "", err
	}

	if componentConfig.Kind != "ComponentVendorConfig" {
		return componentConfig, "", fmt.Errorf("invalid 'kind: %s' in the component vendoring config file '%s'. Supported kinds: 'ComponentVendorConfig'",
			componentConfig.Kind,
			cfg.ComponentVendorConfigFileName)
	}

	return componentConfig, componentPath, nil
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
		t, err = template.New(fmt.Sprintf("source-uri-%s", vendorComponentSpec.Source.Version)).Funcs(sprig.FuncMap()).Funcs(gomplate.CreateFuncs(context.Background(), nil)).Parse(vendorComponentSpec.Source.Uri)
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

	useOciScheme := false
	useLocalFileSystem := false
	sourceIsLocalFile := false

	// Check if `uri` uses the `oci://` scheme (to download the sources from an OCI-compatible registry).
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
			useLocalFileSystem = true

			if u.FileExists(uri) {
				sourceIsLocalFile = true
			}
		}
	}

	u.LogInfo(cliConfig, fmt.Sprintf("Pulling sources for the component '%s' from '%s' into '%s'",
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
		if useOciScheme {
			// Download the Image from the OCI-compatible registry, extract the layers from the tarball, and write to the destination directory
			err = processOciImage(cliConfig, uri, tempDir)
			if err != nil {
				return err
			}
		} else if useLocalFileSystem {
			copyOptions := cp.Options{
				PreserveTimes: false,
				PreserveOwner: false,
				// OnSymlink specifies what to do on symlink
				// Override the destination file if it already exists
				OnSymlink: func(src string) cp.SymlinkAction {
					return cp.Deep
				},
			}

			tempDir2 := tempDir
			if sourceIsLocalFile {
				tempDir2 = filepath.Join(tempDir, filepath.Base(uri))
			}

			if err = cp.Copy(uri, tempDir2, copyOptions); err != nil {
				return err
			}
		} else {
			// Use `go-getter` to download the sources into the temp directory
			// When cloning from the root of a repo w/o using modules (sub-paths), `go-getter` does the following:
			// - If the destination directory does not exist, it creates it and runs `git init`
			// - If the destination directory exists, it should be an already initialized Git repository (otherwise an error will be thrown)
			// For more details, refer to
			// - https://github.com/hashicorp/go-getter/issues/114
			// - https://github.com/hashicorp/go-getter?tab=readme-ov-file#subdirectories
			// We add the `uri` to the already created `tempDir` so it does not exist to allow `go-getter` to create
			// and correctly initialize it
			tempDir = filepath.Join(tempDir, filepath.Base(uri))

			client := &getter.Client{
				Ctx: context.Background(),
				// Define the destination where the files will be stored. This will create the directory if it doesn't exist
				Dst: tempDir,
				// Source
				Src:  uri,
				Mode: getter.ClientModeAny,
			}

			if err = client.Get(); err != nil {
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

			// OnSymlink specifies what to do on symlink
			// Override the destination file if it already exists
			OnSymlink: func(src string) cp.SymlinkAction {
				return cp.Deep
			},
		}

		componentPath2 := componentPath
		if sourceIsLocalFile {
			if filepath.Ext(componentPath) == "" {
				componentPath2 = filepath.Join(componentPath, filepath.Base(uri))
			}
		}

		if err = cp.Copy(tempDir, componentPath2, copyOptions); err != nil {
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
				t, err = template.New(fmt.Sprintf("mixin-uri-%s", mixin.Version)).Funcs(sprig.FuncMap()).Funcs(gomplate.CreateFuncs(context.Background(), nil)).Parse(mixin.Uri)
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
				filepath.Join(componentPath, mixin.Filename),
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
						Dst:  filepath.Join(tempDir, mixin.Filename),
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

// ExecuteStackVendorInternal executes the command to vendor an Atmos stack
// TODO: implement this
func ExecuteStackVendorInternal(
	stack string,
	dryRun bool,
) error {
	return fmt.Errorf("command 'atmos vendor pull --stack <stack>' is not supported yet")
}
