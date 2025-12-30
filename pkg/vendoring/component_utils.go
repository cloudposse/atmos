package vendoring

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hairyhenderson/gomplate/v3"
	"github.com/jfrog/jfrog-client-go/utils/log"
	cp "github.com/otiai10/copy"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/downloader"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const ociScheme = "oci://"

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
	return "", fmt.Errorf("%w:%s", errUtils.ErrComponentConfigFileNotFound, basePath)
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
		return componentConfig, "", fmt.Errorf("%w:%s", errUtils.ErrFolderNotFound, componentPath)
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
		return componentConfig, "", fmt.Errorf("%w: '%s' in file '%s'", errUtils.ErrInvalidComponentKind, componentConfig.Kind, cfg.ComponentVendorConfigFileName)
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

	return errUtils.ErrStackPullNotSupported
}

func copyComponentToDestination(tempDir, componentPath string, vendorComponentSpec *schema.VendorComponentSpec, sourceIsLocalFile bool, uri string) error {
	// Copy from the temp folder to the destination folder and skip the excluded files
	copyOptions := cp.Options{
		// Skip specifies which files should be skipped
		Skip: createComponentSkipFunc(tempDir, vendorComponentSpec),

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
			componentPath2 = filepath.Join(componentPath, exec.SanitizeFileName(uri))
		}
	}

	if err := cp.Copy(tempDir, componentPath2, copyOptions); err != nil {
		return err
	}
	return nil
}

func createComponentSkipFunc(tempDir string, vendorComponentSpec *schema.VendorComponentSpec) ComponentSkipFunc {
	return func(srcInfo os.FileInfo, src, dest string) (bool, error) {
		if filepath.Base(src) == ".git" {
			return true, nil
		}

		trimmedSrc := u.TrimBasePathFromPath(tempDir+"/", src)

		// Exclude the files that match the 'excluded_paths' patterns
		// It supports POSIX-style Globs for file names/paths (double-star `**` is supported)
		// https://en.wikipedia.org/wiki/Glob_(programming)
		// https://github.com/bmatcuk/doublestar#patterns
		if len(vendorComponentSpec.Source.ExcludedPaths) > 0 {
			return checkComponentExcludes(vendorComponentSpec.Source.ExcludedPaths, src, trimmedSrc)
		}
		// Only include the files that match the 'included_paths' patterns (if any pattern is specified)
		if len(vendorComponentSpec.Source.IncludedPaths) > 0 {
			return checkComponentIncludes(vendorComponentSpec.Source.IncludedPaths, src, trimmedSrc)
		}

		// If 'included_paths' is not provided, include all files that were not excluded
		log.Debug("Including", u.TrimBasePathFromPath(tempDir+"/", src))
		return false, nil
	}
}

func checkComponentExcludes(excludePaths []string, src, trimmedSrc string) (bool, error) {
	for _, excludePath := range excludePaths {
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
	return false, nil
}

func checkComponentIncludes(includePaths []string, src, trimmedSrc string) (bool, error) {
	anyMatches := false
	for _, includePath := range includePaths {
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

func ExecuteComponentVendorInternal(
	atmosConfig *schema.AtmosConfiguration,
	vendorComponentSpec *schema.VendorComponentSpec,
	component string,
	componentPath string,
	dryRun bool,
) error {
	defer perf.Track(atmosConfig, "exec.ExecuteComponentVendorInternal")()

	if vendorComponentSpec.Source.Uri == "" {
		return fmt.Errorf("%w:'%s'", errUtils.ErrURIMustBeSpecified, cfg.ComponentVendorConfigFileName)
	}
	uri := vendorComponentSpec.Source.Uri
	// Parse 'uri' template
	if vendorComponentSpec.Source.Version != "" {
		t, err := template.New(fmt.Sprintf("source-uri-%s", vendorComponentSpec.Source.Version)).Funcs(exec.GetSprigFuncMap()).Funcs(gomplate.CreateFuncs(context.Background(), nil)).Parse(vendorComponentSpec.Source.Uri)
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
			return nil, errUtils.ErrMissingMixinURI
		}

		if mixin.Filename == "" {
			return nil, errUtils.ErrMissingMixinFilename
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

	tmpl, err := template.New("mixin-uri").Funcs(exec.GetSprigFuncMap()).Funcs(gomplate.CreateFuncs(context.Background(), nil)).Parse(mixin.Uri)
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

	defer exec.RemoveTempDir(tempDir)

	switch p.pkgType {
	case pkgTypeRemote:
		tempDir = filepath.Join(tempDir, exec.SanitizeFileName(p.uri))

		opts := []downloader.GoGetterOption{}
		if p.vendorComponentSpec != nil && p.vendorComponentSpec.Source.Retry != nil {
			opts = append(opts, downloader.WithRetryConfig(p.vendorComponentSpec.Source.Retry))
		}
		if err := downloader.NewGoGetterDownloader(atmosConfig, opts...).Fetch(p.uri, tempDir, downloader.ClientModeAny, 10*time.Minute); err != nil {
			return fmt.Errorf("failed to download package %s error %w", p.name, err)
		}

	case pkgTypeOci:
		// Download the Image from the OCI-compatible registry, extract the layers from the tarball, and write to the destination directory
		if err := exec.ProcessOciImage(atmosConfig, p.uri, tempDir); err != nil {
			return fmt.Errorf("Failed to process OCI image %s error %w", p.name, err)
		}

	case pkgTypeLocal:
		if err := handlePkgTypeLocalComponent(tempDir, p); err != nil {
			return err
		}
	default:
		return fmt.Errorf("%w %s for package %s", errUtils.ErrUnknownPackageType, p.pkgType.String(), p.name)
	}
	if err := copyComponentToDestination(tempDir, p.componentPath, p.vendorComponentSpec, p.sourceIsLocalFile, p.uri); err != nil {
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
		tempDir2 = filepath.Join(tempDir, exec.SanitizeFileName(p.uri))
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

	defer exec.RemoveTempDir(tempDir)

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
		err = exec.ProcessOciImage(atmosConfig, p.uri, tempDir)
		if err != nil {
			return fmt.Errorf("failed to process OCI image %s error %w", p.name, err)
		}

	case pkgTypeLocal:
		if p.uri == "" {
			return errUtils.ErrMixinEmpty
		}
		// Implement local mixin installation logic
		return errUtils.ErrMixinNotImplemented

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
	}

	if err := cp.Copy(tempDir, p.componentPath, copyOptions); err != nil {
		return fmt.Errorf("Failed to copy package %s error %w", p.name, err)
	}

	return nil
}
