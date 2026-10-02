package scaffold

import (
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
	"github.com/cloudposse/atmos/pkg/project/config"
)

const renderedE2EScaffoldYAML = `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: rendered-e2e
spec:
  fields:
    - name: project_name
      type: input
      default: demo
`

// requireGitBinaryForRenderedE2E skips the test when no git binary is on
// PATH: go-getter's git:: fetch (used by source.Resolve/Hydrate) shells out
// to a real git binary, independent of how the fixture repo below is built.
func requireGitBinaryForRenderedE2E(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not found on PATH; required by go-getter's git clone")
	}
}

// readRenderedFile reads a file rendered from the fixture git repo and
// normalizes CRLF to LF: on Windows runners, Git's core.autocrlf=true
// rewrites the fixture's LF line endings to CRLF on checkout, which is a
// checkout-environment detail unrelated to what these tests assert about
// rendered/merge behavior.
func readRenderedFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	return strings.ReplaceAll(string(content), "\r\n", "\n")
}

func renderedE2EFileURI(path string) string {
	cleaned := filepath.ToSlash(filepath.Clean(path))
	if filepath.VolumeName(path) != "" && cleaned != "" && cleaned[0] != '/' {
		cleaned = "/" + cleaned
	}
	return (&url.URL{Scheme: "file", Path: cleaned}).String()
}

// buildTwoTagTemplateRepo creates a local git repo (via go-git, no external
// git binary needed for fixture construction) with two tagged versions of a
// scaffold template: v1 and v2, where the template changes update.txt
// between the two tags but never touches static.txt -- so a real --update
// run has one file the template changed and one it never touches (a stand-in
// for a user hand-edit that must survive). Also returns both tags' commit
// hashes so callers can assert that rendered-mode records the immutable
// resolved SHA for whichever tag was actually generated against, rather than
// the mutable "v1"/"v2" tag names.
func buildTwoTagTemplateRepo(t *testing.T) (repoDir, v1Commit, v2Commit string) {
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

	writeFile("scaffold.yaml", renderedE2EScaffoldYAML)
	writeFile("static.txt", "static content\n")
	writeFile("update.txt", "v1 content\n")
	require.NoError(t, wt.AddGlob("."))
	commit1, err := wt.Commit("v1", &git.CommitOptions{Author: sig})
	require.NoError(t, err)
	_, err = repo.CreateTag("v1", commit1, nil)
	require.NoError(t, err)

	writeFile("update.txt", "v2 content\n")
	require.NoError(t, wt.AddGlob("."))
	commit2, err := wt.Commit("v2", &git.CommitOptions{Author: sig})
	require.NoError(t, err)
	_, err = repo.CreateTag("v2", commit2, nil)
	require.NoError(t, err)

	return repoDir, commit1.String(), commit2.String()
}

