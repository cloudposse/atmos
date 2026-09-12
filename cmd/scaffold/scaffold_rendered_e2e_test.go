package scaffold

import (
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
// for a user hand-edit that must survive).
func buildTwoTagTemplateRepo(t *testing.T) string {
	t.Helper()
	repoDir := t.TempDir()
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

	return repoDir
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
//     all -- the key behavioral difference from update-strategy=tracked
func TestScaffoldGenerate_UpdateStrategyRendered_EndToEnd(t *testing.T) {
	requireGitBinaryForRenderedE2E(t)
	t.Cleanup(func() { viper.Reset() })

	repoDir := buildTwoTagTemplateRepo(t)
	src := "git::" + renderedE2EFileURI(repoDir)
	targetDir := t.TempDir()

	cmd1 := &cobra.Command{}
	scaffoldGenerateParser.RegisterFlags(cmd1)
	require.NoError(t, cmd1.Flags().Set("ref", "v1"))
	require.NoError(t, cmd1.Flags().Set("interactive", "false"))

	require.NoError(t, scaffoldGenerateCmd.RunE(cmd1, []string{src, targetDir}))

	staticPath := filepath.Join(targetDir, "static.txt")
	updatePath := filepath.Join(targetDir, "update.txt")

	v1Update, err := os.ReadFile(updatePath)
	require.NoError(t, err)
	assert.Equal(t, "v1 content\n", string(v1Update))

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

	finalUpdate, err := os.ReadFile(updatePath)
	require.NoError(t, err)
	assert.Equal(t, "v2 content\n", string(finalUpdate), "the template's own v1->v2 change must be applied")

	_, gitStatErr = os.Stat(filepath.Join(targetDir, ".git"))
	assert.True(t, os.IsNotExist(gitStatErr), "rendered mode must never require the target to become a git repository")
}
