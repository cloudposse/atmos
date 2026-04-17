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

	"github.com/cloudposse/atmos/pkg/perf"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hairyhenderson/gomplate/v3"
	"github.com/jfrog/jfrog-client-go/utils/log"
	cp "github.com/otiai10/copy"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/downloader"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/cloudposse/atmos/pkg/vendor"
)

const ociScheme = "oci://"

var (
	ErrMissingMixinURI             = errors.New("'uri' must be specified for each 'mixin' in the 'component.yaml' file")
	ErrMissingMixinFilename        = errors.New("'filename' must be specified for each 'mixin' in the 'component.yaml' file")
	ErrMixinEmpty                  = errors.New("mixin URI cannot be empty")
	ErrMixinNotImplemented         = errors.New("local mixin installation not implemented")
	ErrStackPullNotSupported       = errors.New("command 'atmos vendor pull --stack <stack>' is not supported yet")
	ErrComponentConfigFileNotFound = errors.New("component vendoring config file does not exist in the folder")
	ErrFolderNotFound              = errors.New("folder does not exist")
	ErrInvalidComponentKind        = errors.New("invalid 'kind' in the component vendoring config file. Supported kinds: 'ComponentVendorConfig'")
	ErrUriMustSpecified            = errors.New("'uri' must be specified in 'source.uri' in the component vendoring config file")
)

type ComponentSkipFunc func(os.FileInfo, string, string) (bool, error)

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
	defer perf.Track(atmosConfig, "exec.ReadAndProcessComponentVendorConfigFile")()

	var componentBasePath string
	var componentConfig schema.VendorComponentConfig

	switch componentType {
	case cfg.TerraformComponentType:
		componentBasePath = atmosConfig.Components.Terraform.BasePath
	case cfg.HelmfileComponentType:
		componentBasePath = atmosConfig.Components.Helmfile.BasePath
	case cfg.PackerComponentType:
		componentBasePath = atmosConfig.Components.Packer.BasePath
	default:
		return componentConfig, "", fmt.Errorf("%s,%w", componentType, errUtils.ErrUnsupportedComponentType)
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
		return componentConfig, "", fmt.Errorf("%w: '%s' in file '%s'", ErrInvalidComponentKind, componentConfig.Kind, cfg.ComponentVendorConfigFileName)
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
	defer perf.Track(nil, "exec.ExecuteStackVendorInternal")()

	return ErrStackPullNotSupported
}

func copyComponentToDestination(tempDir, componentPath string, vendorComponentSpec *schema.VendorComponentSpec) error {
	return vendor.CopyToTarget(tempDir, componentPath, vendor.CopyOptions{
		IncludedPaths: vendorComponentSpec.Source.IncludedPaths,
		ExcludedPaths: vendorComponentSpec.Source.ExcludedPaths,
	})
}

// createComponentSkipFunc creates a skip function for component vendoring.
// Delegates to pkg/vendor for the shared implementation.
func createComponentSkipFunc(tempDir string, vendorComponentSpec *schema.VendorComponentSpec) ComponentSkipFunc {
	return vendor.CreateSkipFunc(tempDir, vendorComponentSpec.Source.IncludedPaths, vendorComponentSpec.Source.ExcludedPaths)
}

// checkComponentExcludes checks if the file matches any of the excluded patterns.
// Delegates to pkg/vendor for the shared implementation.
func checkComponentExcludes(excludePaths []string, src, trimmedSrc string) (bool, error) {
	return vendor.ShouldExcludeFile(excludePaths, trimmedSrc)
}

// checkComponentIncludes checks if the file matches any of the included patterns.
// Delegates to pkg/vendor for the shared implementation.
func checkComponentIncludes(includePaths []string, src, trimmedSrc string) (bool, error) {
	return vendor.ShouldIncludeFile(includePaths, trimmedSrc)
}