// TestScaffoldGenerate_UpdateStrategyRendered_EndToEnd drives the real CLI
// stack (RunE -> ScaffoldUI -> engine.Processor -> merge.ThreeWayMerger)
// through a full generate-at-v1, hand-edit, update-to-v2 cycle using
// --update-strategy=rendered, and asserts:
//   - the hand-edited file survives the update untouched (base provenance
//     never involves the target's own git history, so there's nothing for a
//     git-history read to silently overwrite it with)
//   - the template's own v1->v2 change is applied to the file it changed
//   - none of this requires the target directory to be a git repository at
//     all -- the key behavioral difference from update-strategy=tracked.
func TestScaffoldGenerate_UpdateStrategyRendered_EndToEnd(t *testing.T) {
	requireGitBinaryForRenderedE2E(t)
	t.Cleanup(func() { viper.Reset() })

	repoDir, v1Commit, v2Commit := buildTwoTagTemplateRepo(t)
	src := "git::" + renderedE2EFileURI(repoDir)
	targetDir := t.TempDir()

	cmd1 := &cobra.Command{}
	scaffoldGenerateParser.RegisterFlags(cmd1)
	require.NoError(t, cmd1.Flags().Set("ref", "v1"))
	require.NoError(t, cmd1.Flags().Set("interactive", "false"))
	// Establishes this project as rendered-strategy from the first generation:
	// a later --update-strategy=rendered run needs a resolved commit SHA
	// recorded by *some* prior generation, and that capture is keyed on the
	// strategy active at generation time, not retrofitted once an update asks
	// for it.
	require.NoError(t, cmd1.Flags().Set("update-strategy", "rendered"))

	require.NoError(t, scaffoldGenerateCmd.RunE(cmd1, []string{src, targetDir}))

	staticPath := filepath.Join(targetDir, "static.txt")
	updatePath := filepath.Join(targetDir, "update.txt")

	assert.Equal(t, "v1 content\n", readRenderedFile(t, updatePath))

	firstRecord, err := config.LoadProjectRecord(targetDir)
	require.NoError(t, err)
	require.NotNil(t, firstRecord)
	assert.Equal(t, v1Commit, firstRecord.Spec.RenderedRef, "the initial rendered-strategy generation must record the resolved v1 commit SHA, not the mutable v1 tag")

	_, gitStatErr := os.Stat(filepath.Join(targetDir, ".git"))
	require.True(t, os.IsNotExist(gitStatErr), "the target must not be a git repository for this test to prove anything")

	// Simulate a hand-edit to the file the template never touches again.
	require.NoError(t, os.WriteFile(staticPath, []byte("hand-edited content\n"), 0o644))

	cmd2 := &cobra.Command{}
	scaffoldGenerateParser.RegisterFlags(cmd2)
	require.NoError(t, cmd2.Flags().Set("ref", "v2"))
	require.NoError(t, cmd2.Flags().Set("interactive", "false"))
	require.NoError(t, cmd2.Flags().Set("update", "true"))
	require.NoError(t, cmd2.Flags().Set("update-strategy", "rendered"))

	require.NoError(t, scaffoldGenerateCmd.RunE(cmd2, []string{src, targetDir}))

	finalStatic, err := os.ReadFile(staticPath)
	require.NoError(t, err)
	assert.Equal(t, "hand-edited content\n", string(finalStatic), "the hand-edit must survive the rendered-mode update")

	assert.Equal(t, "v2 content\n", readRenderedFile(t, updatePath), "the template's own v1->v2 change must be applied")

	_, gitStatErr = os.Stat(filepath.Join(targetDir, ".git"))
	assert.True(t, os.IsNotExist(gitStatErr), "rendered mode must never require the target to become a git repository")

	// Regression check for the original silent-data-loss bug: an unconditional
	// defaultBaseRef resolution used to run for any --update regardless of
	// strategy, writing a resolved value (e.g. "HEAD") into spec.baseRef even
	// under rendered mode. Rendered must leave spec.baseRef untouched and
	// record its own provenance under spec.renderedRef instead.
	record, err := config.LoadProjectRecord(targetDir)
	require.NoError(t, err)
	require.NotNil(t, record)
	assert.Empty(t, record.Spec.BaseRef, "rendered-mode updates must never populate spec.baseRef")
	assert.Equal(t, v2Commit, record.Spec.RenderedRef, "rendered-mode updates must record the resolved v2 commit SHA, not the mutable v2 tag")
}

