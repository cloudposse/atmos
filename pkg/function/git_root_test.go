package function

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewGitRootFunction(t *testing.T) {
	fn := NewGitRootFunction()

	assert.Equal(t, "repo-root", fn.Name())
	assert.Empty(t, fn.Aliases())
	assert.Equal(t, PreMerge, fn.Phase())
}

func TestGitRootFunctionWithTestRoot(t *testing.T) {
	fn := NewGitRootFunctionWithTestRoot("/test/path")

	result, err := fn.Execute(context.Background(), "", nil)
	require.NoError(t, err)
	assert.Equal(t, "/test/path", result)
}

func TestGitRootFunctionWithEnvOverride(t *testing.T) {
	// Set the test environment variable.
	t.Setenv("TEST_GIT_ROOT", "/env/override/path")

	fn := NewGitRootFunction()

	result, err := fn.Execute(context.Background(), "", nil)
	require.NoError(t, err)
	assert.Equal(t, "/env/override/path", result)
}

func TestGitRootFunctionInGitRepo(t *testing.T) {
	// This test requires being run within a git repository.
	// It should find the actual repo root.
	fn := NewGitRootFunction()

	result, err := fn.Execute(context.Background(), "fallback", nil)
	require.NoError(t, err)

	// The result should be an absolute path.
	resultStr, ok := result.(string)
	require.True(t, ok)
	assert.True(t, filepath.IsAbs(resultStr), "expected absolute path, got %s", resultStr)

	// Verify it contains a .git directory.
	gitPath := filepath.Join(resultStr, ".git")
	_, err = os.Stat(gitPath)
	assert.NoError(t, err, "expected .git directory at %s", gitPath)
}

func TestGitRootFunctionDefaultValue(t *testing.T) {
	// Create a temporary directory outside any git repo.
	tmpDir := t.TempDir()

	fn := NewGitRootFunction()

	// Use execution context to set working directory.
	execCtx := &ExecutionContext{
		WorkingDir: tmpDir,
	}

	// With a default value, should return that when not in a git repo.
	result, err := fn.Execute(context.Background(), "fallback_value", execCtx)
	require.NoError(t, err)
	assert.Equal(t, "fallback_value", result)
}

func TestGitRootFunctionNoDefaultNoRepo(t *testing.T) {
	// Create a temporary directory outside any git repo.
	tmpDir := t.TempDir()

	fn := NewGitRootFunction()

	// Use execution context to set working directory.
	execCtx := &ExecutionContext{
		WorkingDir: tmpDir,
	}

	// Without a default value, should return an error.
	_, err := fn.Execute(context.Background(), "", execCtx)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrExecutionFailed)
}

func TestGitRootFunctionWithContext(t *testing.T) {
	// Test that working directory from context is used.
	fn := NewGitRootFunctionWithTestRoot("/context/path")

	execCtx := &ExecutionContext{
		WorkingDir: "/some/other/path",
	}

	// With test root set, it should still return the test root.
	result, err := fn.Execute(context.Background(), "", execCtx)
	require.NoError(t, err)
	assert.Equal(t, "/context/path", result)
}
