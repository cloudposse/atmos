package exec

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloudposse/atmos/pkg/perf"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jfrog/jfrog-client-go/utils/log"
	cp "github.com/otiai10/copy"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/downloader"
	"github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/cloudposse/atmos/pkg/vendor"
	"github.com/cloudposse/atmos/pkg/vendoring"
	"github.com/cloudposse/atmos/pkg/vendoring/lockfile"
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

	// Resolve to absolute so the vendor lock's target-containment check (pkg/vendoring/lockfile)
	// compares two absolute paths; a relative atmosConfig.BasePath otherwise makes componentPath
	// relative to CWD, which the lock validation cannot reliably relate back to BasePath.
	componentPath, err := filepath.Abs(componentPath)
	if err != nil {
		return componentConfig, "", fmt.Errorf("resolve component path: %w", err)
	}

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

func ExecuteComponentVendorInternal(
	atmosConfig *schema.AtmosConfiguration,
	vendorComponentSpec *schema.VendorComponentSpec,
	component string,
	componentPath string,
	dryRun bool,
	refreshLock bool,
) error {
	defer perf.Track(atmosConfig, "exec.ExecuteComponentVendorInternal")()

	packages, err := buildComponentVendorPackages(atmosConfig, vendorComponentSpec, component, componentPath)
	if err != nil {
		return err
	}
	if !dryRun && !refreshLock {
		packages, err = filterMaterializedComponentVendorPackages(packages, atmosConfig)
		if err != nil {
			return err
		}
	}
	if len(packages) > 0 {
		return executeVendorModel(packages, dryRun, atmosConfig)
	}
	return nil
}

// ExecuteComponentVendorPullBatch resolves and pulls multiple components declared via their own
// component.yaml manifests in a single batched run (one progress bar, one completion summary),
// instead of one executeVendorModel call per component. Used by `atmos vendor update --pull`
// to avoid a separate "0/1" progress block per updated component.
//
// Resolution errors are propagated immediately (fail-fast): silently skipping a component whose
// component.yaml fails to parse would silently under-pull, matching the existing single-component
// behavior in handleComponentVendor (internal/exec/vendor.go), which also fails fast.
func ExecuteComponentVendorPullBatch(
	atmosConfig *schema.AtmosConfiguration,
	components []string,
	componentType string,
	dryRun bool,
	refreshLock bool,
) error {
	defer perf.Track(atmosConfig, "exec.ExecuteComponentVendorPullBatch")()

	if len(components) == 0 {
		return nil
	}

	var allPackages []pkgComponentVendor
	for _, component := range components {
		config, componentPath, err := ReadAndProcessComponentVendorConfigFile(atmosConfig, component, componentType)
		if err != nil {
			return fmt.Errorf("component %q: %w", component, err)
		}
		packages, err := buildComponentVendorPackages(atmosConfig, &config.Spec, component, componentPath)
		if err != nil {
			return fmt.Errorf("component %q: %w", component, err)
		}
		if !dryRun && !refreshLock {
			packages, err = filterMaterializedComponentVendorPackages(packages, atmosConfig)
			if err != nil {
				return fmt.Errorf("component %q: verify vendor lock: %w", component, err)
			}
		}
		allPackages = append(allPackages, packages...)
	}

	if len(allPackages) == 0 {
		return nil
	}
	return executeVendorModel(allPackages, dryRun, atmosConfig)
}

