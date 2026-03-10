package github

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProvider_Detect(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected bool
	}{
		{
			name:     "detects when GITHUB_ACTIONS is true",
			envValue: "true",
			expected: true,
		},
		{
			name:     "does not detect when GITHUB_ACTIONS is false",
			envValue: "false",
			expected: false,
		},
		{
			name:     "does not detect when GITHUB_ACTIONS is empty",
			envValue: "",
			expected: false,
		},
		{
			name:     "does not detect when GITHUB_ACTIONS is 1",
			envValue: "1",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GITHUB_ACTIONS", tt.envValue)
			p := NewProvider()
			assert.Equal(t, tt.expected, p.Detect())
		})
	}
}

func TestProvider_Name(t *testing.T) {
	p := NewProvider()
	assert.Equal(t, "github-actions", p.Name())
}

func TestProvider_Context(t *testing.T) {
	t.Run("parses environment variables", func(t *testing.T) {
		t.Setenv("GITHUB_RUN_ID", "12345")
		t.Setenv("GITHUB_RUN_NUMBER", "42")
		t.Setenv("GITHUB_WORKFLOW", "CI")
		t.Setenv("GITHUB_JOB", "build")
		t.Setenv("GITHUB_ACTOR", "user")
		t.Setenv("GITHUB_EVENT_NAME", "push")
		t.Setenv("GITHUB_REF", "refs/heads/main")
		t.Setenv("GITHUB_SHA", "abc123")
		t.Setenv("GITHUB_REPOSITORY", "owner/repo")
		t.Setenv("GITHUB_REF_NAME", "main")
		t.Setenv("GITHUB_HEAD_REF", "")

		p := NewProvider()
		ctx, err := p.Context()
		require.NoError(t, err)
		assert.Equal(t, "github-actions", ctx.Provider)
		assert.Equal(t, "12345", ctx.RunID)
		assert.Equal(t, 42, ctx.RunNumber)
		assert.Equal(t, "owner", ctx.RepoOwner)
		assert.Equal(t, "repo", ctx.RepoName)
		assert.Equal(t, "main", ctx.Branch)
	})
}

func TestProvider_EnsureClient(t *testing.T) {
	t.Run("succeeds when GITHUB_TOKEN is set", func(t *testing.T) {
		t.Setenv("GITHUB_TOKEN", "test-token")
		p := NewProvider()
		err := p.ensureClient()
		require.NoError(t, err)
		assert.NotNil(t, p.client)
	})

	t.Run("succeeds when GH_TOKEN is set", func(t *testing.T) {
		// Clear GITHUB_TOKEN to ensure GH_TOKEN fallback works.
		t.Setenv("GITHUB_TOKEN", "")
		t.Setenv("GH_TOKEN", "test-token")
		p := NewProvider()
		err := p.ensureClient()
		require.NoError(t, err)
		assert.NotNil(t, p.client)
	})

	t.Run("fails when no token is available", func(t *testing.T) {
		t.Setenv("GITHUB_TOKEN", "")
		t.Setenv("GH_TOKEN", "")
		p := NewProvider()
		err := p.ensureClient()
		require.Error(t, err)
		assert.Nil(t, p.client)
	})

	t.Run("caches result across calls", func(t *testing.T) {
		t.Setenv("GITHUB_TOKEN", "test-token")
		p := NewProvider()

		err1 := p.ensureClient()
		require.NoError(t, err1)
		client1 := p.client

		err2 := p.ensureClient()
		require.NoError(t, err2)
		assert.Same(t, client1, p.client, "client should be the same instance")
	})

	t.Run("NewProviderWithClient skips lazy init", func(t *testing.T) {
		// Even without tokens, NewProviderWithClient should work.
		t.Setenv("GITHUB_TOKEN", "")
		t.Setenv("GH_TOKEN", "")

		client := &Client{}
		p := NewProviderWithClient(client)
		err := p.ensureClient()
		require.NoError(t, err)
		assert.Same(t, client, p.client)
	})
}

func TestProvider_DetectIndependentOfToken(t *testing.T) {
	// This is the key test: detection works without GITHUB_TOKEN.
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")

	p := NewProvider()
	assert.True(t, p.Detect(), "detection should succeed without token")
	assert.Equal(t, "github-actions", p.Name(), "name should be available without token")

	// Context should also work without token (it reads env vars, not API).
	ctx, err := p.Context()
	require.NoError(t, err)
	assert.Equal(t, "github-actions", ctx.Provider)

	// OutputWriter should work without token.
	_ = p.OutputWriter()

	// Only API methods should fail without token.
	err = p.ensureClient()
	require.Error(t, err, "ensureClient should fail without token")
}

func TestProvider_OutputWriter_IndependentOfToken(t *testing.T) {
	// OutputWriter should work without a GitHub API client.
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_OUTPUT", os.DevNull)
	t.Setenv("GITHUB_STEP_SUMMARY", os.DevNull)

	p := NewProvider()
	writer := p.OutputWriter()
	assert.NotNil(t, writer)
}
