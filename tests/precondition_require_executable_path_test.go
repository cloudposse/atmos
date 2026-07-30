package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withShortExecutablePathRetry temporarily shrinks the requireExecutablePath
// retry window so tests can exercise the retry loop's sleep/exhaustion
// branches without waiting out the real 15s production timeout.
func withShortExecutablePathRetry(t *testing.T, timeout, interval time.Duration) {
	t.Helper()

	origTimeout, origInterval := executablePathRetryTimeout, executablePathRetryInterval
	executablePathRetryTimeout = timeout
	executablePathRetryInterval = interval
	t.Cleanup(func() {
		executablePathRetryTimeout = origTimeout
		executablePathRetryInterval = origInterval
	})
}

// TestRequireExecutablePath_ResolvesOnFirstLookup covers the immediate-success
// path: exec.LookPath finds the tool on the very first attempt, so the retry
// loop's sleep branch is never taken, and the resolved path is returned
// directly (no separate follow-up LookPath call, per the TOCTOU-closing
// contract described in the function's doc comment).
func TestRequireExecutablePath_ResolvesOnFirstLookup(t *testing.T) {
	os.Unsetenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS")

	binDir := t.TempDir()
	binName := cachedTestToolBinaryName("atmos-require-path-fake")
	binPath := filepath.Join(binDir, binName)
	require.NoError(t, os.WriteFile(binPath, []byte("fake\n"), 0o755))
	t.Setenv("PATH", binDir)

	got := requireExecutablePath(t, "atmos-require-path-fake", "testing")
	assert.Equal(t, binPath, got)
}

// TestRequireExecutablePath_RetriesUntilResolved covers the poll/sleep branch:
// the binary isn't present on the first lookup but appears shortly after,
// simulating CI's "PATH updated a step earlier, not yet visible" race that
// this retry loop exists to absorb.
func TestRequireExecutablePath_RetriesUntilResolved(t *testing.T) {
	os.Unsetenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS")
	withShortExecutablePathRetry(t, 2*time.Second, 10*time.Millisecond)

	binDir := t.TempDir()
	binName := cachedTestToolBinaryName("atmos-require-path-delayed")
	binPath := filepath.Join(binDir, binName)
	t.Setenv("PATH", binDir)

	done := make(chan struct{})
	go func() {
		defer close(done)
		time.Sleep(60 * time.Millisecond)
		_ = os.WriteFile(binPath, []byte("fake\n"), 0o755)
	}()
	t.Cleanup(func() { <-done })

	got := requireExecutablePath(t, "atmos-require-path-delayed", "testing")
	assert.Equal(t, binPath, got)
}

// TestRequireExecutablePath_ExhaustedRetriesSkip covers the
// retries-exhausted-but-checks-enabled path: the binary never appears, so the
// loop must give up at the deadline and skip (not fail) the test, having
// generated forensics output first. The retry window is shrunk so this
// doesn't cost the real 15s on every run.
//
// The sibling "checks disabled -> t.Fatalf" branch (requireExecutablePath's
// ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true path) is intentionally not
// exercised here: calling it directly would fail this test (t.Fatalf marks
// the calling (sub)test as failed, not skipped), and testing it via a
// subprocess re-exec of this test binary would also re-run this package's
// TestMain (cli_test.go), which provisions real toolchain binaries and can
// start Floci emulators -- a disproportionate cost/risk for covering a single
// already-templated error-message call whose surrounding logic (retry loop,
// forensics generation, ShouldCheckPreconditions branch selection) is
// otherwise identical to, and already covered by, the skip path below.
func TestRequireExecutablePath_ExhaustedRetriesSkip(t *testing.T) {
	os.Unsetenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS")
	withShortExecutablePathRetry(t, 20*time.Millisecond, 5*time.Millisecond)

	t.Setenv("PATH", t.TempDir())

	t.Run("missing binary skips", func(t *testing.T) {
		requireExecutablePath(t, "definitely-not-a-real-binary-req-path-xyz", "testing")
		t.Error("expected requireExecutablePath to skip a test when the binary never resolves")
	})
}

// TestRequireTerraformPath_WrapsRequireExecutablePath and
// TestRequireTofuPath_WrapsRequireExecutablePath confirm the thin wrappers
// delegate to requireExecutablePath: whether or not the real binary resolves
// in this environment, the call completes without a hard failure -- either it
// skips (tool absent) or it returns a real, exec.LookPath-equivalent path.
func TestRequireTerraformPath_WrapsRequireExecutablePath(t *testing.T) {
	got := RequireTerraformPath(t)

	want, err := exec.LookPath("terraform")
	require.NoError(t, err, "terraform must be resolvable once RequireTerraformPath returns without skipping")
	assert.Equal(t, want, got)
}

func TestRequireTofuPath_WrapsRequireExecutablePath(t *testing.T) {
	got := RequireTofuPath(t)

	want, err := exec.LookPath("tofu")
	require.NoError(t, err, "tofu must be resolvable once RequireTofuPath returns without skipping")
	assert.Equal(t, want, got)
}