func ExecuteComponentVendorInternal(
	atmosConfig *schema.AtmosConfiguration,
	vendorComponentSpec *schema.VendorComponentSpec,
	component string,
	componentPath string,
	dryRun bool,
) error {
	defer perf.Track(atmosConfig, "exec.ExecuteComponentVendorInternal")()

	if vendorComponentSpec.Source.Uri == "" {
		return fmt.Errorf("%w:'%s'", ErrUriMustSpecified, cfg.ComponentVendorConfigFileName)
	}
	uri := vendorComponentSpec.Source.Uri
	// Parse 'uri' template
	if vendorComponentSpec.Source.Version != "" {
		t, err := template.New(fmt.Sprintf("source-uri-%s", vendorComponentSpec.Source.Version)).Funcs(getSprigFuncMap()).Funcs(gomplate.CreateFuncs(context.Background(), nil)).Parse(vendorComponentSpec.Source.Uri)
		if err != nil {
			return err
		}
		var tpl bytes.Buffer
		err = t.Execute(&tpl, vendorComponentSpec.Source)
		if err != nil {
			return err
		}
		uri = tpl.String()
	}
	var useOciScheme, useLocalFileSystem, sourceIsLocalFile bool

	// Check if `uri` uses the `oci://` scheme (to download the sources from an OCI-compatible registry).
	if strings.HasPrefix(uri, ociScheme) {
		useOciScheme = true
		uri = strings.TrimPrefix(uri, ociScheme)
	}

	if !useOciScheme {
		uri, useLocalFileSystem, sourceIsLocalFile = handleLocalFileScheme(componentPath, uri)
	}
	pType := determinePackageType(useOciScheme, useLocalFileSystem)

	// Warn if the source is an archived GitHub repository.
	// TODO: thread context.Context through the vendor pipeline so this check cancels on Ctrl+C.
	// See https://github.com/cloudposse/atmos/issues for the tracking issue.
	if pType == pkgTypeRemote {
		warnIfArchivedGitHubRepo(context.Background(), uri, component)
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
		mixinPkgs, err := processComponentMixins(vendorComponentSpec, componentPath)
		if err != nil {
			return err
		}
		packages = append(packages, mixinPkgs...)
	}
	if len(packages) > 0 {
		return executeVendorModel(packages, dryRun, atmosConfig)
	}
	return nil
}

// handleLocalFileScheme processes the URI for local file system paths.
// Check if `uri` is a file path.
// If it's a file path, check if it's an absolute path.
// If it's not absolute path, join it with the base path (component dir) and convert to absolute path.
func handleLocalFileScheme(componentPath, uri string) (string, bool, bool) {
	var useLocalFileSystem, sourceIsLocalFile bool

	// Handle absolute path resolution
	if absPath, err := u.JoinPathAndValidate(componentPath, uri); err == nil {
		uri = absPath
		useLocalFileSystem = true
		sourceIsLocalFile = u.FileExists(uri)
	}

	// Handle file:// scheme
	if parsedURL, err := url.Parse(uri); err == nil && parsedURL.Scheme != "" {
		if parsedURL.Scheme == "file" {
			trimmedPath := strings.TrimPrefix(filepath.ToSlash(parsedURL.Path), "/")
			uri = filepath.Clean(trimmedPath)
			useLocalFileSystem = true
		}
	}

	return uri, useLocalFileSystem, sourceIsLocalFile
}

