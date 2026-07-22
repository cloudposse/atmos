package install

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	cp "github.com/otiai10/copy"

	"github.com/cloudposse/atmos/pkg/downloader"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/vendor"
	"github.com/cloudposse/atmos/pkg/vendoring/lockfile"
)

// componentVendorInstaller installs a component.yaml's component or one of its mixins: fetch (via
// fetchToTempDir), copy to the component directory (vendor.CopyToTarget for a component, or a
// whole-tempDir copy for a mixin -- see install below), then record a vendor.lock.yaml receipt.
type componentVendorInstaller struct {
	srcURI        string
	name          string
	componentPath string
	localFile     bool
	pType         PkgType
	version       string
	rawVersion    string
	spec          *schema.VendorComponentSpec
	mixin         bool
	mixinFile     string
}

// ComponentPackageParams are the fields NewComponentVendorPackage needs to build a component.yaml
// component or mixin's VendorPackage. A struct rather than positional parameters: Options Pattern
// (CLAUDE.md) applies once a constructor has more than four parameters.
type ComponentPackageParams struct {
	Name              string
	URI               string
	ComponentPath     string
	Version           string
	RawVersion        string
	PkgType           PkgType
	SourceIsLocalFile bool
	Spec              *schema.VendorComponentSpec
	IsMixin           bool
	MixinFilename     string
}

// NewComponentVendorPackage builds a VendorPackage for a component.yaml's component (IsMixin
// false) or one of its mixins (IsMixin true, MixinFilename set).
func NewComponentVendorPackage(params *ComponentPackageParams) VendorPackage {
	defer perf.Track(nil, "install.NewComponentVendorPackage")()

	return VendorPackage{
		Name:       params.Name,
		Version:    params.Version,
		RawVersion: params.RawVersion,
		installer: &componentVendorInstaller{
			srcURI:        params.URI,
			name:          params.Name,
			componentPath: params.ComponentPath,
			localFile:     params.SourceIsLocalFile,
			pType:         params.PkgType,
			version:       params.Version,
			rawVersion:    params.RawVersion,
			spec:          params.Spec,
			mixin:         params.IsMixin,
			mixinFile:     params.MixinFilename,
		},
	}
}

func (p *componentVendorInstaller) install(ctx context.Context, tempDir string, atmosConfig *schema.AtmosConfiguration) error {
	if p.mixin {
		return p.installMixin(ctx, tempDir, atmosConfig)
	}
	return p.installComponent(ctx, tempDir, atmosConfig)
}

func (p *componentVendorInstaller) installComponent(ctx context.Context, tempDir string, atmosConfig *schema.AtmosConfiguration) error {
	fetchedDir, metadata, err := fetchToTempDir(ctx, atmosConfig, p.srcURI, p.pType, tempDir, fetchOptions{
		ClientMode: downloader.ClientModeAny,
		// component.yaml's remote sources (unlike vendor.yaml's) join tempDir with the
		// sanitized source filename before fetching -- installComponent's pre-unification
		// behavior, preserved here exactly.
		JoinSanitizedFilename: true,
		SourceIsLocalFile:     p.localFile,
		Retry:                 p.retry(),
	})
	if err != nil {
		return err
	}

	if err := vendor.CopyToTarget(fetchedDir, p.componentPath, vendor.CopyOptions{
		IncludedPaths: p.includedPaths(),
		ExcludedPaths: p.excludedPaths(),
	}); err != nil {
		return fmt.Errorf("failed to copy package %s error %w", p.name, err)
	}

	recordOpts := lockfile.RecordOptions{
		IncludedPaths: p.includedPaths(),
		ExcludedPaths: p.excludedPaths(),
		HTTPMetadata:  metadata,
	}
	if p.rawVersion != "" {
		recordOpts.VersionConstraint = p.rawVersion
		recordOpts.ResolvedVersion = p.version
	}
	if err := lockfile.Record(ctx, atmosConfig, p.pType.String(), p.name, fetchedDir, p.componentPath, lockDeclaredSource(p.pType, p.srcURI), recordOpts); err != nil {
		return fmt.Errorf("record component vendor lock: %w", err)
	}
	return nil
}

