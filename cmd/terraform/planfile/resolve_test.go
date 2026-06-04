package planfile

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestResolveContext(t *testing.T) {
	t.Run("all flag returns empty SHA", func(t *testing.T) {
		resolved, err := resolveContext(true)
		require.NoError(t, err)
		assert.Empty(t, resolved.SHA)
	})

	t.Run("env var ATMOS_CI_SHA takes precedence", func(t *testing.T) {
		t.Setenv("ATMOS_CI_SHA", "env-sha-123")
		t.Setenv("GIT_COMMIT", "git-sha-456")

		resolved, err := resolveContext(false)
		require.NoError(t, err)
		assert.Equal(t, "env-sha-123", resolved.SHA)
	})

	t.Run("GIT_COMMIT fallback", func(t *testing.T) {
		t.Setenv("ATMOS_CI_SHA", "")
		t.Setenv("GIT_COMMIT", "git-sha-456")
		t.Setenv("CI_COMMIT_SHA", "")
		t.Setenv("COMMIT_SHA", "")

		resolved, err := resolveContext(false)
		require.NoError(t, err)
		assert.Equal(t, "git-sha-456", resolved.SHA)
	})

	t.Run("CI_COMMIT_SHA fallback", func(t *testing.T) {
		t.Setenv("ATMOS_CI_SHA", "")
		t.Setenv("GIT_COMMIT", "")
		t.Setenv("CI_COMMIT_SHA", "ci-sha-789")
		t.Setenv("COMMIT_SHA", "")

		resolved, err := resolveContext(false)
		require.NoError(t, err)
		assert.Equal(t, "ci-sha-789", resolved.SHA)
	})

	t.Run("falls back to git HEAD when no env vars set", func(t *testing.T) {
		// Clear all SHA env vars.
		t.Setenv("ATMOS_CI_SHA", "")
		t.Setenv("GIT_COMMIT", "")
		t.Setenv("CI_COMMIT_SHA", "")
		t.Setenv("COMMIT_SHA", "")

		// Should succeed if we're in a git repo, which we are during tests.
		resolved, err := resolveContext(false)
		require.NoError(t, err)
		assert.NotEmpty(t, resolved.SHA, "should resolve SHA from git HEAD")
	})

	t.Run("branch from env var", func(t *testing.T) {
		t.Setenv("ATMOS_CI_SHA", "sha123")
		t.Setenv("ATMOS_CI_BRANCH", "feature/test")

		resolved, err := resolveContext(false)
		require.NoError(t, err)
		assert.Equal(t, "feature/test", resolved.Branch)
	})
}

func TestResolveKey(t *testing.T) {
	t.Run("generates key from component stack and sha", func(t *testing.T) {
		key, err := resolveKey("my-component", "my-stack", "abc123")
		require.NoError(t, err)
		assert.Equal(t, "my-stack/my-component/abc123.tfplan.tar", key)
	})

	t.Run("missing stack returns error", func(t *testing.T) {
		_, err := resolveKey("component", "", "sha")
		assert.Error(t, err)
		assert.True(t, errors.Is(err, errUtils.ErrPlanfileKeyInvalid))
	})

	t.Run("missing component returns error", func(t *testing.T) {
		_, err := resolveKey("", "stack", "sha")
		assert.Error(t, err)
		assert.True(t, errors.Is(err, errUtils.ErrPlanfileKeyInvalid))
	})

	t.Run("missing sha returns error", func(t *testing.T) {
		_, err := resolveKey("component", "stack", "")
		assert.Error(t, err)
		assert.True(t, errors.Is(err, errUtils.ErrPlanfileKeyInvalid))
	})
}

func TestBuildQuery(t *testing.T) {
	t.Run("all empty returns All query", func(t *testing.T) {
		q := buildQuery("", "", "")
		assert.True(t, q.All)
		assert.Empty(t, q.Components)
		assert.Empty(t, q.Stacks)
		assert.Empty(t, q.SHAs)
	})

	t.Run("component only", func(t *testing.T) {
		q := buildQuery("vpc", "", "")
		assert.False(t, q.All)
		assert.Equal(t, []string{"vpc"}, q.Components)
		assert.Empty(t, q.Stacks)
		assert.Empty(t, q.SHAs)
	})

	t.Run("all dimensions", func(t *testing.T) {
		q := buildQuery("vpc", "dev", "abc123")
		assert.False(t, q.All)
		assert.Equal(t, []string{"vpc"}, q.Components)
		assert.Equal(t, []string{"dev"}, q.Stacks)
		assert.Equal(t, []string{"abc123"}, q.SHAs)
	})

	t.Run("stack and sha only", func(t *testing.T) {
		q := buildQuery("", "prod", "def456")
		assert.False(t, q.All)
		assert.Empty(t, q.Components)
		assert.Equal(t, []string{"prod"}, q.Stacks)
		assert.Equal(t, []string{"def456"}, q.SHAs)
	})
}

func TestGetFirstEnvValue(t *testing.T) {
	t.Run("returns first set value", func(t *testing.T) {
		t.Setenv("TEST_A", "")
		t.Setenv("TEST_B", "value-b")
		t.Setenv("TEST_C", "value-c")

		result := getFirstEnvValue("TEST_A", "TEST_B", "TEST_C")
		assert.Equal(t, "value-b", result)
	})

	t.Run("returns empty when none set", func(t *testing.T) {
		t.Setenv("TEST_X", "")
		t.Setenv("TEST_Y", "")

		result := getFirstEnvValue("TEST_X", "TEST_Y")
		assert.Empty(t, result)
	})
}
