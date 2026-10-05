package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// packerFixtureWorkDir copies the shared "../tests/fixtures/scenarios/packer"
// fixture into a private per-test temp directory and returns its path.
//
// Windows acceptance shard 1 runs this cmd test binary concurrently with
// internal/exec's TestExecutePacker_Validate. Both commands generate and
// clean up the same stack/component var-file, so sharing the tracked fixture
// lets one process remove it while the other's Packer process opens it.
// Under the race job's -shuffle=on -parallel=4, cmd-level packer tests race
// on the same tracked fixture directory for the same reason (seen as
// "Failed to open file nonprod-aws-bastion.packer.vars.json" failures in
// TestPackerInitCmd/TestPackerInspectCmd). Giving every test that actually
// invokes packer its own copy keeps these integration tests deterministic
// on every OS.
func packerFixtureWorkDir(t *testing.T) string {
	t.Helper()

	fixtureDir := "../tests/fixtures/scenarios/packer"
	workDir := filepath.Join(t.TempDir(), "packer")
	require.NoError(t, os.CopyFS(workDir, os.DirFS(fixtureDir)))
	return workDir
}
