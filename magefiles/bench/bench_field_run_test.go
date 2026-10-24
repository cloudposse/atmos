//go:build mage

package bench

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/magefiles/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestBenchFieldTestBuildsBeforeMeasuringAndCleansWorktrees(t *testing.T) {
	root := benchSetUpRun(t)
	testutil.RunGit(t, root, "commit", "--allow-empty", "-qm", "candidate")
	out := filepath.Join(t.TempDir(), "field")
	require.NoError(t, (Bench{}).FieldTest("HEAD~1,HEAD", out))
	data, err := os.ReadFile(filepath.Join(out, "report.md"))
	require.NoError(t, err)
	require.Contains(t, string(data), "Known-good reference: `HEAD~1`")
	require.Contains(t, string(data), "No significant regressions")
	versions, err := benchFieldVersions(root, "HEAD~1,HEAD")
	require.NoError(t, err)
	for _, version := range versions {
		snapshot, err := benchLoad(filepath.Join(out, benchFieldMeasurements, version.SHA))
		require.NoError(t, err)
		require.Equal(t, benchFieldMode, snapshot.Manifest.Mode)
		require.Equal(t, version.Ref, snapshot.Manifest.SourceRef)
		require.Empty(t, snapshot.Manifest.RunID)
		require.NotEmpty(t, snapshot.Manifest.HarnessSHA)
		require.FileExists(t, filepath.Join(out, "binaries", version.SHA))
		require.NoDirExists(t, filepath.Join(out, "source-"+version.SHA))
	}
	commands, err := os.ReadFile(os.Getenv("ATMOS_BENCH_TEST_LOG"))
	require.NoError(t, err)
	require.Less(t, strings.LastIndex(string(commands), `"build"`), strings.Index(string(commands), `"test"`))
	require.Contains(t, string(commands), `"ATMOS_BENCH_OBSERVE_STORE_READS":"true"`)
	require.ErrorContains(t, (Bench{}).FieldTest("HEAD~1,HEAD", out), "must be empty")
}

func TestBenchFieldFailures(t *testing.T) {
	for _, stage := range []string{"build", "test", "env", "bad ref"} {
		t.Run(stage, func(t *testing.T) {
			root := benchSetUpRun(t)
			testutil.RunGit(t, root, "commit", "--allow-empty", "-qm", "candidate")
			out := filepath.Join(t.TempDir(), "field")
			t.Setenv("ATMOS_BENCH_TEST_FAIL", stage)
			refs := "HEAD~1,HEAD"
			if stage == "bad ref" {
				refs = "-invalid,HEAD"
			}
			require.Error(t, (Bench{}).FieldTest(refs, out))
			trees, err := benchOutput(root, "git", "worktree", "list", "--porcelain")
			require.NoError(t, err)
			require.Equal(t, 1, strings.Count(trees, "worktree "))
			require.NoFileExists(t, filepath.Join(out, "report.md"))
		})
	}
}

func TestBenchFieldFilesystemErrors(t *testing.T) {
	root := benchSetUpRun(t)
	dir := t.TempDir()
	version := benchFieldVersion{Ref: "HEAD"}
	var err error
	version.SHA, err = benchOutput(root, "git", "rev-parse", "HEAD")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "blocked"), nil, 0o600))
	require.Error(t, benchFieldOutput(filepath.Join(dir, "blocked")))
	require.Error(t, benchFieldMeasure(context.Background(), root, filepath.Join(dir, "blocked"), version))
	require.Error(t, benchFieldCollect(context.Background(), root, dir, version, &benchManifest{Samples: 10}))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "build-"+version.SHA+".log"), 0o700))
	require.Error(t, benchFieldBuild(context.Background(), root, dir, version))
	require.NoDirExists(t, filepath.Join(dir, "source-"+version.SHA))
}
