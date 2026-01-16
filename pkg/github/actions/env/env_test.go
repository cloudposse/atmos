package env

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsGitHubActions(t *testing.T) {
	t.Run("returns false when not in CI", func(t *testing.T) {
		// Unset by setting to empty value via t.Setenv.
		t.Setenv("GITHUB_ACTIONS", "")
		os.Unsetenv("GITHUB_ACTIONS")
		assert.False(t, IsGitHubActions())
	})

	t.Run("returns true when GITHUB_ACTIONS=true", func(t *testing.T) {
		t.Setenv("GITHUB_ACTIONS", "true")
		assert.True(t, IsGitHubActions())
	})

	t.Run("returns false when GITHUB_ACTIONS has other value", func(t *testing.T) {
		t.Setenv("GITHUB_ACTIONS", "false")
		assert.False(t, IsGitHubActions())
	})
}

func TestGetOutputPath(t *testing.T) {
	t.Run("returns empty when not set", func(t *testing.T) {
		t.Setenv("GITHUB_OUTPUT", "")
		os.Unsetenv("GITHUB_OUTPUT")
		assert.Empty(t, GetOutputPath())
	})

	t.Run("returns path when set", func(t *testing.T) {
		t.Setenv("GITHUB_OUTPUT", "/tmp/github_output")
		assert.Equal(t, "/tmp/github_output", GetOutputPath())
	})
}

func TestGetEnvPath(t *testing.T) {
	t.Run("returns empty when not set", func(t *testing.T) {
		t.Setenv("GITHUB_ENV", "")
		os.Unsetenv("GITHUB_ENV")
		assert.Empty(t, GetEnvPath())
	})

	t.Run("returns path when set", func(t *testing.T) {
		t.Setenv("GITHUB_ENV", "/tmp/github_env")
		assert.Equal(t, "/tmp/github_env", GetEnvPath())
	})
}

func TestGetPathPath(t *testing.T) {
	t.Run("returns empty when not set", func(t *testing.T) {
		t.Setenv("GITHUB_PATH", "")
		os.Unsetenv("GITHUB_PATH")
		assert.Empty(t, GetPathPath())
	})

	t.Run("returns path when set", func(t *testing.T) {
		t.Setenv("GITHUB_PATH", "/tmp/github_path")
		assert.Equal(t, "/tmp/github_path", GetPathPath())
	})
}

func TestGetSummaryPath(t *testing.T) {
	t.Run("returns empty when not set", func(t *testing.T) {
		t.Setenv("GITHUB_STEP_SUMMARY", "")
		os.Unsetenv("GITHUB_STEP_SUMMARY")
		assert.Empty(t, GetSummaryPath())
	})

	t.Run("returns path when set", func(t *testing.T) {
		t.Setenv("GITHUB_STEP_SUMMARY", "/tmp/github_step_summary")
		assert.Equal(t, "/tmp/github_step_summary", GetSummaryPath())
	})
}
