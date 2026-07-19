package vendor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/vendoring/lockfile"
)

// newVendorCleanTestCmd builds a throwaway *cobra.Command carrying exactly the flags
// vendorCleanCmd's RunE closure needs: its own component/force/dry-run flags, plus the
// global flags internal/exec.ProcessCommandLineArgs reads directly off cmd.Flags()
// (base-path, config, config-path, profile). The real vendorCleanCmd can't be used for
// this in `go test ./cmd/vendor/...`: those global flags only exist as RootCmd's
// persistent flags, which live in package `cmd` (the root command package) - a package
// this test binary never compiles or links, since cmd/vendor is a dependency of cmd, not
// the reverse. Mirrors newVendorPullTestCmd (update.go's tests, vendor_test.go) for the
// identical reason.
func newVendorCleanTestCmd() *cobra.Command {
	c := &cobra.Command{Use: "clean"}
	c.Flags().StringP("component", "c", "", "")
	c.Flags().Bool("force", false, "")
	c.Flags().Bool("dry-run", false, "")
	c.Flags().String("base-path", "", "")
	c.Flags().StringSlice("config", nil, "")
	c.Flags().StringSlice("config-path", nil, "")
	c.Flags().StringSlice("profile", nil, "")
	return c
}

// writeCleanLockFixture chdirs into a fresh temp project root, materializes a single
// lock-owned file under a relative "vendor" target, and records it in vendor.lock.yaml
// under the "mock" component name. Returns the absolute path to the materialized file.
// BasePath: "." mirrors what cfg.InitCliConfig actually produces for a directory with no
// atmos.yaml (proven by inspecting InitCliConfig's output for a chdir'd temp dir), so the
// fixture is read back from the exact same resolved path vendorCleanCmd's RunE uses.
func writeCleanLockFixture(t *testing.T) (filePath string) {
	t.Helper()

	root := t.TempDir()
	chdirTest(t, root)

	targetDir := filepath.Join(root, "vendor")
	require.NoError(t, os.MkdirAll(targetDir, 0o755))
	filePath = filepath.Join(targetDir, "owned.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("original"), 0o644))

	config := &schema.AtmosConfiguration{BasePath: "."}
	files, err := lockfile.Inventory(targetDir)
	require.NoError(t, err)

	lock := lockfile.New()
	lock.Artifacts["artifact-mock"] = lockfile.Artifact{
		Component: "mock",
		Kind:      "source",
		Target:    "vendor",
		Files:     files,
	}
	require.NoError(t, lockfile.Save(config, lock))

	return filePath
}

// TestVendorCleanCmd_RemovesLockOwnedFiles proves the happy path end to end: a clean lock
// entry for a materialized file is removed from disk, reported via "Removed", and dropped
// from vendor.lock.yaml.
func TestVendorCleanCmd_RemovesLockOwnedFiles(t *testing.T) {
	filePath := writeCleanLockFixture(t)
	stderr := setupVendorUICapture(t)

	cmd := newVendorCleanTestCmd()
	err := vendorCleanCmd.RunE(cmd, nil)
	require.NoError(t, err)

	assert.NoFileExists(t, filePath)
	assert.Contains(t, plainOutput(stderr.String()), "Removed")

	config := &schema.AtmosConfiguration{BasePath: "."}
	loaded, err := lockfile.Load(config)
	require.NoError(t, err)
	assert.Empty(t, loaded.Artifacts, "cleaned artifact must be dropped from the lock")
}

// TestVendorCleanCmd_DryRunPreservesFiles proves --dry-run reports what would be removed
// without touching the filesystem or the lock.
func TestVendorCleanCmd_DryRunPreservesFiles(t *testing.T) {
	filePath := writeCleanLockFixture(t)
	stderr := setupVendorUICapture(t)

	cmd := newVendorCleanTestCmd()
	require.NoError(t, cmd.Flags().Set("dry-run", "true"))
	err := vendorCleanCmd.RunE(cmd, nil)
	require.NoError(t, err)

	assert.FileExists(t, filePath, "dry-run must not remove the file")
	assert.Contains(t, plainOutput(stderr.String()), "Would remove")

	config := &schema.AtmosConfiguration{BasePath: "."}
	loaded, err := lockfile.Load(config)
	require.NoError(t, err)
	assert.Contains(t, loaded.Artifacts, "artifact-mock", "dry-run must not touch the lock")
}

// TestVendorCleanCmd_ModifiedFilePreservedWithoutForce proves a locally modified lock-owned
// file is preserved (not deleted) and reported as a conflict, and that RunE returns
// errModifiedVendorFiles wrapped with the conflict count, unless --force is given.
func TestVendorCleanCmd_ModifiedFilePreservedWithoutForce(t *testing.T) {
	filePath := writeCleanLockFixture(t)
	require.NoError(t, os.WriteFile(filePath, []byte("modified-on-disk"), 0o644))
	stderr := setupVendorUICapture(t)

	cmd := newVendorCleanTestCmd()
	err := vendorCleanCmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errModifiedVendorFiles)
	assert.Contains(t, err.Error(), "1")

	assert.FileExists(t, filePath, "a modified lock-owned file must be preserved without --force")
	assert.Equal(t, "modified-on-disk", readFile(t, filePath))
	assert.Contains(t, plainOutput(stderr.String()), "Preserved modified vendor file")

	config := &schema.AtmosConfiguration{BasePath: "."}
	loaded, err := lockfile.Load(config)
	require.NoError(t, err)
	assert.Contains(t, loaded.Artifacts, "artifact-mock", "a conflicted clean must leave the lock untouched")
}

