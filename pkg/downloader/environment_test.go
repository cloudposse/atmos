package downloader

import (
	"context"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOverlayEnvironment(t *testing.T) {
	tests := []struct {
		name    string
		base    []string
		overlay []string
		want    []string
	}{
		{name: "empty overlay copies base", base: []string{"A=1", "B=2"}, overlay: nil, want: []string{"A=1", "B=2"}},
		{name: "overlay replaces same name and appends new", base: []string{"A=1", "B=2"}, overlay: []string{"B=3", "C=4"}, want: []string{"A=1", "B=3", "C=4"}},
		{name: "later overlay entry wins", base: []string{"A=1"}, overlay: []string{"A=2", "A=3"}, want: []string{"A=3"}},
		{name: "value may contain equals", base: nil, overlay: []string{"GIT_SSH_COMMAND=ssh -o X=y"}, want: []string{"GIT_SSH_COMMAND=ssh -o X=y"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := append([]string(nil), tt.base...)
			overlay := append([]string(nil), tt.overlay...)

			got := overlayEnvironment(tt.base, tt.overlay)

			assert.Equal(t, tt.want, got)
			// Neither input is modified.
			assert.Equal(t, base, append([]string(nil), tt.base...))
			assert.Equal(t, overlay, append([]string(nil), tt.overlay...))
		})
	}
}

func TestWithEnvironment_MergesAndIsolatesCopies(t *testing.T) {
	assert.Nil(t, EnvironmentFromContext(context.Background()))
	assert.Nil(t, EnvironmentFromContext(nil)) //nolint:staticcheck // A nil context must not panic.

	parent := context.Background()
	assert.Equal(t, parent, WithEnvironment(parent, nil), "an empty overlay returns the context unchanged")

	ctx := WithEnvironment(parent, []string{"A=1", "B=2"})
	child := WithEnvironment(ctx, []string{"B=3"})

	assert.Equal(t, []string{"A=1", "B=2"}, EnvironmentFromContext(ctx))
	assert.Equal(t, []string{"A=1", "B=3"}, EnvironmentFromContext(child))

	// Mutating a returned copy never changes what the context carries.
	got := EnvironmentFromContext(ctx)
	got[0] = "A=mutated"
	assert.Equal(t, []string{"A=1", "B=2"}, EnvironmentFromContext(ctx))
}

func TestApplyFetchEnvironment(t *testing.T) {
	t.Run("no overlay leaves the command environment untouched", func(t *testing.T) {
		cmd := exec.Command("git", "version")
		applyFetchEnvironment(context.Background(), cmd)
		assert.Nil(t, cmd.Env, "a nil Env means the subprocess inherits the process environment, as before")
	})

	t.Run("overlay starts from the process environment", func(t *testing.T) {
		t.Setenv("ATMOS_TEST_FETCH_ENV_BASE", "process")
		t.Setenv("ATMOS_TEST_FETCH_ENV_SHADOWED", "process")
		cmd := exec.Command("git", "version")

		applyFetchEnvironment(WithEnvironment(context.Background(), []string{"ATMOS_TEST_FETCH_ENV_SHADOWED=identity", "ATMOS_TEST_FETCH_ENV_NEW=identity"}), cmd)

		require.NotNil(t, cmd.Env)
		assert.Contains(t, cmd.Env, "ATMOS_TEST_FETCH_ENV_BASE=process")
		assert.Contains(t, cmd.Env, "ATMOS_TEST_FETCH_ENV_SHADOWED=identity")
		assert.NotContains(t, cmd.Env, "ATMOS_TEST_FETCH_ENV_SHADOWED=process")
		assert.Contains(t, cmd.Env, "ATMOS_TEST_FETCH_ENV_NEW=identity")
	})

	t.Run("overlay layers over an environment setupGitEnv already built", func(t *testing.T) {
		cmd := exec.Command("git", "version")
		cmd.Env = []string{"KEEP=1", "GIT_SSH_COMMAND=ssh -i key"}

		applyFetchEnvironment(WithEnvironment(context.Background(), []string{"SSH_AUTH_SOCK=/tmp/agent.sock"}), cmd)

		assert.Equal(t, []string{"KEEP=1", "GIT_SSH_COMMAND=ssh -i key", "SSH_AUTH_SOCK=/tmp/agent.sock"}, cmd.Env)
	})
}

func TestFetchEnvironmentValue_PrefersFetchEnvironmentOverProcess(t *testing.T) {
	t.Setenv("ATMOS_TEST_FETCH_VALUE", "process")
	t.Setenv("ATMOS_TEST_FETCH_VALUE_ONLY_PROCESS", "process-only")
	ctx := WithEnvironment(context.Background(), []string{"ATMOS_TEST_FETCH_VALUE=identity"})

	assert.Equal(t, "identity", fetchEnvironmentValue(ctx, "ATMOS_TEST_FETCH_VALUE"))
	assert.Equal(t, "process-only", fetchEnvironmentValue(ctx, "ATMOS_TEST_FETCH_VALUE_ONLY_PROCESS"))
	assert.Equal(t, "process", fetchEnvironmentValue(context.Background(), "ATMOS_TEST_FETCH_VALUE"))
}
