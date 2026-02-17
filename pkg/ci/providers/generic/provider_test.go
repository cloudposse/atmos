package generic

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
)

func TestProvider(t *testing.T) {
	p := NewProvider()

	t.Run("Name returns generic", func(t *testing.T) {
		assert.Equal(t, ProviderName, p.Name())
	})

	t.Run("Detect always returns false", func(t *testing.T) {
		// Generic provider is never auto-detected.
		assert.False(t, p.Detect())
	})

	t.Run("Context returns empty context", func(t *testing.T) {
		ctx, err := p.Context()
		require.NoError(t, err)
		assert.Equal(t, ProviderName, ctx.Provider)
	})

	t.Run("Context uses environment variables", func(t *testing.T) {
		// Set environment variables.
		t.Setenv("ATMOS_CI_SHA", "abc123")
		t.Setenv("ATMOS_CI_BRANCH", "feature/test")
		t.Setenv("ATMOS_CI_REPOSITORY", "owner/repo")
		t.Setenv("ATMOS_CI_ACTOR", "testuser")

		// Create new provider to pick up env vars.
		p := NewProvider()
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

func TestOutputWriter(t *testing.T) {
	t.Run("WriteOutput to file", func(t *testing.T) {
		tmpDir := t.TempDir()
		outputFile := filepath.Join(tmpDir, "output")

		w := &OutputWriter{
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

		w := &OutputWriter{
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

		w := &OutputWriter{
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
		w := &OutputWriter{}

		// Should not error when no file is configured.
		err := w.WriteOutput("key", "value")
		assert.NoError(t, err)
	})

	t.Run("WriteSummary without file writes to stderr", func(t *testing.T) {
		w := &OutputWriter{}

		// Should not error when no file is configured.
		err := w.WriteSummary("test summary")
		assert.NoError(t, err)
	})
}

func TestCreateCheckRun(t *testing.T) {
	p := NewProvider()
	ctx := context.Background()

	t.Run("returns check run with correct fields", func(t *testing.T) {
		opts := &provider.CreateCheckRunOptions{
			Name:    "atmos/plan: plat-ue2-dev/vpc",
			Status:  provider.CheckRunStatePending,
			Title:   "Planning vpc",
			Summary: "Running terraform plan",
		}

		checkRun, err := p.CreateCheckRun(ctx, opts)
		require.NoError(t, err)
		assert.NotZero(t, checkRun.ID)
		assert.Equal(t, opts.Name, checkRun.Name)
		assert.Equal(t, opts.Status, checkRun.Status)
		assert.Equal(t, opts.Title, checkRun.Title)
		assert.Equal(t, opts.Summary, checkRun.Summary)
		assert.False(t, checkRun.StartedAt.IsZero())
	})

	t.Run("incrementing IDs", func(t *testing.T) {
		p := NewProvider()
		opts := &provider.CreateCheckRunOptions{
			Name:   "check-1",
			Status: provider.CheckRunStatePending,
		}

		first, err := p.CreateCheckRun(ctx, opts)
		require.NoError(t, err)

		opts.Name = "check-2"
		second, err := p.CreateCheckRun(ctx, opts)
		require.NoError(t, err)

		assert.NotEqual(t, first.ID, second.ID)
		assert.Equal(t, first.ID+1, second.ID)
	})
}

func TestUpdateCheckRun(t *testing.T) {
	p := NewProvider()
	ctx := context.Background()

	t.Run("success status", func(t *testing.T) {
		opts := &provider.UpdateCheckRunOptions{
			CheckRunID: 1,
			Name:       "atmos/plan: plat-ue2-dev/vpc",
			Status:     provider.CheckRunStateSuccess,
			Conclusion: "success",
			Title:      "Plan succeeded",
			Summary:    "No changes needed",
		}

		checkRun, err := p.UpdateCheckRun(ctx, opts)
		require.NoError(t, err)
		assert.Equal(t, int64(1), checkRun.ID)
		assert.Equal(t, opts.Name, checkRun.Name)
		assert.Equal(t, opts.Status, checkRun.Status)
		assert.Equal(t, opts.Conclusion, checkRun.Conclusion)
		assert.Equal(t, opts.Title, checkRun.Title)
		assert.Equal(t, opts.Summary, checkRun.Summary)
	})

	t.Run("failure status", func(t *testing.T) {
		opts := &provider.UpdateCheckRunOptions{
			CheckRunID: 2,
			Name:       "atmos/plan: plat-ue2-dev/vpc",
			Status:     provider.CheckRunStateFailure,
			Conclusion: "failure",
		}

		checkRun, err := p.UpdateCheckRun(ctx, opts)
		require.NoError(t, err)
		assert.Equal(t, provider.CheckRunStateFailure, checkRun.Status)
	})

	t.Run("error status", func(t *testing.T) {
		opts := &provider.UpdateCheckRunOptions{
			CheckRunID: 3,
			Name:       "atmos/plan: plat-ue2-dev/vpc",
			Status:     provider.CheckRunStateError,
		}

		checkRun, err := p.UpdateCheckRun(ctx, opts)
		require.NoError(t, err)
		assert.Equal(t, provider.CheckRunStateError, checkRun.Status)
	})

	t.Run("cancelled status", func(t *testing.T) {
		opts := &provider.UpdateCheckRunOptions{
			CheckRunID: 4,
			Name:       "atmos/plan: plat-ue2-dev/vpc",
			Status:     provider.CheckRunStateCancelled,
		}

		checkRun, err := p.UpdateCheckRun(ctx, opts)
		require.NoError(t, err)
		assert.Equal(t, provider.CheckRunStateCancelled, checkRun.Status)
	})

	t.Run("in progress status", func(t *testing.T) {
		opts := &provider.UpdateCheckRunOptions{
			CheckRunID: 5,
			Name:       "atmos/plan: plat-ue2-dev/vpc",
			Status:     provider.CheckRunStateInProgress,
		}

		checkRun, err := p.UpdateCheckRun(ctx, opts)
		require.NoError(t, err)
		assert.Equal(t, provider.CheckRunStateInProgress, checkRun.Status)
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