// TestVendorCleanCmd_ForceRemovesModifiedFiles proves --force deletes a locally modified
// lock-owned file instead of preserving it.
func TestVendorCleanCmd_ForceRemovesModifiedFiles(t *testing.T) {
	filePath := writeCleanLockFixture(t)
	require.NoError(t, os.WriteFile(filePath, []byte("modified-on-disk"), 0o644))
	setupVendorUICapture(t)

	cmd := newVendorCleanTestCmd()
	require.NoError(t, cmd.Flags().Set("force", "true"))
	err := vendorCleanCmd.RunE(cmd, nil)
	require.NoError(t, err)

	assert.NoFileExists(t, filePath, "--force must remove even a modified lock-owned file")
}

// TestVendorCleanCmd_ComponentFilterScopesRemoval proves --component only cleans the named
// component's artifact, leaving files owned by other components untouched.
func TestVendorCleanCmd_ComponentFilterScopesRemoval(t *testing.T) {
	root := t.TempDir()
	chdirTest(t, root)

	firstDir := filepath.Join(root, "vendor-first")
	secondDir := filepath.Join(root, "vendor-second")
	require.NoError(t, os.MkdirAll(firstDir, 0o755))
	require.NoError(t, os.MkdirAll(secondDir, 0o755))
	firstFile := filepath.Join(firstDir, "owned.txt")
	secondFile := filepath.Join(secondDir, "owned.txt")
	require.NoError(t, os.WriteFile(firstFile, []byte("first"), 0o644))
	require.NoError(t, os.WriteFile(secondFile, []byte("second"), 0o644))

	config := &schema.AtmosConfiguration{BasePath: "."}
	firstFiles, err := lockfile.Inventory(firstDir)
	require.NoError(t, err)
	secondFiles, err := lockfile.Inventory(secondDir)
	require.NoError(t, err)

	lock := lockfile.New()
	lock.Artifacts["artifact-first"] = lockfile.Artifact{Component: "first", Kind: "source", Target: "vendor-first", Files: firstFiles}
	lock.Artifacts["artifact-second"] = lockfile.Artifact{Component: "second", Kind: "source", Target: "vendor-second", Files: secondFiles}
	require.NoError(t, lockfile.Save(config, lock))

	setupVendorUICapture(t)
	cmd := newVendorCleanTestCmd()
	require.NoError(t, cmd.Flags().Set("component", "first"))
	err = vendorCleanCmd.RunE(cmd, nil)
	require.NoError(t, err)

	assert.NoFileExists(t, firstFile, "the selected component's file must be removed")
	assert.FileExists(t, secondFile, "an unselected component's file must be left alone")

	loaded, err := lockfile.Load(config)
	require.NoError(t, err)
	assert.NotContains(t, loaded.Artifacts, "artifact-first")
	assert.Contains(t, loaded.Artifacts, "artifact-second")
}

