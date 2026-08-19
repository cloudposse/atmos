package toolchain

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain/lockfile"
)

func TestRunLock_RejectsInvalidMaxConcurrency(t *testing.T) {
	err := RunLock(nil, LockOptions{MaxConcurrency: 0})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidFlagValue)
}

func TestRunLock_NoToolsConfigured(t *testing.T) {
	setupTestIO(t)
	filePath := createTempToolVersionsFile(t, "")
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: filePath}})

	err := RunLock(nil, LockOptions{MaxConcurrency: 4})
	require.NoError(t, err)
}

func TestRunLock_UnknownToolRequested(t *testing.T) {
	setupTestIO(t)
	filePath := createTempToolVersionsFile(t, "owner/repo 1.0.0\n")
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: filePath}})

	err := RunLock([]string{"nonexistent-tool"}, LockOptions{MaxConcurrency: 4})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrToolNotFound)
}

// TestResolveLockTargets covers the empty-args-means-everything convention (sorted for
// determinism, since it's built from a map), per-version expansion of multi-version
// .tool-versions lines, and alias resolution to a tool's default version for explicit
// names.
func TestResolveLockTargets(t *testing.T) {
	toolVersions := &ToolVersions{Tools: map[string][]string{
		"hashicorp/terraform": {"1.11.4", "1.9.8"},
		"jqlang/jq":           {"jq-1.7.1"},
	}}

	t.Run("empty selects everything, sorted", func(t *testing.T) {
		targets, err := resolveLockTargets(toolVersions, nil)
		require.NoError(t, err)
		require.Len(t, targets, 3)
		// Sorted by owner, then repo, then version.
		assert.Equal(t, toolInfo{"1.11.4", "hashicorp", "terraform"}, targets[0])
		assert.Equal(t, toolInfo{"1.9.8", "hashicorp", "terraform"}, targets[1])
		assert.Equal(t, toolInfo{"jq-1.7.1", "jqlang", "jq"}, targets[2])
	})

	t.Run("explicit name resolves to default (first) version", func(t *testing.T) {
		targets, err := resolveLockTargets(toolVersions, []string{"hashicorp/terraform"})
		require.NoError(t, err)
		require.Len(t, targets, 1)
		assert.Equal(t, toolInfo{"1.11.4", "hashicorp", "terraform"}, targets[0])
	})

	t.Run("unknown tool errors", func(t *testing.T) {
		_, err := resolveLockTargets(toolVersions, []string{"nonexistent"})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrToolNotFound)
	})
}

// TestRunLock_ReportsInTargetOrder verifies that concurrent workers (MaxConcurrency > 1)
// report outcomes in the original target order, not completion order -- mirroring the
// same guarantee (and the same regression class) fixed for RunUpdate. Uses tools that
// don't exist in any registry, so every worker fails at the same FindTool step -- the
// property under test is reporting order, not success.
// TestRunLock_ReportsAllTargets confirms every target is reported exactly once. Lines now print
// live as each tool completes (via runConcurrentBatchWithLiveProgress, matching `atmos toolchain
// install`'s batch-mode convention) instead of being buffered and printed in original target
// order after the whole batch finishes -- so completion order, not target order, is what's
// observable here. See TestRunConcurrentBatchWithLiveProgress_ResultsPreserveItemOrder
// (batch_progress_test.go) for the guarantee that still holds regardless of completion order:
// the underlying per-item results are never misattributed to the wrong item.
//
// Uses captureUITestOutput, not captureCleanTestOutput: the latter forces TTY mode and redirects
// stderr through an os.Pipe that's only drained after the tested function returns. Combined with
// this test's real concurrent batch (which activates the live, ticker-driven renderer under
// force-tty), that redirect deadlocked on Windows CI: the renderer's repeated writes filled the
// pipe's bounded OS buffer, and since nothing reads it until RunLock returns, the write blocked
// forever, hanging the test for the full 40-minute Go test timeout. See
// docs/fixes/2026-08-08-toolchain-live-renderer-windows-ci-deadlock.md.
func TestRunLock_ReportsAllTargets(t *testing.T) {
	filePath := createTempToolVersionsFile(t, "nonexistent-owner/aaa 1.0.0\nnonexistent-owner/bbb 1.0.0\nnonexistent-owner/ccc 1.0.0\n")
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: filePath}})

	var err error
	output := captureUITestOutput(t, func() {
		err = RunLock(nil, LockOptions{MaxConcurrency: 4})
	})
	require.Error(t, err, "tools that don't exist in any registry should be reported as failed")

	assert.Contains(t, output, "aaa")
	assert.Contains(t, output, "bbb")
	assert.Contains(t, output, "ccc")
}

