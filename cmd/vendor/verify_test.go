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

// newVendorVerifyTestCmd builds a throwaway *cobra.Command carrying exactly the flags
// vendorVerifyCmd's RunE closure needs: its own component/format flags (mirroring what
// vendorVerifyParser.RegisterFlags would add to the real command), plus the global flags
// internal/exec.ProcessCommandLineArgs reads directly off cmd.Flags() (base-path, config,
// config-path, profile). Mirrors newVendorCleanTestCmd (clean_test.go) for the identical reason:
// the real vendorVerifyCmd can't be used directly in `go test ./cmd/vendor/...` since it never
// gets RootCmd's persistent global flags in this test binary.
func newVendorVerifyTestCmd() *cobra.Command {
	c := &cobra.Command{Use: "verify"}
	c.Flags().StringP("component", "c", "", "")
	c.Flags().String("format", "table", "")
	c.Flags().String("base-path", "", "")
	c.Flags().StringSlice("config", nil, "")
	c.Flags().StringSlice("config-path", nil, "")
	c.Flags().StringSlice("profile", nil, "")
	return c
}

// writeVerifyLockFixture chdirs into a fresh temp project root, materializes a single lock-owned
// file under a relative "vendor" target, and records it in vendor.lock.yaml under the "mock"
// component name. Returns the absolute path to the materialized file. Mirrors
// writeCleanLockFixture (clean_test.go).
func writeVerifyLockFixture(t *testing.T) (filePath string) {
	t.Helper()

	const component = "mock"

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
	lock.Artifacts["artifact-"+component] = lockfile.Artifact{
		Name:   component,
		Kind:   "source",
		Target: "vendor",
		Files:  files,
	}
	require.NoError(t, lockfile.Save(config, lock))

	return filePath
}

// TestVendorVerifyCmd_NoDriftSucceeds proves an unchanged lock-owned file reports zero drift, a
// success message, and a nil error.
func TestVendorVerifyCmd_NoDriftSucceeds(t *testing.T) {
	writeVerifyLockFixture(t)
	stderr := setupVendorUICapture(t)

	cmd := newVendorVerifyTestCmd()
	err := vendorVerifyCmd.RunE(cmd, nil)

	require.NoError(t, err)
	assert.Contains(t, plainOutput(stderr.String()), "No drift detected")
}

// TestVendorVerifyCmd_ModifiedFileReportsDriftAndFails proves a locally modified lock-owned file
// is reported as drift (checksum mismatch) and RunE returns a non-nil error naming the drift
// count, for CI-friendly non-zero exit behavior.
func TestVendorVerifyCmd_ModifiedFileReportsDriftAndFails(t *testing.T) {
	filePath := writeVerifyLockFixture(t)
	require.NoError(t, os.WriteFile(filePath, []byte("modified-on-disk"), 0o644))
	stderr := setupVendorUICapture(t)

	cmd := newVendorVerifyTestCmd()
	err := vendorVerifyCmd.RunE(cmd, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, errVendorLockDrift)
	assert.Contains(t, err.Error(), "1")
	output := plainOutput(stderr.String())
	assert.Contains(t, output, "mock")
	assert.Contains(t, output, "checksum mismatch")
}

// TestVendorVerifyCmd_MissingFileReportsDrift proves a lock-owned file removed from disk is
// reported as "missing" drift, not silently ignored.
func TestVendorVerifyCmd_MissingFileReportsDrift(t *testing.T) {
	filePath := writeVerifyLockFixture(t)
	require.NoError(t, os.Remove(filePath))
	stderr := setupVendorUICapture(t)

	cmd := newVendorVerifyTestCmd()
	err := vendorVerifyCmd.RunE(cmd, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, errVendorLockDrift)
	output := plainOutput(stderr.String())
	assert.Contains(t, output, "mock")
	assert.Contains(t, output, "missing")
}

// TestVendorVerifyCmd_ComponentFilterScopesResults proves --component only reports drift for the
// named component, silently ignoring drift on other components.
func TestVendorVerifyCmd_ComponentFilterScopesResults(t *testing.T) {
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
	lock.Artifacts["artifact-first"] = lockfile.Artifact{Name: "first", Kind: "source", Target: "vendor-first", Files: firstFiles}
	lock.Artifacts["artifact-second"] = lockfile.Artifact{Name: "second", Kind: "source", Target: "vendor-second", Files: secondFiles}
	require.NoError(t, lockfile.Save(config, lock))

	// Modify both files, but only ask to verify "first".
	require.NoError(t, os.WriteFile(firstFile, []byte("tampered"), 0o644))
	require.NoError(t, os.WriteFile(secondFile, []byte("tampered"), 0o644))

	stderr := setupVendorUICapture(t)
	cmd := newVendorVerifyTestCmd()
	require.NoError(t, cmd.Flags().Set("component", "first"))
	err = vendorVerifyCmd.RunE(cmd, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "1")
	output := plainOutput(stderr.String())
	assert.Contains(t, output, "first")
	assert.NotContains(t, output, "second")
}

// TestVendorVerifyCmd_JSONFormat proves --format json writes structured drift data to stdout
// instead of a table to stderr.
func TestVendorVerifyCmd_JSONFormat(t *testing.T) {
	filePath := writeVerifyLockFixture(t)
	require.NoError(t, os.Remove(filePath))
	stdout := captureVendorStdout(t)

	cmd := newVendorVerifyTestCmd()
	require.NoError(t, cmd.Flags().Set("format", "json"))
	err := vendorVerifyCmd.RunE(cmd, nil)

	require.Error(t, err)
	assert.Contains(t, stdout.String(), `"component"`)
	assert.Contains(t, stdout.String(), `"mock"`)
	assert.Contains(t, stdout.String(), `"missing"`)
}

// TestVendorVerifyCmd_NoLockFileSucceeds proves a project with no vendor.lock.yaml at all (never
// vendored, or freshly cloned) reports zero drift rather than erroring.
func TestVendorVerifyCmd_NoLockFileSucceeds(t *testing.T) {
	root := t.TempDir()
	chdirTest(t, root)
	stderr := setupVendorUICapture(t)

	cmd := newVendorVerifyTestCmd()
	err := vendorVerifyCmd.RunE(cmd, nil)

	require.NoError(t, err)
	assert.Contains(t, plainOutput(stderr.String()), "No drift detected")
}

// TestVendorVerifyCmd_ProcessCommandLineArgsError proves a broken command wiring (a required
// global flag missing off cmd.Flags()) surfaces the ProcessCommandLineArgs error instead of
// panicking or being silently swallowed.
func TestVendorVerifyCmd_ProcessCommandLineArgsError(t *testing.T) {
	// Deliberately omit "base-path", which internal/exec.ProcessCommandLineArgs reads directly
	// off cmd.Flags() and error-checks.
	cmd := &cobra.Command{Use: "verify"}
	cmd.Flags().StringP("component", "c", "", "")
	cmd.Flags().String("format", "table", "")

	err := vendorVerifyCmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "base-path")
}