// TestVendorCleanCmd_NoLockOwnedFilesIsANoop proves an empty/absent lock produces neither an
// error nor any output, exercising the Removed/Conflicts loops with zero elements.
func TestVendorCleanCmd_NoLockOwnedFilesIsANoop(t *testing.T) {
	root := t.TempDir()
	chdirTest(t, root)
	stderr := setupVendorUICapture(t)

	cmd := newVendorCleanTestCmd()
	err := vendorCleanCmd.RunE(cmd, nil)
	require.NoError(t, err)
	assert.Empty(t, plainOutput(stderr.String()))
}

// TestVendorCleanCmd_ProcessCommandLineArgsError proves a broken command wiring (a required
// global flag missing off cmd.Flags()) surfaces the ProcessCommandLineArgs error instead of
// panicking or being silently swallowed.
func TestVendorCleanCmd_ProcessCommandLineArgsError(t *testing.T) {
	// Deliberately omit "base-path", which internal/exec.ProcessCommandLineArgs reads directly
	// off cmd.Flags() and error-checks.
	cmd := &cobra.Command{Use: "clean"}
	cmd.Flags().StringP("component", "c", "", "")
	cmd.Flags().Bool("force", false, "")
	cmd.Flags().Bool("dry-run", false, "")

	err := vendorCleanCmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "base-path")
}

// TestVendorCleanCmd_InitCliConfigError proves an invalid --config path (an explicit config
// file that does not exist) surfaces the cfg.InitCliConfig error.
func TestVendorCleanCmd_InitCliConfigError(t *testing.T) {
	root := t.TempDir()
	chdirTest(t, root)

	cmd := newVendorCleanTestCmd()
	require.NoError(t, cmd.Flags().Set("config", filepath.Join(root, "does-not-exist.yaml")))
	err := vendorCleanCmd.RunE(cmd, nil)
	require.Error(t, err)
}

// TestVendorCleanCmd_LockfileCleanErrorPropagates proves a lockfile.Clean error (a lock
// entry whose target escapes the project root, e.g. from a tampered vendor.lock.yaml) is
// returned by RunE rather than swallowed, and that the file outside the project is left
// untouched.
func TestVendorCleanCmd_LockfileCleanErrorPropagates(t *testing.T) {
	rootParent := t.TempDir()
	root := filepath.Join(rootParent, "project")
	outside := filepath.Join(rootParent, "outside")
	require.NoError(t, os.MkdirAll(root, 0o755))
	require.NoError(t, os.MkdirAll(outside, 0o755))
	outsideFile := filepath.Join(outside, "owned.txt")
	require.NoError(t, os.WriteFile(outsideFile, []byte("do not remove"), 0o644))
	chdirTest(t, root)

	config := &schema.AtmosConfiguration{BasePath: "."}
	maliciousLock := `version: 1
artifacts:
  malicious:
    component: malicious
    kind: local
    target: ` + outside + `
    source: {}
    files:
      - path: owned.txt
        type: file
        mode: 420
        sha256: ignored
    order: 1
`
	require.NoError(t, os.WriteFile(lockfile.Path(config), []byte(maliciousLock), 0o644))

	cmd := newVendorCleanTestCmd()
	err := vendorCleanCmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.FileExists(t, outsideFile, "a lockfile.Clean error must leave files outside the project untouched")
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}