// TestScaffoldGenerate_UpdateStrategyRendered_DryRun_EndToEnd drives a real
// --update --dry-run --update-strategy=rendered preview and proves it
// succeeds without writing anything to the target directory. Regression test
// for a bug where the rendered-mode merge base's own internal render shared
// ui.processor's DryRun flag with the outer run: engine.Processor.ProcessFile
// skips real writes whenever DryRun is set, so the pristine "old ref" render
// into its own temp directory silently produced no files whenever the outer
// run was itself a --dry-run preview -- leaving SetupRenderedBaseStorage's
// merge base empty and the dry-run 3-way merge with nothing to diff against
// (surfacing as a merge failure instead of a clean preview).
func TestScaffoldGenerate_UpdateStrategyRendered_DryRun_EndToEnd(t *testing.T) {
	requireGitBinaryForRenderedE2E(t)
	t.Cleanup(func() { viper.Reset() })

	repoDir, v1Commit, _ := buildTwoTagTemplateRepo(t)
	src := "git::" + renderedE2EFileURI(repoDir)
	targetDir := t.TempDir()

	cmd1 := &cobra.Command{}
	scaffoldGenerateParser.RegisterFlags(cmd1)
	require.NoError(t, cmd1.Flags().Set("ref", "v1"))
	require.NoError(t, cmd1.Flags().Set("interactive", "false"))
	require.NoError(t, cmd1.Flags().Set("update-strategy", "rendered"))

	require.NoError(t, scaffoldGenerateCmd.RunE(cmd1, []string{src, targetDir}))

	staticPath := filepath.Join(targetDir, "static.txt")
	updatePath := filepath.Join(targetDir, "update.txt")

	// Simulate a hand-edit, mirroring the non-dry-run test above.
	require.NoError(t, os.WriteFile(staticPath, []byte("hand-edited content\n"), 0o644))

	cmd2 := &cobra.Command{}
	scaffoldGenerateParser.RegisterFlags(cmd2)
	require.NoError(t, cmd2.Flags().Set("ref", "v2"))
	require.NoError(t, cmd2.Flags().Set("interactive", "false"))
	require.NoError(t, cmd2.Flags().Set("update", "true"))
	require.NoError(t, cmd2.Flags().Set("dry-run", "true"))
	require.NoError(t, cmd2.Flags().Set("update-strategy", "rendered"))

	require.NoError(t, scaffoldGenerateCmd.RunE(cmd2, []string{src, targetDir}),
		"a --dry-run rendered-mode update preview must succeed even though its internal merge-base render must write to its own temp dir despite the outer dry-run")

	// --dry-run must never touch the target directory.
	finalStatic, err := os.ReadFile(staticPath)
	require.NoError(t, err)
	assert.Equal(t, "hand-edited content\n", string(finalStatic), "--dry-run must never write to the target directory")

	assert.Equal(t, "v1 content\n", readRenderedFile(t, updatePath), "--dry-run must never write to the target directory")

	// The dry-run preview must never persist a new project record either.
	record, err := config.LoadProjectRecord(targetDir)
	require.NoError(t, err)
	require.NotNil(t, record)
	assert.Equal(t, v1Commit, record.Spec.RenderedRef, "--dry-run must not overwrite the recorded provenance from the initial (non-dry-run) generation")
}

// buildRemovalTemplateRepo creates a local git repo with two tagged versions
// of a scaffold template, mirroring buildTwoTagTemplateRepo, but where v2
// stops generating removed.txt entirely (rather than changing its content).
// Static.txt is present at both tags and never changes, a stand-in for a
// file the template leaves alone so an unrelated file's behavior in the same
// update can be asserted unaffected.
func buildRemovalTemplateRepo(t *testing.T) (repoDir string) {
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

	writeFile("scaffold.yaml", renderedE2EScaffoldYAML)
	writeFile("static.txt", "static content\n")
	writeFile("removed.txt", "removed content\n")
	require.NoError(t, wt.AddGlob("."))
	commit1, err := wt.Commit("v1", &git.CommitOptions{Author: sig})
	require.NoError(t, err)
	_, err = repo.CreateTag("v1", commit1, nil)
	require.NoError(t, err)

	require.NoError(t, os.Remove(filepath.Join(repoDir, "removed.txt")))
	require.NoError(t, wt.AddGlob("."))
	commit2, err := wt.Commit("v2", &git.CommitOptions{Author: sig})
	require.NoError(t, err)
	_, err = repo.CreateTag("v2", commit2, nil)
	require.NoError(t, err)

	return repoDir
}

