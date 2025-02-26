package exec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/hairyhenderson/gomplate/v3"
	cp "github.com/otiai10/copy"

	"github.com/cloudposse/atmos/internal/tui/templates/term"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
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
	atmosConfig schema.AtmosConfiguration,
	component string,
	componentType string,
) (schema.VendorComponentConfig, string, error) {
	var componentBasePath string
	var componentConfig schema.VendorComponentConfig

	if componentType == "terraform" {
		componentBasePath = atmosConfig.Components.Terraform.BasePath
	} else if componentType == "helmfile" {
		componentBasePath = atmosConfig.Components.Helmfile.BasePath
	} else {
		return componentConfig, "", fmt.Errorf("type '%s' is not supported. Valid types are 'terraform' and 'helmfile'", componentType)
	}

	componentPath := filepath.Join(atmosConfig.BasePath, componentBasePath, component)

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

// ExecuteStackVendorInternal executes the command to vendor an Atmos stack
// TODO: implement this
func ExecuteStackVendorInternal(
	stack string,
	dryRun bool,
) error {
	return fmt.Errorf("command 'atmos vendor pull --stack <stack>' is not supported yet")
}

func copyComponentToDestination(atmosConfig schema.AtmosConfiguration, tempDir, componentPath string, vendorComponentSpec schema.VendorComponentSpec, sourceIsLocalFile bool, uri string) error {
	// Copy from the temp folder to the destination folder and skip the excluded files
	copyOptions := cp.Options{
		// Skip specifies which files should be skipped
		Skip: func(srcInfo os.FileInfo, src, dest string) (bool, error) {
			if filepath.Base(src) == ".git" {
				return true, nil
			}

			trimmedSrc := u.TrimBasePathFromPath(tempDir+"/", src)

			// Exclude the files that match the 'excluded_paths' patterns
			// It supports POSIX-style Globs for file names/paths (double-star `**` is supported)
			// https://en.wikipedia.org/wiki/Glob_(programming)
			// https://github.com/bmatcuk/doublestar#patterns
			for _, excludePath := range vendorComponentSpec.Source.ExcludedPaths {
				excludePath := filepath.Clean(excludePath)
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
			if len(vendorComponentSpec.Source.IncludedPaths) > 0 {
				anyMatches := false
				for _, includePath := range vendorComponentSpec.Source.IncludedPaths {
					includePath := filepath.Clean(includePath)
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
			componentPath2 = filepath.Join(componentPath, SanitizeFileName(uri))
		}
	}

	if err := cp.Copy(tempDir, componentPath2, copyOptions); err != nil {
		return err
	}
	return nil
}

func ExecuteComponentVendorInternal(
	atmosConfig schema.AtmosConfiguration,
	vendorComponentSpec schema.VendorComponentSpec,
	component string,
	componentPath string,
	dryRun bool,
) error {
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
		u, err := url.Parse(uri)
		if err == nil && u.Scheme != "" {
			if u.Scheme == "file" {
				trimmedPath := strings.TrimPrefix(filepath.ToSlash(u.Path), "/")
				uri = filepath.Clean(trimmedPath)
				useLocalFileSystem = true
			}
		}
	}

	var pType pkgType
	if useOciScheme {
		pType = pkgTypeOci
	} else if useLocalFileSystem {
		pType = pkgTypeLocal
	} else {
		pType = pkgTypeRemote
	}

	componentPkg := pkgComponentVendor{
		uri:                 uri,
		name:                component,
		componentPath:       componentPath,
		sourceIsLocalFile:   sourceIsLocalFile,
		pkgType:             pType,
		version:             vendorComponentSpec.Source.Version,
		vendorComponentSpec: vendorComponentSpec,
		IsComponent:         true,
	}

	var packages []pkgComponentVendor
	packages = append(packages, componentPkg)

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
			// Check if it's a local file
			if absPath, err := u.JoinAbsolutePathWithPath(componentPath, uri); err == nil {
				if u.FileExists(absPath) {
					pType = pkgTypeLocal
					continue
				}
			}
			if useOciScheme {
				pType = pkgTypeOci
			} else {
				pType = pkgTypeRemote
			}

			pkg := pkgComponentVendor{
				uri:                 uri,
				pkgType:             pType,
				name:                "mixin " + uri,
				sourceIsLocalFile:   false,
				IsMixins:            true,
				vendorComponentSpec: vendorComponentSpec,
				version:             mixin.Version,
				componentPath:       componentPath,
				mixinFilename:       mixin.Filename,
			}

			packages = append(packages, pkg)
		}
	}

	// Run TUI to process packages
	if len(packages) > 0 {
		model := newModelComponentVendorInternal(packages, dryRun, &atmosConfig)
		var opts []tea.ProgramOption
		// Disable TUI if no TTY support is available
		if !term.IsTTYSupportForStdout() {
			opts = []tea.ProgramOption{tea.WithoutRenderer(), tea.WithInput(nil)}
			u.LogWarning("TTY is not supported. Running in non-interactive mode")
		}
		if _, err := tea.NewProgram(&model, opts...).Run(); err != nil {
			return fmt.Errorf("running download error: %w", err)
		}
	}
	return nil
}
