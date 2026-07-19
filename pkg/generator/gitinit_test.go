package generator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitGitRepository_CreatesInitialCommit(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# demo\n"), 0o600))

	skipped, err := InitGitRepository(InitGitOptions{
		TargetPath:      dir,
		TemplateName:    "basic",
		TemplateVersion: "1.0.0",
	})
	require.NoError(t, err)
	assert.False(t, skipped)

	repo, err := git.PlainOpen(dir)
	require.NoError(t, err)
	head, err := repo.Head()
	require.NoError(t, err)
	commit, err := repo.CommitObject(head.Hash())
	require.NoError(t, err)
	assert.Equal(t, "Initial commit from atmos init (basic@1.0.0)", commit.Message)

	wt, err := repo.Worktree()
	require.NoError(t, err)
	status, err := wt.Status()
	require.NoError(t, err)
	assert.True(t, status.IsClean(), "generated repository should be clean after initial commit")
}

func TestInitGitRepository_SkipsInsideExistingRepo(t *testing.T) {
	root := t.TempDir()
	_, err := git.PlainInit(root, false)
	require.NoError(t, err)
	child := filepath.Join(root, "generated")
	require.NoError(t, os.MkdirAll(child, 0o755))

	skipped, err := InitGitRepository(InitGitOptions{TargetPath: child, TemplateName: "basic"})
	require.NoError(t, err)
	assert.True(t, skipped)
	_, statErr := os.Stat(filepath.Join(child, ".git"))
	assert.True(t, os.IsNotExist(statErr), "nested target should not get its own .git")
}

func TestInitialCommitMessage_NoVersion(t *testing.T) {
	assert.Equal(t, "Initial commit from atmos init (basic)", initialCommitMessage("basic", ""))
}