// TestScaffoldGenerate_UpdateStrategyRendered_DeletesRemovedFile_EndToEnd
// drives a real generate-at-v1, update-to-v2 cycle where v2 stops generating
// removed.txt, and asserts the real --update run deletes it: before this
// fix, a file the template stopped generating survived forever, silently.
func TestScaffoldGenerate_UpdateStrategyRendered_DeletesRemovedFile_EndToEnd(t *testing.T) {
	requireGitBinaryForRenderedE2E(t)
	t.Cleanup(func() { viper.Reset() })

	repoDir := buildRemovalTemplateRepo(t)
	src := "git::" + renderedE2EFileURI(repoDir)
	targetDir := t.TempDir()

	cmd1 := &cobra.Command{}
	scaffoldGenerateParser.RegisterFlags(cmd1)
	require.NoError(t, cmd1.Flags().Set("ref", "v1"))
	require.NoError(t, cmd1.Flags().Set("interactive", "false"))
	require.NoError(t, cmd1.Flags().Set("update-strategy", "rendered"))
	require.NoError(t, scaffoldGenerateCmd.RunE(cmd1, []string{src, targetDir}))

	removedPath := filepath.Join(targetDir, "removed.txt")
	staticPath := filepath.Join(targetDir, "static.txt")
	require.FileExists(t, removedPath)

	cmd2 := &cobra.Command{}
	scaffoldGenerateParser.RegisterFlags(cmd2)
	require.NoError(t, cmd2.Flags().Set("ref", "v2"))
	require.NoError(t, cmd2.Flags().Set("interactive", "false"))
	require.NoError(t, cmd2.Flags().Set("update", "true"))
	require.NoError(t, cmd2.Flags().Set("update-strategy", "rendered"))
	require.NoError(t, scaffoldGenerateCmd.RunE(cmd2, []string{src, targetDir}))

	_, statErr := os.Stat(removedPath)
	assert.True(t, os.IsNotExist(statErr), "a file the template stopped generating must be deleted on a clean --update")
	assert.Equal(t, "static content\n", readRenderedFile(t, staticPath), "an unrelated file must be unaffected by the deletion pass")
}

// TestScaffoldGenerate_UpdateStrategyRendered_ConflictsOnLocallyEditedRemovedFile_EndToEnd
// mirrors the test above, but the user hand-edits removed.txt before v2 stops
// generating it: the update must leave the hand-edit untouched and fail with
// an unresolved merge conflict instead of silently deleting or silently
// keeping it.
func TestScaffoldGenerate_UpdateStrategyRendered_ConflictsOnLocallyEditedRemovedFile_EndToEnd(t *testing.T) {
	requireGitBinaryForRenderedE2E(t)
	t.Cleanup(func() { viper.Reset() })

	repoDir := buildRemovalTemplateRepo(t)
	src := "git::" + renderedE2EFileURI(repoDir)
	targetDir := t.TempDir()

	cmd1 := &cobra.Command{}
	scaffoldGenerateParser.RegisterFlags(cmd1)
	require.NoError(t, cmd1.Flags().Set("ref", "v1"))
	require.NoError(t, cmd1.Flags().Set("interactive", "false"))
	require.NoError(t, cmd1.Flags().Set("update-strategy", "rendered"))
	require.NoError(t, scaffoldGenerateCmd.RunE(cmd1, []string{src, targetDir}))

	removedPath := filepath.Join(targetDir, "removed.txt")
	require.NoError(t, os.WriteFile(removedPath, []byte("hand-edited content\n"), 0o644))

	cmd2 := &cobra.Command{}
	scaffoldGenerateParser.RegisterFlags(cmd2)
	require.NoError(t, cmd2.Flags().Set("ref", "v2"))
	require.NoError(t, cmd2.Flags().Set("interactive", "false"))
	require.NoError(t, cmd2.Flags().Set("update", "true"))
	require.NoError(t, cmd2.Flags().Set("update-strategy", "rendered"))
	err := scaffoldGenerateCmd.RunE(cmd2, []string{src, targetDir})

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrMergeConflict)
	finalContent, readErr := os.ReadFile(removedPath)
	require.NoError(t, readErr)
	assert.Equal(t, "hand-edited content\n", string(finalContent), "a locally-edited file the template removed must survive the conflict untouched")
}