func (p *componentVendorInstaller) installMixin(ctx context.Context, tempDir string, atmosConfig *schema.AtmosConfiguration) error {
	if p.pType == PkgTypeLocal && p.srcURI == "" {
		return ErrMixinEmpty
	}

	// A mixin's remote or local fetch writes to tempDir/<mixinFile> (ClientModeFile for remote,
	// Target for both); OCI writes directly into tempDir. Either way the whole of tempDir -- not
	// just fetchToTempDir's returned content root -- is what gets copied to the destination below,
	// matching the pre-unification installMixin, which always copied from its own top-level tempDir.
	_, metadata, err := fetchToTempDir(ctx, atmosConfig, p.srcURI, p.pType, tempDir, fetchOptions{
		ClientMode: downloader.ClientModeFile,
		Target:     filepath.Join(tempDir, p.mixinFile),
		Retry:      p.retry(),
	})
	if err != nil {
		return err
	}

	copyOptions := cp.Options{
		PreserveTimes: false,
		PreserveOwner: false,
		OnSymlink:     func(string) cp.SymlinkAction { return cp.Deep },
		// If the destination already has a .git directory (from a previous vendor run), leave
		// it untouched to avoid permission errors on git packfiles, which often have
		// restrictive permissions.
		OnDirExists: func(src, dest string) cp.DirExistsAction {
			if filepath.Base(dest) == ".git" {
				return cp.Untouchable
			}
			return cp.Merge
		},
	}
	if err := cp.Copy(tempDir, p.componentPath, copyOptions); err != nil {
		return fmt.Errorf("failed to copy package %s error %w", p.name, err)
	}

	if err := lockfile.Record(ctx, atmosConfig, p.pType.String(), p.name, tempDir, p.componentPath, lockDeclaredSource(p.pType, p.srcURI), lockfile.RecordOptions{
		Mixin:         true,
		MixinFilename: p.mixinFile,
		HTTPMetadata:  metadata,
	}); err != nil {
		return fmt.Errorf("record mixin vendor lock: %w", err)
	}
	return nil
}

func (p *componentVendorInstaller) dryRunCheck(_ context.Context, atmosConfig *schema.AtmosConfiguration) error {
	log.Debug("Dry-run mode: custom detection required for component (or mixin) URI", "component", p.name, "uri", p.srcURI)
	if err := detectIfNeeded(atmosConfig, p.srcURI); err != nil {
		return fmt.Errorf("dry-run: detection failed for component %s: %w", p.name, err)
	}
	time.Sleep(100 * time.Millisecond)
	return nil
}

func (p *componentVendorInstaller) isMixin() bool { return p.mixin }

// artifactID matches lockfile.Record's own writer-list computation (name alone for a component,
// name+mixin filename for a mixin), so the materialization check (isMaterialized, below) and the
// lock write (install, via lockfile.Record) always agree on the same key.
func (p *componentVendorInstaller) artifactID(atmosConfig *schema.AtmosConfiguration) (string, error) {
	writers := []string{p.name}
	if p.mixin {
		writers = append(writers, p.mixinFile)
	}
	return lockfile.ArtifactID(atmosConfig, p.pType.String(), p.componentPath, writers...)
}

// isMaterialized compares the component's own copy-filter patterns against the receipt only for a
// genuine component install. A mixin is always inventoried unfiltered (see installMixin's
// VendorInventory call and its RecordOptions, which never set IncludedPaths/ExcludedPaths), even
// though it shares the parent component.yaml's *Spec -- so p.includedPaths()/excludedPaths() would
// otherwise report every mixin as spuriously drifted whenever the component itself declares
// include/exclude patterns.
func (p *componentVendorInstaller) isMaterialized(atmosConfig *schema.AtmosConfiguration) (lockfile.MaterializationCheck, error) {
	id, err := p.artifactID(atmosConfig)
	if err != nil {
		return lockfile.MaterializationCheck{}, err
	}
	params := lockfile.MaterializationParams{
		ID:       id,
		Declared: lockDeclaredSource(p.pType, p.srcURI),
		Target:   p.componentPath,
	}
	if !p.mixin {
		params.IncludedPaths = p.includedPaths()
		params.ExcludedPaths = p.excludedPaths()
	}
	return lockfile.IsMaterialized(atmosConfig, params)
}

func (p *componentVendorInstaller) pkgType() PkgType        { return p.pType }
func (p *componentVendorInstaller) uri() string             { return p.srcURI }
func (p *componentVendorInstaller) target() string          { return p.componentPath }
func (p *componentVendorInstaller) mixinFilename() string   { return p.mixinFile }
func (p *componentVendorInstaller) sourceIsLocalFile() bool { return p.localFile }

func (p *componentVendorInstaller) retry() *schema.RetryConfig {
	if p.spec == nil {
		return nil
	}
	return p.spec.Source.Retry
}

func (p *componentVendorInstaller) includedPaths() []string {
	if p.spec == nil {
		return nil
	}
	return p.spec.Source.IncludedPaths
}

func (p *componentVendorInstaller) excludedPaths() []string {
	if p.spec == nil {
		return nil
	}
	return p.spec.Source.ExcludedPaths
}
