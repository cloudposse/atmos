package initcmd

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

const initRenderedE2EScaffoldYAML = `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: init-rendered-e2e
spec:
  fields:
    - name: project_name
      type: input
      default: demo
`

// requireGitBinaryForInitRenderedE2E mirrors cmd/scaffold's
// requireGitBinaryForRenderedE2E: go-getter's git:: fetch (used by
// source.Resolve/Hydrate) shells out to a real git binary regardless of how
// the fixture repo below is built.
func requireGitBinaryForInitRenderedE2E(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not found on PATH; required by go-getter's git clone")
	}
}

// readInitRenderedFile mirrors cmd/scaffold's readRenderedFile: normalizes
// CRLF to LF since Windows runners' core.autocrlf=true rewrites the
// fixture's LF line endings on checkout.
func readInitRenderedFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	return strings.ReplaceAll(string(content), "\r\n", "\n")
}

func initRenderedE2EFileURI(path string) string {
	cleaned := filepath.ToSlash(filepath.Clean(path))
	if filepath.VolumeName(path) != "" && cleaned != "" && cleaned[0] != '/' {
		cleaned = "/" + cleaned
	}
	return (&url.URL{Scheme: "file", Path: cleaned}).String()
}

// buildInitConflictThresholdTemplateRepo mirrors cmd/scaffold's
// buildConflictThresholdTemplateRepo: a two-tag template repo where
// conflict.txt's content is engineered so a single-line, same-hunk
// disagreement between base/ours/theirs lands at exactly 66% changed by
// pkg/generator/merge's LCS-based change-percentage calculation (2 lines
// counted changed on each of the ours/theirs sides, over a 6-line base:
// int(4/6*100) == 66) -- comfortably above the default 50% threshold but
// comfortably below 100.
func buildInitConflictThresholdTemplateRepo(t *testing.T) (repoDir string) {
	t.Helper()
	repoDir = t.TempDir()
	repo, err := git.PlainInitWithOptions(repoDir, &git.PlainInitOptions{
		InitOptions: git.InitOptions{DefaultBranch: plumbing.NewBranchReferenceName("main")},
	})
	require.NoError(t, err)
	wt, err := repo.Worktree()
	require.NoError(t, err)

	writeFile := func(name, content string) {
		path := filepath.Join(repoDir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
	sig := &object.Signature{Name: "Test User", Email: "test@example.com", When: time.Now()}

	// No trailing newline: a trailing newline would split into an extra
	// empty-string element that always matches across base/ours/theirs,
	// diluting the change percentage away from the exact 66% this fixture
	// depends on.
	writeFile("scaffold.yaml", initRenderedE2EScaffoldYAML)
	writeFile("conflict.txt", "template-v1\nl2\nl3\nl4\nl5\nl6")
	require.NoError(t, wt.AddGlob("."))
	commit1, err := wt.Commit("v1", &git.CommitOptions{Author: sig})
	require.NoError(t, err)
	_, err = repo.CreateTag("v1", commit1, nil)
	require.NoError(t, err)

	// Only the first line changes at v2; l2-l6 are untouched, so this and the
	// user's hand-edit below conflict on the same single hunk instead of
	// merging cleanly into two independent, non-overlapping changes.
	writeFile("conflict.txt", "template-v2\nl2\nl3\nl4\nl5\nl6")
	require.NoError(t, wt.AddGlob("."))
	commit2, err := wt.Commit("v2", &git.CommitOptions{Author: sig})
	require.NoError(t, err)
	_, err = repo.CreateTag("v2", commit2, nil)
	require.NoError(t, err)

	return repoDir
}

// TestInitCmd_MaxChangesThreshold_EndToEnd mirrors cmd/scaffold's
// TestScaffoldGenerate_MaxChangesThreshold_EndToEnd: it drives the real CLI
// stack (RunE -> InitUI -> engine.Processor -> merge.ThreeWayMerger) through
// a real 3-way merge that produces a genuine conflict (base, ours, and
// theirs all disagree on the same line of conflict.txt), and asserts
// --max-changes wires all the way through to the merger's threshold check:
//   - with the default threshold (50), the conflict's 66% change size (see
//     buildInitConflictThresholdTemplateRepo) exceeds it and the update
//     fails with errUtils.ErrThreeWayMerge
//   - with --max-changes=100, the identical conflict is under the threshold
//     and the merge itself proceeds instead (still surfacing
//     errUtils.ErrMergeConflict, since the default conflict strategy is
//     manual, but this time having actually written conflict markers)
func TestInitCmd_MaxChangesThreshold_EndToEnd(t *testing.T) {
	requireGitBinaryForInitRenderedE2E(t)
	t.Cleanup(func() { viper.Reset() })

	repoDir := buildInitConflictThresholdTemplateRepo(t)
	src := "git::" + initRenderedE2EFileURI(repoDir)
	targetDir := t.TempDir()

	cmd1 := &cobra.Command{}
	initParser.RegisterFlags(cmd1)
	require.NoError(t, cmd1.Flags().Set("ref", "v1"))
	require.NoError(t, cmd1.Flags().Set("interactive", "false"))
	require.NoError(t, cmd1.Flags().Set("no-git", "true"))
	require.NoError(t, cmd1.Flags().Set("update-strategy", "rendered"))

	require.NoError(t, initCmd.RunE(cmd1, []string{src, targetDir}))

	conflictPath := filepath.Join(targetDir, "conflict.txt")
	assert.Equal(t, "template-v1\nl2\nl3\nl4\nl5\nl6", readInitRenderedFile(t, conflictPath))

	// Hand-edit the same line the template also changes at v2, so base (v1
	// render), ours (this hand-edit), and theirs (v2 render) all disagree on
	// that one line -- a real conflict, not a clean fast-forward.
	require.NoError(t, os.WriteFile(conflictPath, []byte("user-edit\nl2\nl3\nl4\nl5\nl6"), 0o644))

	// Default --max-changes (50): the conflict's 66% change size exceeds the
	// threshold, so the update must fail instead of silently applying.
	cmdDefault := &cobra.Command{}
	initParser.RegisterFlags(cmdDefault)
	require.NoError(t, cmdDefault.Flags().Set("ref", "v2"))
	require.NoError(t, cmdDefault.Flags().Set("interactive", "false"))
	require.NoError(t, cmdDefault.Flags().Set("no-git", "true"))
	require.NoError(t, cmdDefault.Flags().Set("update", "true"))
	require.NoError(t, cmdDefault.Flags().Set("update-strategy", "rendered"))

	err := initCmd.RunE(cmdDefault, []string{src, targetDir})
	require.Error(t, err)
	// engine.Processor wraps the merger's ErrMergeThresholdExceeded in a fresh
	// errUtils.ErrThreeWayMerge without preserving the underlying cause (no
	// WithCause on this wrap), so the threshold-specific sentinel and its
	// "Too many changes detected" explanation aren't reachable from here --
	// ErrThreeWayMerge is the strongest assertion available for this failure.
	assert.ErrorIs(t, err, errUtils.ErrThreeWayMerge)
	assert.Contains(t, fmt.Sprintf("%+v", err), "too extensive for automatic merging")

	// The failed merge must not have touched the target file.
	assert.Equal(t, "user-edit\nl2\nl3\nl4\nl5\nl6", readInitRenderedFile(t, conflictPath))

	// --max-changes=100: the identical conflict is now under threshold, so
	// the merge itself proceeds instead of being blocked by ErrThreeWayMerge.
	// It still surfaces an error -- the default (manual) conflict strategy
	// always returns errUtils.ErrMergeConflict so the user notices and
	// resolves the markers -- but this time the merge actually ran and wrote
	// the file, proving --max-changes raised the threshold rather than the
	// conflict simply being reported differently.
	cmdRaised := &cobra.Command{}
	initParser.RegisterFlags(cmdRaised)
	require.NoError(t, cmdRaised.Flags().Set("ref", "v2"))
	require.NoError(t, cmdRaised.Flags().Set("interactive", "false"))
	require.NoError(t, cmdRaised.Flags().Set("no-git", "true"))
	require.NoError(t, cmdRaised.Flags().Set("update", "true"))
	require.NoError(t, cmdRaised.Flags().Set("update-strategy", "rendered"))
	require.NoError(t, cmdRaised.Flags().Set("max-changes", "100"))

	err = initCmd.RunE(cmdRaised, []string{src, targetDir})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrMergeConflict)

	merged := readInitRenderedFile(t, conflictPath)
	assert.NotEqual(t, "user-edit\nl2\nl3\nl4\nl5\nl6", merged, "--max-changes=100 must let the merge write conflict markers instead of leaving the hand-edit untouched by a blocked update")
	assert.Contains(t, merged, "<<<<<<<", "the default manual conflict strategy must write real conflict markers")
	assert.Contains(t, merged, "l2\nl3\nl4\nl5\nl6", "the untouched tail of the file must survive the merge unchanged")
}
