package ci

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenericProvider(t *testing.T) {
	p := NewGenericProvider()

	t.Run("Name returns generic", func(t *testing.T) {
		assert.Equal(t, GenericProviderName, p.Name())
	})

	t.Run("Detect always returns false", func(t *testing.T) {
		// Generic provider is never auto-detected.
		assert.False(t, p.Detect())
	})

	t.Run("Context returns empty context", func(t *testing.T) {
		ctx, err := p.Context()
		require.NoError(t, err)
		assert.Equal(t, GenericProviderName, ctx.Provider)
	})

	t.Run("Context uses environment variables", func(t *testing.T) {
		// Set environment variables.
		t.Setenv("ATMOS_CI_SHA", "abc123")
		t.Setenv("ATMOS_CI_BRANCH", "feature/test")
		t.Setenv("ATMOS_CI_REPOSITORY", "owner/repo")
		t.Setenv("ATMOS_CI_ACTOR", "testuser")

		// Create new provider to pick up env vars.
		p := NewGenericProvider()
		ctx, err := p.Context()
		require.NoError(t, err)

		assert.Equal(t, "abc123", ctx.SHA)
		assert.Equal(t, "feature/test", ctx.Branch)
		assert.Equal(t, "owner/repo", ctx.Repository)
		assert.Equal(t, "owner", ctx.RepoOwner)
		assert.Equal(t, "repo", ctx.RepoName)
		assert.Equal(t, "testuser", ctx.Actor)
	})

	t.Run("OutputWriter returns writer", func(t *testing.T) {
		w := p.OutputWriter()
		assert.NotNil(t, w)
	})
}

func TestGenericOutputWriter(t *testing.T) {
	t.Run("WriteOutput to file", func(t *testing.T) {
		tmpDir := t.TempDir()
		outputFile := filepath.Join(tmpDir, "output")

		w := &GenericOutputWriter{
			outputFile: outputFile,
		}

		err := w.WriteOutput("key1", "value1")
		require.NoError(t, err)

		err = w.WriteOutput("key2", "value2")
		require.NoError(t, err)

		content, err := os.ReadFile(outputFile)
		require.NoError(t, err)
		assert.Contains(t, string(content), "key1=value1")
		assert.Contains(t, string(content), "key2=value2")
	})

	t.Run("WriteOutput multiline value uses heredoc", func(t *testing.T) {
		tmpDir := t.TempDir()
		outputFile := filepath.Join(tmpDir, "output")

		w := &GenericOutputWriter{
			outputFile: outputFile,
		}

		err := w.WriteOutput("multiline", "line1\nline2\nline3")
		require.NoError(t, err)

		content, err := os.ReadFile(outputFile)
		require.NoError(t, err)
		assert.Contains(t, string(content), "multiline<<EOF")
		assert.Contains(t, string(content), "line1\nline2\nline3")
		assert.Contains(t, string(content), "EOF")
	})

	t.Run("WriteSummary to file", func(t *testing.T) {
		tmpDir := t.TempDir()
		summaryFile := filepath.Join(tmpDir, "summary")

		w := &GenericOutputWriter{
			summaryFile: summaryFile,
		}

		err := w.WriteSummary("# Summary\n\nTest content")
		require.NoError(t, err)

		content, err := os.ReadFile(summaryFile)
		require.NoError(t, err)
		assert.Contains(t, string(content), "# Summary")
		assert.Contains(t, string(content), "Test content")
	})

	t.Run("WriteOutput without file logs debug", func(t *testing.T) {
		w := &GenericOutputWriter{}

		// Should not error when no file is configured.
		err := w.WriteOutput("key", "value")
		assert.NoError(t, err)
	})

	t.Run("WriteSummary without file writes to stderr", func(t *testing.T) {
		w := &GenericOutputWriter{}

		// Should not error when no file is configured.
		err := w.WriteSummary("test summary")
		assert.NoError(t, err)
	})
}

func TestGetFirstEnv(t *testing.T) {
	t.Run("returns first set variable", func(t *testing.T) {
		t.Setenv("TEST_VAR_1", "")
		t.Setenv("TEST_VAR_2", "second")
		t.Setenv("TEST_VAR_3", "third")

		result := getFirstEnv("TEST_VAR_1", "TEST_VAR_2", "TEST_VAR_3")
		assert.Equal(t, "second", result)
	})

	t.Run("returns empty if none set", func(t *testing.T) {
		result := getFirstEnv("NONEXISTENT_VAR_1", "NONEXISTENT_VAR_2")
		assert.Empty(t, result)
	})
}