func processComponentMixins(vendorComponentSpec *schema.VendorComponentSpec, componentPath string) ([]pkgComponentVendor, error) {
	var packages []pkgComponentVendor
	for _, mixin := range vendorComponentSpec.Mixins {
		if mixin.Uri == "" {
			return nil, ErrMissingMixinURI
		}

		if mixin.Filename == "" {
			return nil, ErrMissingMixinFilename
		}

		// Parse 'uri' template
		uri, err := parseMixinURI(&mixin)
		if err != nil {
			return nil, err
		}
		pType := pkgTypeRemote
		// Check if `uri` uses the `oci://` scheme (to download the sources from an OCI-compatible registry).
		useOciScheme := false
		if strings.HasPrefix(uri, ociScheme) {
			useOciScheme = true
			pType = pkgTypeOci
			uri = strings.TrimPrefix(uri, ociScheme)
		}

		// Check if `uri` is a file path.
		// If it's a file path, check if it's an absolute path.
		// If it's not absolute path, join it with the base path (component dir) and convert to absolute path.
		if !useOciScheme {
			if absPath, err := u.JoinPathAndValidate(componentPath, uri); err == nil {
				uri = absPath
			}
		}
		// Check if it's a local file .
		if absPath, err := u.JoinPathAndValidate(componentPath, uri); err == nil {
			if u.FileExists(absPath) {
				continue
			}
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
	return packages, nil
}

func parseMixinURI(mixin *schema.VendorComponentMixins) (string, error) {
	if mixin.Version == "" {
		return mixin.Uri, nil
	}

	tmpl, err := template.New("mixin-uri").Funcs(getSprigFuncMap()).Funcs(gomplate.CreateFuncs(context.Background(), nil)).Parse(mixin.Uri)
	if err != nil {
		return "", err
	}

	var tpl bytes.Buffer
	if err := tmpl.Execute(&tpl, mixin); err != nil {
		return "", err
	}

	return tpl.String(), nil
}

func downloadComponentAndInstall(p *pkgComponentVendor, dryRun bool, atmosConfig *schema.AtmosConfiguration) tea.Cmd {
	return func() tea.Msg {
		if dryRun {
			if needsCustomDetection(p.uri) {
				log.Debug("Dry-run mode: custom detection required for component (or mixin) URI", "component", p.name, "uri", p.uri)
				detector := downloader.NewCustomGitDetector(atmosConfig, "")
				_, _, err := detector.Detect(p.uri, "")
				if err != nil {
					return installedPkgMsg{
						err:  fmt.Errorf("dry-run: detection failed for component %s: %w", p.name, err),
						name: p.name,
					}
				}
			} else {
				log.Debug("Dry-run mode: skipping custom detection; URI already supported by go-getter", "component", p.name, "uri", p.uri)
			}
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
			err:  fmt.Errorf("%w %s for package %s", errUtils.ErrUnknownPackageType, p.pkgType.String(), p.name),
			name: p.name,
		}
	}
}

func installComponent(p *pkgComponentVendor, atmosConfig *schema.AtmosConfiguration) error {
	// Create temp folder
	// We are using a temp folder for the following reasons:
	// 1. 'git' does not clone into an existing folder (and we have the existing component folder with `component.yaml` in it)
	// 2. We have the option to skip some files we don't need and include only the files we need when copying from the temp folder to the destination folder
	tempDir, err := createTempDir()
	if err != nil {
		return err
	}

	defer removeTempDir(tempDir)

	switch p.pkgType {
	case pkgTypeRemote:
		tempDir = filepath.Join(tempDir, SanitizeFileName(p.uri))

		opts := []downloader.GoGetterOption{}
		if p.vendorComponentSpec != nil && p.vendorComponentSpec.Source.Retry != nil {
			opts = append(opts, downloader.WithRetryConfig(p.vendorComponentSpec.Source.Retry))
		}
		if err := downloader.NewGoGetterDownloader(atmosConfig, opts...).Fetch(p.uri, tempDir, downloader.ClientModeAny, 10*time.Minute); err != nil {
			return fmt.Errorf("failed to download package %s error %w", p.name, err)
		}

	case pkgTypeOci:
		// Download the Image from the OCI-compatible registry, extract the layers from the tarball, and write to the destination directory
		if err := processOciImage(atmosConfig, p.uri, tempDir); err != nil {
			return fmt.Errorf("Failed to process OCI image %s error %w", p.name, err)
		}

	case pkgTypeLocal:
		if err := handlePkgTypeLocalComponent(tempDir, p); err != nil {
			return err
		}
	default:
		return fmt.Errorf("%w %s for package %s", errUtils.ErrUnknownPackageType, p.pkgType.String(), p.name)
	}
	if err := copyComponentToDestination(tempDir, p.componentPath, p.vendorComponentSpec); err != nil {
		return fmt.Errorf("failed to copy package %s error %w", p.name, err)
	}

	return nil
}

func handlePkgTypeLocalComponent(tempDir string, p *pkgComponentVendor) error {
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

	if err := cp.Copy(p.uri, tempDir2, copyOptions); err != nil {
		return fmt.Errorf("failed to copy package %s error %w", p.name, err)
	}
	return nil
}

func installMixin(p *pkgComponentVendor, atmosConfig *schema.AtmosConfiguration) error {
	tempDir, err := os.MkdirTemp("", "atmos-vendor-mixin")
	if err != nil {
		return fmt.Errorf("Failed to create temp directory %w", err)
	}

	defer removeTempDir(tempDir)

	switch p.pkgType {
	case pkgTypeRemote:
		opts := []downloader.GoGetterOption{}
		if p.vendorComponentSpec != nil && p.vendorComponentSpec.Source.Retry != nil {
			opts = append(opts, downloader.WithRetryConfig(p.vendorComponentSpec.Source.Retry))
		}
		if err = downloader.NewGoGetterDownloader(atmosConfig, opts...).Fetch(p.uri, filepath.Join(tempDir, p.mixinFilename), downloader.ClientModeFile, 10*time.Minute); err != nil {
			return fmt.Errorf("failed to download package %s error %w", p.name, err)
		}

	case pkgTypeOci:
		// Download the Image from the OCI-compatible registry, extract the layers from the tarball, and write to the destination directory
		err = processOciImage(atmosConfig, p.uri, tempDir)
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
		return fmt.Errorf("%w %s for package %s", errUtils.ErrUnknownPackageType, p.pkgType.String(), p.name)
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

		// OnDirExists handles existing directories at the destination.
		// If the destination already has a .git directory (from a previous vendor run),
		// we need to leave it untouched to avoid permission errors on git packfiles
		// which often have restrictive permissions.
		OnDirExists: func(src, dest string) cp.DirExistsAction {
			if filepath.Base(dest) == ".git" {
				return cp.Untouchable
			}
			return cp.Merge
		},
	}

	if err := cp.Copy(tempDir, p.componentPath, copyOptions); err != nil {
		return fmt.Errorf("Failed to copy package %s error %w", p.name, err)
	}

	return nil
}
