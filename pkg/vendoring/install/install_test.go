package install

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// newMaterializedAtmosPackage installs a fresh local-source vendor.yaml package once (recording a
// vendor.lock.yaml receipt), returning the now-materialized VendorPackage plus the atmosConfig and
// target directory it was installed to. Shared by FilterPending's enforcement-level tests below.
func newMaterializedAtmosPackage(t *testing.T, name string) (atmosConfig *schema.AtmosConfiguration, pkg VendorPackage, target string) {
	t.Helper()

	base := t.TempDir()
	sourceDir := filepath.Join(base, "source-"+name)
	require.NoError(t, os.MkdirAll(sourceDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "main.tf"), []byte("# "+name+"\n"), 0o644))

	target = filepath.Join(base, "target-"+name)
	atmosConfig = &schema.AtmosConfiguration{BasePath: base}

	pkg = NewAtmosVendorPackage(&AtmosPackageParams{
		Name:       name,
		URI:        sourceDir,
		TargetPath: target,
		PkgType:    PkgTypeLocal,
	})

	result, err := Install(atmosConfig, pkg, InstallOptions{})
	require.NoError(t, err)
	require.NoError(t, result.Err)

	return atmosConfig, pkg, target
}

// TestFilterPending_EnforcementLevels is the enforcement-level x drift-state matrix: every
// combination of LockEnforcementSilent/Warn/Strict (and the empty-string default) against a
// materialized package (always dropped) and a drifted one (silent/warn retain it in pending with no
// error; strict withholds it and reports an error instead).
func TestFilterPending_EnforcementLevels(t *testing.T) {
	t.Run("materialized package is always dropped regardless of enforcement level", func(t *testing.T) {
		for _, enforcement := range []string{LockEnforcementSilent, LockEnforcementWarn, LockEnforcementStrict, ""} {
			t.Run(enforcement, func(t *testing.T) {
				atmosConfig, pkg, _ := newMaterializedAtmosPackage(t, "vpc")

				pending, err := FilterPending(atmosConfig, []VendorPackage{pkg}, InstallOptions{LockEnforcement: enforcement})

				require.NoError(t, err)
				assert.Empty(t, pending)
			})
		}
	})

	for _, enforcement := range []string{LockEnforcementSilent, LockEnforcementWarn, ""} {
		t.Run("drifted package is retained in pending with no error under "+enforcement, func(t *testing.T) {
			atmosConfig, pkg, target := newMaterializedAtmosPackage(t, "vpc")
			require.NoError(t, os.WriteFile(filepath.Join(target, "main.tf"), []byte("modified"), 0o644))

			pending, err := FilterPending(atmosConfig, []VendorPackage{pkg}, InstallOptions{LockEnforcement: enforcement})

			require.NoError(t, err)
			require.Len(t, pending, 1)
			assert.Equal(t, "vpc", pending[0].Name)
		})
	}

	t.Run("strict enforcement blocks with ErrLockDriftBlocked instead of returning the package", func(t *testing.T) {
		atmosConfig, pkg, target := newMaterializedAtmosPackage(t, "vpc")
		require.NoError(t, os.WriteFile(filepath.Join(target, "main.tf"), []byte("modified"), 0o644))

		pending, err := FilterPending(atmosConfig, []VendorPackage{pkg}, InstallOptions{LockEnforcement: LockEnforcementStrict})

		require.Error(t, err)
		require.ErrorIs(t, err, ErrLockDriftBlocked)
		assert.Contains(t, err.Error(), "vpc")
		assert.Nil(t, pending, "strict mode must not return a partial pending list on failure")
	})
}

// TestFilterPending_StrictListsEveryDriftedPackage proves strict mode's error names every drifted
// package in one FilterPending call, not just the first one encountered.
func TestFilterPending_StrictListsEveryDriftedPackage(t *testing.T) {
	atmosConfig, pkgA, targetA := newMaterializedAtmosPackage(t, "a")

	sourceDirB := filepath.Join(atmosConfig.BasePath, "source-b")
	require.NoError(t, os.MkdirAll(sourceDirB, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sourceDirB, "main.tf"), []byte("# b\n"), 0o644))
	targetB := filepath.Join(atmosConfig.BasePath, "target-b")
	pkgB := NewAtmosVendorPackage(&AtmosPackageParams{Name: "b", URI: sourceDirB, TargetPath: targetB, PkgType: PkgTypeLocal})
	resultB, err := Install(atmosConfig, pkgB, InstallOptions{})
	require.NoError(t, err)
	require.NoError(t, resultB.Err)

	require.NoError(t, os.WriteFile(filepath.Join(targetA, "main.tf"), []byte("modified-a"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(targetB, "main.tf"), []byte("modified-b"), 0o644))

	_, err = FilterPending(atmosConfig, []VendorPackage{pkgA, pkgB}, InstallOptions{LockEnforcement: LockEnforcementStrict})

	require.Error(t, err)
	require.ErrorIs(t, err, ErrLockDriftBlocked)
	assert.Contains(t, err.Error(), "a (")
	assert.Contains(t, err.Error(), "b (")
}

// TestFilterPending_DryRunAndRefreshLockBypassEnforcement proves DryRun and RefreshLock still
// short-circuit FilterPending to "return every package unfiltered" for every enforcement level,
// including strict -- neither ever blocks or filters, matching this function's documented contract
// that enforcement is irrelevant to a dry run or an explicit refresh.
func TestFilterPending_DryRunAndRefreshLockBypassEnforcement(t *testing.T) {
	for _, enforcement := range []string{LockEnforcementSilent, LockEnforcementWarn, LockEnforcementStrict} {
		t.Run("dry-run/"+enforcement, func(t *testing.T) {
			atmosConfig, pkg, target := newMaterializedAtmosPackage(t, "vpc")
			require.NoError(t, os.WriteFile(filepath.Join(target, "main.tf"), []byte("modified"), 0o644))

			pending, err := FilterPending(atmosConfig, []VendorPackage{pkg}, InstallOptions{DryRun: true, LockEnforcement: enforcement})

			require.NoError(t, err)
			require.Len(t, pending, 1)
		})

		t.Run("refresh-lock/"+enforcement, func(t *testing.T) {
			atmosConfig, pkg, target := newMaterializedAtmosPackage(t, "vpc")
			require.NoError(t, os.WriteFile(filepath.Join(target, "main.tf"), []byte("modified"), 0o644))

			pending, err := FilterPending(atmosConfig, []VendorPackage{pkg}, InstallOptions{RefreshLock: true, LockEnforcement: enforcement})

			require.NoError(t, err)
			require.Len(t, pending, 1)
		})
	}
}
