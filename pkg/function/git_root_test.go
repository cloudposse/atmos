package function

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitRootFunction_Execute_InGitRepo(t *testing.T) {
	fn := NewGitRootFunction()

	// This test assumes we're running in a git repository.
	result, err := fn.Execute(context.Background(), "", nil)
	require.NoError(t, err)

	// Result should be a non-empty path.
	path, ok := result.(string)
	require.True(t, ok)
	assert.NotEmpty(t, path)

	// The path should exist.
	_, err = os.Stat(path)
	assert.NoError(t, err)

	// The path should contain a .git directory.
	gitDir := filepath.Join(path, ".git")
	_, err = os.Stat(gitDir)
	assert.NoError(t, err)
}

func TestNewGitRootFunction(t *testing.T) {
	fn := NewGitRootFunction()
	require.NotNil(t, fn)
	assert.Equal(t, TagRepoRoot, fn.Name())
	assert.Equal(t, PreMerge, fn.Phase())
	assert.Contains(t, fn.Aliases(), "git-root")
}

func TestGitRootFunction_Aliases(t *testing.T) {
	fn := NewGitRootFunction()
	aliases := fn.Aliases()
	assert.Len(t, aliases, 1)
	assert.Equal(t, "git-root", aliases[0])
}
