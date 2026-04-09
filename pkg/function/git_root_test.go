package function

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitRootFunction_Execute_InGitRepo(t *testing.T) {
	// Skip if not in a git repository.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	if err := cmd.Run(); err != nil {
		t.Skip("not running inside a git repository")
	}

	fn := NewGitRootFunction()

	result, err := fn.Execute(context.Background(), "", nil)
	require.NoError(t, err)

	// Result should be a non-empty path.
	path, ok := result.(string)
	require.True(t, ok)
	assert.NotEmpty(t, path)

	// The path should exist.
	_, err = os.Stat(path)
	assert.NoError(t, err)

	// The path should contain a .git directory or file.
	gitDir := filepath.Join(path, ".git")
	_, err = os.Stat(gitDir)
	assert.NoError(t, err)
}

func TestGitRootFunction_Metadata(t *testing.T) {
	fn := NewGitRootFunction()
	require.NotNil(t, fn)
	assert.Equal(t, TagRepoRoot, fn.Name())
	assert.Equal(t, PreMerge, fn.Phase())

	aliases := fn.Aliases()
	require.Len(t, aliases, 1)
	assert.Equal(t, "git-root", aliases[0])
}
