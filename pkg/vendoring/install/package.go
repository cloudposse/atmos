// Package install implements the single, unified install path for `atmos vendor pull`/`update`:
// fetching a declared vendor.yaml source or component.yaml component/mixin into a scratch
// directory, copying it to its target, and recording a vendor.lock.yaml receipt. It has no
// Bubble Tea dependency, so Install and FilterPending are directly unit-testable and reusable by
// any non-interactive call path; internal/exec/vendor_model.go's TUI model calls Install from a
// thin tea.Cmd closure.
package install

import (
	"context"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/vendoring/lockfile"
)

// PkgType classifies how a package's URI must be fetched.
type PkgType int

const (
	// PkgTypeRemote is fetched via go-getter (git, http(s), s3, gcs, ...).
	PkgTypeRemote PkgType = iota
	// PkgTypeOci is fetched from an OCI-compatible registry.
	PkgTypeOci
	// PkgTypeLocal is copied from the local filesystem.
	PkgTypeLocal
)

// String renders a PkgType for logging and as the lock file's "kind" field.
func (p PkgType) String() string {
	defer perf.Track(nil, "install.PkgType.String")()

	names := [...]string{"remote", "oci", "local"}
	if p < PkgTypeRemote || p > PkgTypeLocal {
		return "unknown"
	}
	return names[p-PkgTypeRemote]
}

// pkgInstaller is implemented by atmosVendorInstaller and componentVendorInstaller, the two
// concrete backends behind VendorPackage. Every pkgType-specific and vendor.yaml-vs-component.yaml
// branch stays inside these two implementations; callers of VendorPackage/Install/FilterPending
// never switch on which one they hold.
type pkgInstaller interface {
	// install fetches the package into tempDir (a caller-owned, already-created scratch
	// directory removed after this call returns), copies it to its declared target, and
	// records a vendor.lock.yaml receipt for it.
	install(ctx context.Context, tempDir string, atmosConfig *schema.AtmosConfiguration) error
	// dryRunCheck performs a --dry-run's only side effect: the same custom-detector probe a real
	// fetch would trigger for go-getter-unsupported URI schemes, without writing anything.
	dryRunCheck(ctx context.Context, atmosConfig *schema.AtmosConfiguration) error
	// isMixin reports whether this package is a component.yaml mixin rather than a genuine
	// top-level component or vendor.yaml source/target.
	isMixin() bool
	// isMaterialized reports whether an existing vendor.lock.yaml receipt already proves this
	// package's declared source, copy-filter patterns, and every destination file are unchanged, so
	// Install can be skipped -- and, when not, why not (see lockfile.MaterializationCheck).
	isMaterialized(atmosConfig *schema.AtmosConfiguration) (lockfile.MaterializationCheck, error)
	// pkgType, uri, target, mixinFilename, and sourceIsLocalFile expose the fields tests and
	// FilterPending need without leaking either concrete installer type to callers.
	pkgType() PkgType
	uri() string
	target() string
	mixinFilename() string
	sourceIsLocalFile() bool
}

// VendorPackage is the single installable unit for `atmos vendor pull`/`update`: either one
// target of a vendor.yaml source, or a component.yaml's component or one of its mixins. Build one
// via NewAtmosVendorPackage or NewComponentVendorPackage; callers never need to know which
// concrete installer backs it.
type VendorPackage struct {
	// Name identifies the package for progress/status reporting: a vendor.yaml source's
	// component name (or its URI when no component name is declared), a component.yaml's
	// component name, or "mixin <uri>" for a mixin.
	Name string
	// Version is the declared version/ref, shown alongside Name in status output. Empty when
	// the source has no explicit version. For a range-declared `version:`, this is the resolved
	// concrete version (what's actually fetched), not the raw range -- see RawVersion.
	Version string
	// RawVersion is the source's originally-declared version string, populated only when it
	// differs from the resolved Version above -- i.e. only for a range-declared `version:` (see
	// pkg/vendoring/install/version_resolve.go). Empty for an exact-pinned version:, matching
	// lockfile.Source.VersionConstraint's own omitempty convention.
	RawVersion string

	installer pkgInstaller
}

// IsMixin reports whether pkg is a component.yaml mixin.
func (pkg VendorPackage) IsMixin() bool {
	defer perf.Track(nil, "install.VendorPackage.IsMixin")()

	return pkg.installer != nil && pkg.installer.isMixin()
}

// PkgType reports how pkg's URI must be fetched.
func (pkg VendorPackage) PkgType() PkgType {
	defer perf.Track(nil, "install.VendorPackage.PkgType")()

	return pkg.installer.pkgType()
}

// URI returns pkg's declared source URI (scheme-stripped for OCI; see lockDeclaredSource).
func (pkg VendorPackage) URI() string {
	defer perf.Track(nil, "install.VendorPackage.URI")()

	return pkg.installer.uri()
}

// Target returns pkg's destination directory: a vendor.yaml target's path, or a component.yaml
// component/mixin's component directory.
func (pkg VendorPackage) Target() string {
	defer perf.Track(nil, "install.VendorPackage.Target")()

	return pkg.installer.target()
}

// MixinFilename returns the mixin's declared output filename, or "" for a non-mixin package.
func (pkg VendorPackage) MixinFilename() string {
	defer perf.Track(nil, "install.VendorPackage.MixinFilename")()

	return pkg.installer.mixinFilename()
}

// SourceIsLocalFile reports whether pkg's URI names a single local file (as opposed to a local
// directory, or a remote/OCI source) -- see fetchOptions.SourceIsLocalFile's doc comment for how
// this changes fetch/copy behavior.
func (pkg VendorPackage) SourceIsLocalFile() bool {
	defer perf.Track(nil, "install.VendorPackage.SourceIsLocalFile")()

	return pkg.installer.sourceIsLocalFile()
}
