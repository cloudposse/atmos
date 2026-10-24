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

func TestPkgTypeString(t *testing.T) {
	assert.Equal(t, "remote", PkgTypeRemote.String())
	assert.Equal(t, "oci", PkgTypeOci.String())
	assert.Equal(t, "local", PkgTypeLocal.String())
	assert.Equal(t, "unknown", PkgType(999).String())
}

// TestLockDeclaredSource proves lockDeclaredSource restores the "oci://" scheme the legacy
// installer strips before provenance resolution (so the lock records the original declared
// source), while leaving every other PkgType's uri untouched.
func TestLockDeclaredSource(t *testing.T) {
	tests := []struct {
		name string
		kind PkgType
		uri  string
		want string
	}{
		{
			name: "oci kind without the scheme gets it restored",
			kind: PkgTypeOci,
			uri:  "ghcr.io/cloudposse/vpc:1.0.0",
			want: "oci://ghcr.io/cloudposse/vpc:1.0.0",
		},
		{
			name: "oci kind that already carries the scheme is left alone",
			kind: PkgTypeOci,
			uri:  "oci://ghcr.io/cloudposse/vpc:1.0.0",
			want: "oci://ghcr.io/cloudposse/vpc:1.0.0",
		},
		{
			name: "remote kind is never prefixed",
			kind: PkgTypeRemote,
			uri:  "github.com/cloudposse/terraform-null-label.git",
			want: "github.com/cloudposse/terraform-null-label.git",
		},
		{
			name: "local kind is never prefixed",
			kind: PkgTypeLocal,
			uri:  "/abs/path/to/component",
			want: "/abs/path/to/component",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, lockDeclaredSource(tt.kind, tt.uri))
		})
	}
}

// TestAtmosVendorInstaller_Install_RecordsLockAndSkipsWhenMaterialized proves an atmos-vendor
// package's Install call fetches a local source, copies it to its target, and records a
// vendor.lock.yaml receipt; a second Install for a differently-named package sharing the same
// source records an independent receipt; and FilterPending skips a package whose target still
// matches its receipt while surfacing one whose target was modified out-of-band.
func TestAtmosVendorInstaller_Install_RecordsLockAndSkipsWhenMaterialized(t *testing.T) {
	base := t.TempDir()
	sourceDir := filepath.Join(base, "source")
	require.NoError(t, os.MkdirAll(sourceDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "main.tf"), []byte("# vpc\n"), 0o644))

	target := filepath.Join(base, "target")
	atmosConfig := &schema.AtmosConfiguration{BasePath: base}

	pkg := NewAtmosVendorPackage(&AtmosPackageParams{
		Name:       "vpc",
		URI:        sourceDir,
		TargetPath: target,
		PkgType:    PkgTypeLocal,
	})
	secondary := NewAtmosVendorPackage(&AtmosPackageParams{
		Name:       "vpc-secondary",
		URI:        sourceDir,
		TargetPath: target,
		PkgType:    PkgTypeLocal,
	})

	result, err := Install(atmosConfig, pkg, InstallOptions{})
	require.NoError(t, err)
	require.NoError(t, result.Err)
	assert.FileExists(t, filepath.Join(target, "main.tf"))

	result, err = Install(atmosConfig, secondary, InstallOptions{})
	require.NoError(t, err)
	require.NoError(t, result.Err)

	lock, err := lockfile.Load(atmosConfig)
	require.NoError(t, err)
	require.Len(t, lock.Artifacts, 2)
	for _, artifact := range lock.Artifacts {
		require.Equal(t, "sha256", artifact.Source.Digest[:6])
	}

	pending, err := FilterPending(atmosConfig, []VendorPackage{pkg, secondary}, InstallOptions{})
	require.NoError(t, err)
	assert.Empty(t, pending, "both packages must be reported materialized immediately after Install")

	require.NoError(t, os.WriteFile(filepath.Join(target, "main.tf"), []byte("modified"), 0o644))

	pending, err = FilterPending(atmosConfig, []VendorPackage{pkg, secondary}, InstallOptions{})
	require.NoError(t, err)
	require.Len(t, pending, 2, "a modified target file invalidates every artifact that owns it")
}

// TestAtmosVendorInstaller_IsMaterialized_DetectsIncludedExcludedPathsDrift proves a vendor.yaml
// source's isMaterialized detects a change to its own included_paths/excluded_paths as drift, even
// when the declared source URI itself is unchanged -- closing the gap where changing only a
// source's copy-filter patterns was previously invisible to materialization.
func TestAtmosVendorInstaller_IsMaterialized_DetectsIncludedExcludedPathsDrift(t *testing.T) {
	base := t.TempDir()
	sourceDir := filepath.Join(base, "source")
	require.NoError(t, os.MkdirAll(sourceDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "main.tf"), []byte("# vpc\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "README.md"), []byte("# readme\n"), 0o644))

	target := filepath.Join(base, "target")
	atmosConfig := &schema.AtmosConfiguration{BasePath: base}

	pkg := NewAtmosVendorPackage(&AtmosPackageParams{
		Name:       "vpc",
		URI:        sourceDir,
		TargetPath: target,
		PkgType:    PkgTypeLocal,
		Source:     schema.AtmosVendorSource{IncludedPaths: []string{"*.tf"}},
	})

	result, err := Install(atmosConfig, pkg, InstallOptions{})
	require.NoError(t, err)
	require.NoError(t, result.Err)

	installer, ok := pkg.installer.(*atmosVendorInstaller)
	require.True(t, ok)

	check, err := installer.isMaterialized(atmosConfig)
	require.NoError(t, err)
	assert.True(t, check.Materialized)

	// Widening included_paths in vendor.yaml, with no other change, is drift.
	installer.source.IncludedPaths = []string{"*.tf", "*.md"}
	check, err = installer.isMaterialized(atmosConfig)
	require.NoError(t, err)
	assert.False(t, check.Materialized)
	assert.Equal(t, "included/excluded paths changed", check.Reason)
}

// TestAtmosVendorInstaller_Install_RecordLockErrorSurfaced proves a vendor-lock recording failure
// after a successful copy is surfaced as an install.Result error (naming the package), instead of
// being reported as an overall success despite the receipt never having been written.
func TestAtmosVendorInstaller_Install_RecordLockErrorSurfaced(t *testing.T) {
	sourceDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "main.tf"), []byte("# vpc\n"), 0o644))

	// targetPath deliberately lives outside atmosConfig.BasePath's tree, so lockfile.Replace
	// can't relate it back to the project root and returns an error, even though the preceding
	// copy to targetPath itself succeeds.
	atmosConfig := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	targetPath := t.TempDir()

	pkg := NewAtmosVendorPackage(&AtmosPackageParams{
		Name:       "vpc",
		URI:        sourceDir,
		TargetPath: targetPath,
		PkgType:    PkgTypeLocal,
	})

	result, err := Install(atmosConfig, pkg, InstallOptions{})

	require.NoError(t, err)
	require.Error(t, result.Err)
	assert.Contains(t, result.Err.Error(), "vpc")
	assert.Contains(t, result.Err.Error(), "failed to record vendor lock")
}
