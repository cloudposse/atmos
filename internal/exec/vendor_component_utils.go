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
	"time"

	"github.com/Masterminds/sprig/v3"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/hairyhenderson/gomplate/v3"
	"github.com/hashicorp/go-getter"
	"github.com/jfrog/jfrog-client-go/utils/log"
	cp "github.com/otiai10/copy"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const ociScheme = "oci://"

var (
	ErrMissingMixinURI             = errors.New("'uri' must be specified for each 'mixin' in the 'component.yaml' file")
	ErrMissingMixinFilename        = errors.New("'filename' must be specified for each 'mixin' in the 'component.yaml' file")
	ErrUnsupportedComponentType    = errors.New("'%s' is not supported type. Valid types are 'terraform' and 'helmfile'")
	ErrMixinEmpty                  = errors.New("mixin URI cannot be empty")
	ErrMixinNotImplemented         = errors.New("local mixin installation not implemented")
	ErrStackPullNotSupported       = errors.New("command 'atmos vendor pull --stack <stack>' is not supported yet")
	ErrComponentConfigFileNotFound = errors.New("component vendoring config file does not exist in the folder")
	ErrFolderNotFound              = errors.New("folder does not exist")
)

// findComponentConfigFile identifies the component vendoring config file (`component.yaml` or `component.yml`).
func findComponentConfigFile(basePath, fileName string) (string, error) {
	componentConfigExtensions := []string{"yaml", "yml"}

	for _, ext := range componentConfigExtensions {
		configFilePath := filepath.Join(basePath, fmt.Sprintf("%s.%s", fileName, ext))
		if u.FileExists(configFilePath) {
			return configFilePath, nil
		}
	}
	return "", fmt.Errorf("%w:%s", ErrComponentConfigFileNotFound, basePath)
}

// ReadAndProcessComponentVendorConfigFile reads and processes the component vendoring config file `component.yaml`.
func ReadAndProcessComponentVendorConfigFile(
	atmosConfig *schema.AtmosConfiguration,
	component string,
	componentType string,
) (schema.VendorComponentConfig, string, error) {
	var componentBasePath string
	var componentConfig schema.VendorComponentConfig

	switch componentType {
	case "terraform":
		componentBasePath = atmosConfig.Components.Terraform.BasePath
	case "helmfile":
		componentBasePath = atmosConfig.Components.Helmfile.BasePath
	default:
		return componentConfig, "", fmt.Errorf("%s,%w", componentType, ErrUnsupportedComponentType)
	}
	componentPath := filepath.Join(atmosConfig.BasePath, componentBasePath, component)

	dirExists, err := u.IsDirectory(componentPath)
	if err != nil {
		return componentConfig, "", err
	}

	if !dirExists {
		return componentConfig, "", fmt.Errorf("%w:%s", ErrFolderNotFound, componentPath)
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

// ExecuteComponentVendorInternal executes the 'atmos vendor pull' command for a component.
// Supports all protocols (local files, Git, Mercurial, HTTP, HTTPS, Amazon S3, Google GCP).
// URL and archive formats described in https://github.com/hashicorp/go-getter.
// https://www.allee.xyz/en/posts/getting-started-with-go-getter.
// https://github.com/otiai10/copy.
// https://opencontainers.org/.
// https://github.com/google/go-containerregistry.
// https://docs.aws.amazon.com/AmazonECR/latest/public/public-registries.html.

// ExecuteStackVendorInternal executes the command to vendor an Atmos stack.
// TODO: implement this.
func ExecuteStackVendorInternal(
	stack string,
	dryRun bool,
) error {
	return ErrStackPullNotSupported
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
					log.Debug("Excluding the file since it matches the '%s' pattern from 'excluded_paths'", "path", trimmedSrc, "excluded_paths", excludePath)
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
						log.Debug("Including path since it matches the pattern from 'included_paths'\n", "path", trimmedSrc, "included_paths", includePath)
						anyMatches = true
						break
					}
				}

				if anyMatches {
					return false, nil
				} else {
					log.Debug("Excluding since it does not match any pattern from 'included_paths'", "path", trimmedSrc)
					return true, nil
				}
			}

			// If 'included_paths' is not provided, include all files that were not excluded
			log.Debug("Including", u.TrimBasePathFromPath(tempDir+"/", src))
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
	atmosConfig *schema.AtmosConfiguration,
	vendorComponentSpec *schema.VendorComponentSpec,
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
	if strings.HasPrefix(uri, ociScheme) {
		useOciScheme = true
		uri = strings.TrimPrefix(uri, ociScheme)
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
	pType := determinePackageType(useOciScheme, useLocalFileSystem)
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
				return ErrMissingMixinURI
			}

			if mixin.Filename == "" {
				return ErrMissingMixinFilename
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
			if strings.HasPrefix(uri, ociScheme) {
				useOciScheme = true
				uri = strings.TrimPrefix(uri, ociScheme)
			}

			// Check if `uri` is a file path.
			// If it's a file path, check if it's an absolute path.
			// If it's not absolute path, join it with the base path (component dir) and convert to absolute path.
			if !useOciScheme {
				if absPath, err := u.JoinAbsolutePathWithPath(componentPath, uri); err == nil {
					uri = absPath
				}
			}
			// Check if it's a local file .
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
	if len(packages) > 0 {
		return executeVendorModel(packages, dryRun, atmosConfig)
	}
	return nil
}