// handleVendorPullSweep implements "atmos vendor pull --everything" (and bare "atmos vendor pull",
// which defaults --everything to true — see setDefaultEverythingFlag) for a repo with no vendor.yaml:
// it discovers every component.yaml/component.yml manifest under the configured component-type
// base path(s) — all of terraform/helmfile/packer by default, or just flg.ComponentType when the
// user passed --type explicitly (flg.TypeChanged) — groups the discovered component names by their
// own ComponentType (a repo-wide sweep with no explicit --type can mix terraform/helmfile/packer in
// one run, and ExecuteComponentVendorPullBatch only accepts one componentType per call), and pulls
// each type-group in its own batched call. Mirrors, for "vendor pull", what
// cmd/vendor/update.go's runRepoWideUpdate/runVendorPull already do for "vendor update --pull" in
// the identical repo shape.
func handleVendorPullSweep(atmosConfig *schema.AtmosConfiguration, flg *VendorFlags) error {
	defer perf.Track(atmosConfig, "exec.handleVendorPullSweep")()

	found, err := vendoring.DiscoverAllComponentManifests(flg.ComponentType, flg.TypeChanged)
	if err != nil {
		return err
	}
	if len(found) == 0 {
		return ErrNoVendorSourcesFound
	}

	componentsByType := map[string][]string{}
	for _, rs := range found {
		if rs == nil || rs.Source == nil {
			continue
		}
		componentsByType[rs.ComponentType] = append(componentsByType[rs.ComponentType], rs.Source.Component)
	}

	var errs []error
	for componentType, components := range componentsByType {
		if err := ExecuteComponentVendorPullBatch(atmosConfig, components, componentType, flg.DryRun, flg.RefreshLock); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func filterMaterializedComponentVendorPackages(packages []pkgComponentVendor, atmosConfig *schema.AtmosConfiguration) ([]pkgComponentVendor, error) {
	pending := make([]pkgComponentVendor, 0, len(packages))
	for _, pkg := range packages {
		materialized, err := lockfile.IsMaterialized(
			atmosConfig,
			lockfile.ArtifactID(pkg.pkgType.String(), pkg.componentPath, pkg.name, pkg.mixinFilename),
			lockDeclaredSource(pkg.pkgType, pkg.uri),
			pkg.componentPath,
		)
		if err != nil {
			return nil, err
		}
		if !materialized {
			pending = append(pending, pkg)
		}
	}
	return pending, nil
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
		// Download the Image from the OCI-compatible registry, extract the layers from the tarball, and write to the destination directory.
		// Bounded to the same 10-minute timeout as the go-getter path above.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		if err := oci.ProcessImage(ctx, atmosConfig, p.uri, tempDir); err != nil {
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
	if err := recordComponentVendorLock(p, tempDir, atmosConfig, true); err != nil {
		return fmt.Errorf("record component vendor lock: %w", err)
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
		// Download the Image from the OCI-compatible registry, extract the layers from the tarball, and write to the destination directory.
		// Bounded to the same 10-minute timeout as the go-getter path above.
		ociCtx, ociCancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer ociCancel()
		err = oci.ProcessImage(ociCtx, atmosConfig, p.uri, tempDir)
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
	if err := recordComponentVendorLock(p, tempDir, atmosConfig, false); err != nil {
		return fmt.Errorf("record mixin vendor lock: %w", err)
	}

	return nil
}

// recordComponentVendorLock records component.yaml sources and mixins as
// independent writers. The staging directory gives us the exact copy plan,
// including include/exclude patterns, without claiming locally authored files.
func recordComponentVendorLock(p *pkgComponentVendor, tempDir string, atmosConfig *schema.AtmosConfiguration, applyPatterns bool) error {
	var (
		files []lockfile.File
		err   error
	)
	if applyPatterns && p.vendorComponentSpec != nil {
		files, err = lockfile.VendorInventoryWithPatterns(tempDir, p.vendorComponentSpec.Source.IncludedPaths, p.vendorComponentSpec.Source.ExcludedPaths)
	} else {
		files, err = lockfile.VendorInventory(tempDir)
	}
	if err != nil {
		return err
	}
	declaredSource := lockDeclaredSource(p.pkgType, p.uri)
	resolved, err := downloader.ResolveArtifact(context.Background(), atmosConfig, declaredSource, tempDir)
	if err != nil {
		return err
	}
	identity := resolved.Identity
	if identity == "" {
		identity = lockfile.FilesDigest(files)
	}
	artifact := lockfile.Artifact{
		Component: p.name,
		Kind:      p.pkgType.String(),
		Target:    p.componentPath,
		Source:    lockfile.Source{Declared: declaredSource, Resolved: resolved.Resolved, Digest: identity},
		Files:     files,
	}
	return lockfile.Replace(atmosConfig, lockfile.ArtifactID(artifact.Kind, artifact.Target, p.name, p.mixinFilename), artifact)
}
