package vendor

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/vendoring/updater"
	"github.com/cloudposse/atmos/pkg/vendoring/version"
)

// newExecutionWorktreeFixture stands up a bare "origin" remote plus a cloned workdir with an
// initial vendor.yaml (declaring component "vpc" pinned at 1.0.0) committed and pushed to "main" --
// the shape TestResolveExecutionWorkdir_WorktreeModeIsolatesInvokingCheckout needs to prove
// vendor.update.execution.mode: worktree's isolation guarantee end-to-end, entirely against local
// fixtures (no network).
func newExecutionWorktreeFixture(t *testing.T) (remote, workdir string) {
	t.Helper()
	root := t.TempDir()
	if runtime.GOOS == "windows" {
		// Git for Windows can return successfully while a scanner still holds a short-lived
		// handle in the linked worktree's administrative directory. Remove the fixture before
		// t.TempDir's own cleanup and retry until that handle is released.
		t.Cleanup(func() {
			var cleanupErr error
			require.Eventually(t, func() bool {
				cleanupErr = os.RemoveAll(root)
				return cleanupErr == nil
			}, time.Minute, 250*time.Millisecond, "remove the worktree fixture after Windows releases its file handle")
			require.NoError(t, cleanupErr)
		})
	}
	remote = filepath.Join(root, "remote.git")
	workdir = filepath.Join(root, "workdir")
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		output, err := cmd.CombinedOutput()
		require.NoErrorf(t, err, "git %v failed: %s", args, output)
	}
	run(root, "init", "--bare", remote)
	run(remote, "symbolic-ref", "HEAD", "refs/heads/main")
	run(root, "clone", remote, workdir)
	run(workdir, "config", "user.name", "Atmos Test")
	run(workdir, "config", "user.email", "atmos-test@example.com")
	run(workdir, "config", "commit.gpgSign", "false")
	require.NoError(t, os.WriteFile(filepath.Join(workdir, "vendor.yaml"), []byte(`apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: vpc
      source: github.com/cloudposse/terraform-aws-vpc
      version: 1.0.0
      targets: [components/terraform/vpc]
`), 0o644))
	run(workdir, "add", "vendor.yaml")
	run(workdir, "commit", "-m", "base")
	run(workdir, "branch", "-M", "main")
	run(workdir, "push", "-u", "origin", "main")
	return remote, workdir
}

// TestResolveExecutionWorkdir_WorktreeModeIsolatesInvokingCheckout is the correctness-critical
// proof behind vendor.update.execution.mode: worktree: the discover -> bump cycle
// (updater.ResolveExecutionWorkdir followed by the same runVendorUpdate call vendorUpdateCmd.RunE
// makes) writes its version bump inside the isolated worktree, and the invoking checkout's own
// working tree -- including its branch -- is left completely untouched. Deliberately drives
// updater.ResolveExecutionWorkdir and runVendorUpdate directly rather than the full
// vendorUpdateCmd.RunE (which, under --pull-request, forces an actual "vendor pull"
// materialization requiring a real network-hosted component source -- see the NOTE above
// TestVendorUpdatePullRequestDiscoveryError in component_updater_test.go for the established
// precedent of testing these pieces directly instead). ResolveExecutionWorkdir's own mode-selection
// logic is unit-tested directly in pkg/vendoring/updater; this test is the integration proof that
// cmd/vendor wires it correctly into the rest of the --pull-request publish cycle.
func TestResolveExecutionWorkdir_WorktreeModeIsolatesInvokingCheckout(t *testing.T) {
	_, workdir := newExecutionWorktreeFixture(t)

	lister := &componentUpdaterLister{tags: []string{"1.0.0", "1.1.0"}}
	previous := version.DefaultLister
	version.DefaultLister = lister
	t.Cleanup(func() { version.DefaultLister = previous })

	origWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(workdir)) // Simulate the real CLI: process cwd == the invoking checkout.
	t.Cleanup(func() { require.NoError(t, os.Chdir(origWd)) })

	v := viper.New()
	v.Set("vendor.update.execution.mode", "worktree")

	execWorkdir, err := updater.ResolveExecutionWorkdir(context.Background(), v, workdir)
	require.NoError(t, err)
	worktreePath, cleanup := execWorkdir.Workdir, execWorkdir.Cleanup
	require.NotEmpty(t, worktreePath)
	assert.Equal(t, "main", execWorkdir.ResolvedBase)
	assert.NotEqual(t, workdir, worktreePath)

	// The version-bump write phase: the same runVendorUpdate call vendorUpdateCmd.RunE makes,
	// resolving "vendor.yaml" via its default lookup (no --file override set) -- proving the
	// worktree/BasePath/cwd redirect covers the common case, not just an explicit --file override.
	report, updateErr := runVendorUpdate(&vendorUpdateParams{viper: v, componentType: "terraform", check: false})
	require.NoError(t, updateErr)
	require.Equal(t, 1, report.UpdatedCount())

	// (a) The invoking checkout must be byte-for-byte unchanged: clean status, original branch,
	// original file content.
	statusOut := runGitCapture(t, workdir, "status", "--porcelain")
	assert.Empty(t, statusOut, "invoking checkout must have no working-tree changes")

	branchOut := runGitCapture(t, workdir, "rev-parse", "--abbrev-ref", "HEAD")
	assert.Equal(t, "main", strings.TrimSpace(branchOut), "invoking checkout must remain on its original branch")

	invokingContent, readErr := os.ReadFile(filepath.Join(workdir, "vendor.yaml"))
	require.NoError(t, readErr)
	assert.Contains(t, string(invokingContent), "1.0.0", "invoking checkout's vendor.yaml must retain the original version")
	assert.NotContains(t, string(invokingContent), "1.1.0", "invoking checkout's vendor.yaml must not carry the bump")

	// (b) The worktree, inspected before cleanup runs, has the bumped version.
	worktreeContent, readErr := os.ReadFile(filepath.Join(worktreePath, "vendor.yaml"))
	require.NoError(t, readErr)
	assert.Contains(t, string(worktreeContent), "1.1.0", "worktree's vendor.yaml must carry the bumped version")

	cleanup()

	_, statErr := os.Stat(worktreePath)
	assert.True(t, os.IsNotExist(statErr), "cleanup must remove the worktree")

	// cleanup must also have restored ATMOS_BASE_PATH and the process cwd.
	_, basePathSet := os.LookupEnv("ATMOS_BASE_PATH")
	assert.False(t, basePathSet, "cleanup must restore ATMOS_BASE_PATH to its original (unset) state")
	restoredWd, err := os.Getwd()
	require.NoError(t, err)
	// Compare resolved symlinks since t.TempDir() can return a symlinked path on macOS.
	resolvedWorkdir, err := filepath.EvalSymlinks(workdir)
	require.NoError(t, err)
	resolvedRestored, err := filepath.EvalSymlinks(restoredWd)
	require.NoError(t, err)
	assert.Equal(t, resolvedWorkdir, resolvedRestored, "cleanup must restore the process working directory")
}

// runGitCapture runs a git command in dir and returns its combined output, failing the test on error.
func runGitCapture(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %v failed: %s", args, out)
	return string(out)
}