func downloadComponentAndInstall(p *pkgComponentVendor, dryRun bool, atmosConfig *schema.AtmosConfiguration) tea.Cmd {
	return func() tea.Msg {
		if dryRun {
			// Simulate the action
			time.Sleep(100 * time.Millisecond)
			return installedPkgMsg{
				err:  nil,
				name: p.name,
			}
		}
		if p.IsComponent {
			err := installComponent(p, atmosConfig)
			if err != nil {
				return installedPkgMsg{
					err:  err,
					name: p.name,
				}
			}
			return installedPkgMsg{
				err:  nil,
				name: p.name,
			}
		}
		if p.IsMixins {
			err := installMixin(p, atmosConfig)
			if err != nil {
				return installedPkgMsg{
					err:  err,
					name: p.name,
				}
			}
			return installedPkgMsg{
				err:  nil,
				name: p.name,
			}
		}
		return installedPkgMsg{
			err:  fmt.Errorf("%w %s for package %s", ErrUnknownPackageType, p.pkgType.String(), p.name),
			name: p.name,
		}
	}
}

func installComponent(p *pkgComponentVendor, atmosConfig *schema.AtmosConfiguration) error {
	// Create temp folder
	// We are using a temp folder for the following reasons:
	// 1. 'git' does not clone into an existing folder (and we have the existing component folder with `component.yaml` in it)
	// 2. We have the option to skip some files we don't need and include only the files we need when copying from the temp folder to the destination folder
	tempDir, err := os.MkdirTemp("", fmt.Sprintf("atmos-vendor-%d-*", time.Now().Unix()))
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Ensure directory permissions are restricted
	if err := os.Chmod(tempDir, tempDirPermissions); err != nil {
		return fmt.Errorf("failed to set temp directory permissions: %w", err)
	}

	defer removeTempDir(*atmosConfig, tempDir)

	switch p.pkgType {
	case pkgTypeRemote:
		tempDir = filepath.Join(tempDir, SanitizeFileName(p.uri))

		if err = GoGetterGet(*atmosConfig, p.uri, tempDir, getter.ClientModeAny, 10*time.Minute); err != nil {
			return fmt.Errorf("failed to download package %s error %w", p.name, err)
		}

	case pkgTypeOci:
		// Download the Image from the OCI-compatible registry, extract the layers from the tarball, and write to the destination directory
		err = processOciImage(*atmosConfig, p.uri, tempDir)
		if err != nil {
			return fmt.Errorf("Failed to process OCI image %s error %w", p.name, err)
		}

	case pkgTypeLocal:
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
		if p.sourceIsLocalFile {
			tempDir2 = filepath.Join(tempDir, SanitizeFileName(p.uri))
		}

		if err = cp.Copy(p.uri, tempDir2, copyOptions); err != nil {
			return fmt.Errorf("failed to copy package %s error %w", p.name, err)
		}
	default:
		return fmt.Errorf("%w %s for package %s", ErrUnknownPackageType, p.pkgType.String(), p.name)
	}
	if err = copyComponentToDestination(*atmosConfig, tempDir, p.componentPath, *p.vendorComponentSpec, p.sourceIsLocalFile, p.uri); err != nil {
		return fmt.Errorf("failed to copy package %s error %w", p.name, err)
	}

	return nil
}

func installMixin(p *pkgComponentVendor, atmosConfig *schema.AtmosConfiguration) error {
	tempDir, err := os.MkdirTemp("", "atmos-vendor-mixin")
	if err != nil {
		return fmt.Errorf("Failed to create temp directory %w", err)
	}

	defer removeTempDir(*atmosConfig, tempDir)

	switch p.pkgType {
	case pkgTypeRemote:
		if err = GoGetterGet(*atmosConfig, p.uri, filepath.Join(tempDir, p.mixinFilename), getter.ClientModeFile, 10*time.Minute); err != nil {
			return fmt.Errorf("failed to download package %s error %w", p.name, err)
		}

	case pkgTypeOci:
		// Download the Image from the OCI-compatible registry, extract the layers from the tarball, and write to the destination directory
		err = processOciImage(*atmosConfig, p.uri, tempDir)
		if err != nil {
			return fmt.Errorf("failed to process OCI image %s error %w", p.name, err)
		}

	case pkgTypeLocal:
		if p.uri == "" {
			return ErrMixinEmpty
		}
		// Implement local mixin installation logic
		return ErrMixinNotImplemented

	default:
		return fmt.Errorf("%w %s for package %s", ErrUnknownPackageType, p.pkgType.String(), p.name)
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

	if err := cp.Copy(tempDir, p.componentPath, copyOptions); err != nil {
		return fmt.Errorf("Failed to copy package %s error %w", p.name, err)
	}

	return nil
}