// TestRunLock_ForceWritesLockFileWithoutInstalling verifies the actual contract `atmos
// toolchain lock` exists to provide: even with toolchain.use_lock_file: false (so a real
// `atmos toolchain install` wouldn't write a lock entry), RunLock still writes a real,
// checksum-populated toolchain.lock.yaml entry -- and never extracts/installs the binary
// into the toolchain tree. Mirrors TestLockTool_WritesLockEntryWithoutInstalling
// (installer package, unit-level) at the RunLock orchestration layer, using the same
// real-registry-lookup pattern as TestRunInstall_WithCanonicalFormat (install_test.go).
func TestRunLock_ForceWritesLockFileWithoutInstalling(t *testing.T) {
	setupTestIO(t)
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	filePath := createTempToolVersionsFile(t, "hashicorp/terraform 1.11.4\n")

	// InstallPath MUST be isolated to a per-test temp dir: RunLock performs a real
	// registry lookup + download via NewInstaller(), and without an explicit
	// InstallPath, GetInstallPath() falls back to the real, shared, XDG toolchain
	// cache directory the rest of the acceptance suite depends on (see the identical
	// caveat on TestRunInstall_WithCanonicalFormat in install_test.go).
	prevConfig := atmosConfig
	installPath := filepath.Join(tempDir, ".atmos", "tools")
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: filePath, InstallPath: installPath, UseLockFile: false}})
	defer func() {
		SetAtmosConfig(prevConfig)
	}()

	err := RunLock(nil, LockOptions{MaxConcurrency: 4})
	require.NoError(t, err)

	// The lock file must be written despite use_lock_file: false -- that's the whole
	// point of `atmos toolchain lock`'s force-write behavior.
	lockFilePath := filepath.Join(installPath, "toolchain.lock.yaml")
	lockFile, err := lockfile.Load(lockFilePath)
	require.NoError(t, err)
	tool := lockFile.Tools["hashicorp/terraform"]
	require.NotNil(t, tool, "expected a hashicorp/terraform lock entry")
	entry := tool.Versions["1.11.4"]
	require.NotNil(t, entry, "expected a locked entry for version 1.11.4")
	platform := entry.Platforms[runtime.GOOS+"_"+runtime.GOARCH]
	require.NotNil(t, platform, "expected a lock entry for the current platform")
	assert.NotEmpty(t, platform.ChecksumAlgorithm)
	assert.NotEmpty(t, platform.Checksum)

	// No binary may be installed -- locking must never extract/install into binDir
	// (GetInstallPath()/bin).
	binDir := filepath.Join(installPath, "bin")
	entries, statErr := os.ReadDir(binDir)
	if statErr == nil {
		assert.Empty(t, entries, "RunLock must not install a binary into binDir")
	} else {
		assert.True(t, os.IsNotExist(statErr), "unexpected error reading binDir: %v", statErr)
	}
}

