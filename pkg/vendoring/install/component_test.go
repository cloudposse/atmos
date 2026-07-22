package install

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/vendoring/lockfile"
)

// setupMixinInstallFixture writes content to a temp source file named sourceFileName, and creates
// a fresh component directory under a new temp base path, for a component.yaml mixin install test.
// Shared by TestComponentVendorInstaller_InstallMixin_LocalFileSource and
// TestComponentVendorInstaller_InstallMixin_LocalPkgType_Succeeds, whose only material difference
// is the mixin's pkgType.
func setupMixinInstallFixture(t *testing.T, sourceFileName, content string) (atmosConfig *schema.AtmosConfiguration, componentPath, sourceFile string) {
	t.Helper()

	sourceFile = filepath.Join(t.TempDir(), sourceFileName)
	require.NoError(t, os.WriteFile(sourceFile, []byte(content), 0o644))

	basePath := t.TempDir()
	componentPath = filepath.Join(basePath, "components", "terraform", "vpc")
	require.NoError(t, os.MkdirAll(componentPath, 0o755))

	atmosConfig = &schema.AtmosConfiguration{BasePath: basePath}
	return atmosConfig, componentPath, sourceFile
}

// TestComponentVendorInstaller_InstallMixin_LocalFileSource proves a mixin whose pkgType is
// PkgTypeRemote (resolveMixinPackage, internal/exec/vendor_component_packages.go, never
// reclassifies a local file the way component sources do via handleLocalFileScheme) is
// materialized by fetching a plain local file path through go-getter's built-in local-file
// support - no network access involved - copying it to the component directory, and recording a
// vendor lock entry for it.
func TestComponentVendorInstaller_InstallMixin_LocalFileSource(t *testing.T) {
	atmosConfig, componentPath, sourceFile := setupMixinInstallFixture(t, "context.tf", "# mixin\n")
	pkg := NewComponentVendorPackage(&ComponentPackageParams{
		Name:          "mixin " + sourceFile,
		URI:           sourceFile,
		ComponentPath: componentPath,
		PkgType:       PkgTypeRemote,
		IsMixin:       true,
		MixinFilename: "context.tf",
	})

	result, err := Install(atmosConfig, pkg, InstallOptions{})

	require.NoError(t, err)
	require.NoError(t, result.Err)
	data, readErr := os.ReadFile(filepath.Join(componentPath, "context.tf"))
	require.NoError(t, readErr)
	assert.Equal(t, "# mixin\n", string(data))

	lock, err := lockfile.Load(atmosConfig)
	require.NoError(t, err)
	assert.Len(t, lock.Artifacts, 1, "Install must record a vendor lock entry for the mixin")
}

// TestComponentVendorInstaller_InstallMixin_LocalPkgType_MissingUri proves the PkgTypeLocal
// branch's guard against an empty uri returns a descriptive sentinel error instead of proceeding
// into an unimplemented code path.
func TestComponentVendorInstaller_InstallMixin_LocalPkgType_MissingUri(t *testing.T) {
	pkg := NewComponentVendorPackage(&ComponentPackageParams{Name: "mixin", PkgType: PkgTypeLocal, IsMixin: true})

	result, err := Install(&schema.AtmosConfiguration{}, pkg, InstallOptions{})

	require.NoError(t, err)
	require.Error(t, result.Err)
	assert.ErrorIs(t, result.Err, ErrMixinEmpty)
}

// TestComponentVendorInstaller_InstallMixin_LocalPkgType_Succeeds proves a mixin explicitly
// classified as PkgTypeLocal (unreachable from resolveMixinPackage today, which always produces
// PkgTypeRemote/PkgTypeOci and lets go-getter's own local-file support handle a bare local path -
// see TestComponentVendorInstaller_InstallMixin_LocalFileSource above - but a real, valid
// classification for any other caller of this package's API) installs correctly instead of hitting
// the removed "not implemented" stub: fetchToTempDir's PkgTypeLocal branch copies the source
// straight into tempDir/<mixinFilename>, renaming it along the way, then the mixin is copied to
// the component directory and recorded in the vendor lock exactly like a remote-classified mixin.
func TestComponentVendorInstaller_InstallMixin_LocalPkgType_Succeeds(t *testing.T) {
	atmosConfig, componentPath, sourceFile := setupMixinInstallFixture(t, "shared-context.tf", "# local mixin\n")
	pkg := NewComponentVendorPackage(&ComponentPackageParams{
		Name:          "mixin " + sourceFile,
		URI:           sourceFile,
		ComponentPath: componentPath,
		PkgType:       PkgTypeLocal,
		IsMixin:       true,
		// Deliberately different from the source file's own name, proving the mixin is renamed
		// to its declared filename during install, not just copied verbatim.
		MixinFilename: "context.tf",
	})

	result, err := Install(atmosConfig, pkg, InstallOptions{})

	require.NoError(t, err)
	require.NoError(t, result.Err)
	data, readErr := os.ReadFile(filepath.Join(componentPath, "context.tf"))
	require.NoError(t, readErr)
	assert.Equal(t, "# local mixin\n", string(data))

	lock, err := lockfile.Load(atmosConfig)
	require.NoError(t, err)
	assert.Len(t, lock.Artifacts, 1, "Install must record a vendor lock entry for the local mixin")
}