// TestScaffoldGenerate_UpdateStrategyRendered_RespectsUserDeletedFile_EndToEnd
// drives a real generate-at-v1, update-to-v2 cycle where the user deletes a
// file the template still generates at v2 (update.txt, changed but never
// removed between tags): the update must not recreate it, matching copier's
// documented handling of user-deleted paths.
func TestScaffoldGenerate_UpdateStrategyRendered_RespectsUserDeletedFile_EndToEnd(t *testing.T) {
	requireGitBinaryForRenderedE2E(t)
	t.Cleanup(func() { viper.Reset() })

	repoDir, _, _ := buildTwoTagTemplateRepo(t)
	src := "git::" + renderedE2EFileURI(repoDir)
	targetDir := t.TempDir()

	cmd1 := &cobra.Command{}
	scaffoldGenerateParser.RegisterFlags(cmd1)
	require.NoError(t, cmd1.Flags().Set("ref", "v1"))
	require.NoError(t, cmd1.Flags().Set("interactive", "false"))
	require.NoError(t, cmd1.Flags().Set("update-strategy", "rendered"))
	require.NoError(t, scaffoldGenerateCmd.RunE(cmd1, []string{src, targetDir}))

	updatePath := filepath.Join(targetDir, "update.txt")
	require.FileExists(t, updatePath)
	require.NoError(t, os.Remove(updatePath))

	cmd2 := &cobra.Command{}
	scaffoldGenerateParser.RegisterFlags(cmd2)
	require.NoError(t, cmd2.Flags().Set("ref", "v2"))
	require.NoError(t, cmd2.Flags().Set("interactive", "false"))
	require.NoError(t, cmd2.Flags().Set("update", "true"))
	require.NoError(t, cmd2.Flags().Set("update-strategy", "rendered"))
	require.NoError(t, scaffoldGenerateCmd.RunE(cmd2, []string{src, targetDir}))

	_, statErr := os.Stat(updatePath)
	assert.True(t, os.IsNotExist(statErr), "a file the user deleted must not be recreated by --update")
	assert.Equal(t, "static content\n", readRenderedFile(t, filepath.Join(targetDir, "static.txt")), "an unrelated file must be unaffected")
}

// TestScaffoldGenerate_UpdateStrategyRendered_RecreateDeletedOptsBackIn_EndToEnd
// mirrors the test above, but with --recreate-deleted set: the deleted file
// must be recreated with the new ref's content instead of staying deleted.
func TestScaffoldGenerate_UpdateStrategyRendered_RecreateDeletedOptsBackIn_EndToEnd(t *testing.T) {
	requireGitBinaryForRenderedE2E(t)
	t.Cleanup(func() { viper.Reset() })

	repoDir, _, _ := buildTwoTagTemplateRepo(t)
	src := "git::" + renderedE2EFileURI(repoDir)
	targetDir := t.TempDir()

	cmd1 := &cobra.Command{}
	scaffoldGenerateParser.RegisterFlags(cmd1)
	require.NoError(t, cmd1.Flags().Set("ref", "v1"))
	require.NoError(t, cmd1.Flags().Set("interactive", "false"))
	require.NoError(t, cmd1.Flags().Set("update-strategy", "rendered"))
	require.NoError(t, scaffoldGenerateCmd.RunE(cmd1, []string{src, targetDir}))

	updatePath := filepath.Join(targetDir, "update.txt")
	require.FileExists(t, updatePath)
	require.NoError(t, os.Remove(updatePath))

	cmd2 := &cobra.Command{}
	scaffoldGenerateParser.RegisterFlags(cmd2)
	require.NoError(t, cmd2.Flags().Set("ref", "v2"))
	require.NoError(t, cmd2.Flags().Set("interactive", "false"))
	require.NoError(t, cmd2.Flags().Set("update", "true"))
	require.NoError(t, cmd2.Flags().Set("update-strategy", "rendered"))
	require.NoError(t, cmd2.Flags().Set("recreate-deleted", "true"))
	require.NoError(t, scaffoldGenerateCmd.RunE(cmd2, []string{src, targetDir}))

	assert.Equal(t, "v2 content\n", readRenderedFile(t, updatePath), "--recreate-deleted must recreate a file the user deleted")
}