// TestRunLock_ToolVersionsFileLoadError verifies RunLock surfaces a wrapped
// ErrToolVersionsFileOperation (rather than a bare os.ReadFile error) when the configured
// .tool-versions file can't even be read -- e.g. its parent directory doesn't exist yet.
func TestRunLock_ToolVersionsFileLoadError(t *testing.T) {
	setupTestIO(t)
	missing := filepath.Join(t.TempDir(), "does-not-exist-dir", ".tool-versions")
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: missing}})
	t.Cleanup(func() { SetAtmosConfig(nil) })

	err := RunLock(nil, LockOptions{MaxConcurrency: 4})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrToolVersionsFileOperation)
}

// TestTallyLockOutcomes_NoOutcomes_PrintsZero covers tallyLockOutcomes' defensive default
// branch (reached when neither "locked" nor "failed" segments end up non-empty), asserting the
// summary line it prints and that it correctly reports zero failures.
func TestTallyLockOutcomes_NoOutcomes_PrintsZero(t *testing.T) {
	var failed int
	output := captureUITestOutput(t, func() {
		failed = tallyLockOutcomes(nil)
	})
	assert.Equal(t, 0, failed)
	assert.Contains(t, output, "Locked 0 tool(s)")
}

// TestTallyLockOutcomes_MixedResults verifies both the locked and failed segments are counted
// and printed together, and that the returned failure count matches.
func TestTallyLockOutcomes_MixedResults(t *testing.T) {
	outcomes := []lockOutcome{
		{result: lockResultLocked, message: "a@1.0.0 locked"},
		{result: lockResultLocked, message: "b@1.0.0 locked"},
		{result: lockResultFailed, message: "c@1.0.0: boom"},
	}
	var failed int
	output := captureUITestOutput(t, func() {
		failed = tallyLockOutcomes(outcomes)
	})
	assert.Equal(t, 1, failed)
	assert.Contains(t, output, "Locked 2 tool(s)")
	assert.Contains(t, output, "1 failed")
}

// TestResolveLockTargets_InvalidResolvedKey_PropagatesParseError covers the ParseToolSpec
// error branch: a raw .tool-versions key with more than one slash (technically possible since
// keys are free-form tokens, not validated at parse time) resolves via LookupToolVersion's
// exact-match path, but then fails installer.ParseToolSpec's owner/repo split.
func TestResolveLockTargets_InvalidResolvedKey_PropagatesParseError(t *testing.T) {
	toolVersions := &ToolVersions{Tools: map[string][]string{
		"a/b/c": {"1.0.0"},
	}}

	_, err := resolveLockTargets(toolVersions, []string{"a/b/c"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidToolSpec)
}

// TestLockOneTool_ResolveLatestVersionError verifies a tool pinned to "latest" that resolves to
// a registry lookup failure (owner/repo doesn't exist anywhere) is reported as a failed outcome
// with a "failed to resolve version" message, not a panic or a misleadingly "locked" result.
func TestLockOneTool_ResolveLatestVersionError(t *testing.T) {
	setupTestIO(t)

	outcome := lockOneTool(toolInfo{owner: "nonexistent-owner-abcxyz", repo: "nonexistent-repo-abcxyz", version: "latest"})

	assert.Equal(t, lockResultFailed, outcome.result)
	assert.Contains(t, outcome.message, "failed to resolve version")
}

// TestLockOneTool_LockToolError verifies that a real, findable tool with a version that doesn't
// actually exist as a release (so FindTool succeeds but the download/verify step in LockTool
// fails) is reported as a failed outcome, not treated as locked.
func TestLockOneTool_LockToolError(t *testing.T) {
	setupTestIO(t)
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	prevConfig := atmosConfig
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{
		InstallPath: filepath.Join(tempDir, ".atmos", "tools"),
	}})
	t.Cleanup(func() { SetAtmosConfig(prevConfig) })

	outcome := lockOneTool(toolInfo{owner: "hashicorp", repo: "terraform", version: "0.0.0-nonexistent-version"})

	assert.Equal(t, lockResultFailed, outcome.result)
}