// TestComponentVendorInstaller_IsMaterialized_MixinIgnoresComponentPatterns proves a mixin's
// isMaterialized never compares against the parent component.yaml's own IncludedPaths/ExcludedPaths
// -- which a mixin install never applies (installMixin always inventories unfiltered, see
// VendorInventory in installMixin) -- even though NewComponentVendorPackage gives every mixin the
// same *VendorComponentSpec as its parent component. Without this guard, isMaterialized would
// spuriously report drift for every mixin whenever the component itself declares include/exclude
// patterns for its own (unrelated) copy.
func TestComponentVendorInstaller_IsMaterialized_MixinIgnoresComponentPatterns(t *testing.T) {
	atmosConfig, componentPath, sourceFile := setupMixinInstallFixture(t, "context.tf", "# mixin\n")
	spec := &schema.VendorComponentSpec{Source: schema.VendorComponentSource{IncludedPaths: []string{"*.tf"}}}

	pkg := NewComponentVendorPackage(&ComponentPackageParams{
		Name:          "mixin " + sourceFile,
		URI:           sourceFile,
		ComponentPath: componentPath,
		PkgType:       PkgTypeLocal,
		Spec:          spec,
		IsMixin:       true,
		MixinFilename: "context.tf",
	})

	result, err := Install(atmosConfig, pkg, InstallOptions{})
	require.NoError(t, err)
	require.NoError(t, result.Err)

	installer, ok := pkg.installer.(*componentVendorInstaller)
	require.True(t, ok)

	check, err := installer.isMaterialized(atmosConfig)
	require.NoError(t, err)
	assert.True(t, check.Materialized, "a mixin sharing its parent component's IncludedPaths must not be compared against them; got reason %q", check.Reason)
}

// TestComponentVendorInstaller_InstallMixin_RecordVendorLockErrorSurfaced proves a vendor-lock
// recording failure after a successful copy is surfaced as an install.Result error naming the
// mixin (rather than reporting success despite no receipt ever having been written).
func TestComponentVendorInstaller_InstallMixin_RecordVendorLockErrorSurfaced(t *testing.T) {
	sourceFile := filepath.Join(t.TempDir(), "context.tf")
	require.NoError(t, os.WriteFile(sourceFile, []byte("# mixin\n"), 0o644))

	// componentPath deliberately lives outside atmosConfig.BasePath's tree, so lockfile.Record's
	// lockfile.Replace can't relate it back to the project root, even though the preceding copy
	// to componentPath itself succeeds.
	atmosConfig := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	componentPath := t.TempDir()

	pkg := NewComponentVendorPackage(&ComponentPackageParams{
		Name:          "mixin " + sourceFile,
		URI:           sourceFile,
		ComponentPath: componentPath,
		PkgType:       PkgTypeRemote,
		IsMixin:       true,
		MixinFilename: "context.tf",
	})

	result, err := Install(atmosConfig, pkg, InstallOptions{})

	require.NoError(t, err)
	require.Error(t, result.Err)
	assert.Contains(t, result.Err.Error(), "record mixin vendor lock")
	// The copy itself must have succeeded before the lock recording failed.
	assert.FileExists(t, filepath.Join(componentPath, "context.tf"))
}

// TestComponentVendorInstaller_InstallComponent_RecordVendorLockErrorSurfaced proves a
// vendor-lock recording failure after a successful component copy is surfaced as an
// install.Result error naming the component (rather than reporting success despite no receipt
// ever having been written).
func TestComponentVendorInstaller_InstallComponent_RecordVendorLockErrorSurfaced(t *testing.T) {
	sourceDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "main.tf"), []byte("# vpc\n"), 0o644))

	// componentPath deliberately lives outside atmosConfig.BasePath's tree, so lockfile.Record's
	// lockfile.Replace can't relate it back to the project root, even though the preceding copy
	// to componentPath itself succeeds.
	atmosConfig := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	componentPath := t.TempDir()

	pkg := NewComponentVendorPackage(&ComponentPackageParams{
		Name:          "vpc",
		URI:           sourceDir,
		ComponentPath: componentPath,
		PkgType:       PkgTypeLocal,
		Spec:          &schema.VendorComponentSpec{Source: schema.VendorComponentSource{Uri: sourceDir}},
	})

	result, err := Install(atmosConfig, pkg, InstallOptions{})

	require.NoError(t, err)
	require.Error(t, result.Err)
	assert.Contains(t, result.Err.Error(), "record component vendor lock")
	// The copy itself must have succeeded before the lock recording failed.
	assert.FileExists(t, filepath.Join(componentPath, "main.tf"))
}
