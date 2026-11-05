package downloader

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time guards: a rename of a GHCR credential setting must fail the build here, not silently skip a case.
var _ = schema.AtmosSettings{AtmosGithubToken: "", GithubToken: "", GithubUsername: ""}

// clearGHCREnv isolates a test from credentials in the host environment, such as the ones CI runners set.
func clearGHCREnv(t *testing.T) {
	t.Helper()

	for _, name := range []string{"ATMOS_GITHUB_TOKEN", "GITHUB_TOKEN", "ATMOS_GITHUB_USERNAME", "GITHUB_ACTOR", "GITHUB_USERNAME"} {
		t.Setenv(name, "")
	}
}

func TestWithGHCREnvFallback(t *testing.T) {
	tests := []struct {
		name             string
		env              map[string]string
		settings         schema.AtmosSettings
		expectedAtmosTok string
		expectedToken    string
		expectedUsername string
	}{
		{
			name: "empty settings take the standard GitHub Actions variables",
			env:  map[string]string{"GITHUB_TOKEN": "env-token", "GITHUB_ACTOR": "env-actor"},

			expectedToken:    "env-token",
			expectedUsername: "env-actor",
		},
		{
			name: "Atmos-specific variables are read too",
			env:  map[string]string{"ATMOS_GITHUB_TOKEN": "atmos-token", "ATMOS_GITHUB_USERNAME": "atmos-user"},

			expectedAtmosTok: "atmos-token",
			expectedUsername: "atmos-user",
		},
		{
			name: "username variables keep their precedence",
			env: map[string]string{
				"ATMOS_GITHUB_USERNAME": "first", "GITHUB_ACTOR": "second", "GITHUB_USERNAME": "third",
			},
			expectedUsername: "first",
		},
		{
			name:             "configured settings win over the environment",
			env:              map[string]string{"GITHUB_TOKEN": "env-token", "GITHUB_ACTOR": "env-actor"},
			settings:         schema.AtmosSettings{GithubToken: "cfg-token", GithubUsername: "cfg-user"},
			expectedToken:    "cfg-token",
			expectedUsername: "cfg-user",
		},
		{
			name:             "an environment value fills only the empty setting",
			env:              map[string]string{"GITHUB_TOKEN": "env-token", "GITHUB_ACTOR": "env-actor"},
			settings:         schema.AtmosSettings{GithubUsername: "cfg-user"},
			expectedToken:    "env-token",
			expectedUsername: "cfg-user",
		},
		{
			name: "nothing configured and nothing in the environment stays empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearGHCREnv(t)
			for name, value := range tt.env {
				t.Setenv(name, value)
			}
			original := &schema.AtmosConfiguration{Settings: tt.settings}

			effective := withGHCREnvFallback(original)

			assert.Equal(t, tt.expectedAtmosTok, effective.Settings.AtmosGithubToken)
			assert.Equal(t, tt.expectedToken, effective.Settings.GithubToken)
			assert.Equal(t, tt.expectedUsername, effective.Settings.GithubUsername)
			assert.Equal(t, tt.settings, original.Settings, "the original configuration must not be modified")
		})
	}
}

func TestWithGHCREnvFallback_NilConfig(t *testing.T) {
	clearGHCREnv(t)
	t.Setenv("GITHUB_TOKEN", "env-token")
	t.Setenv("GITHUB_ACTOR", "env-actor")

	effective := withGHCREnvFallback(nil)

	require.NotNil(t, effective)
	assert.Equal(t, "env-token", effective.Settings.GithubToken)
	assert.Equal(t, "env-actor", effective.Settings.GithubUsername)
}

// TestWithGHCREnvFallback_EnablesGHCRAuth is the behavior that matters: with an empty configuration, as an
// atmos.yaml import sees it, environment-only credentials now produce GHCR authentication, and without them
// the pull stays anonymous.
func TestWithGHCREnvFallback_EnablesGHCRAuth(t *testing.T) {
	t.Run("environment-only credentials authenticate", func(t *testing.T) {
		clearGHCREnv(t)
		t.Setenv("GITHUB_TOKEN", "env-token")
		t.Setenv("GITHUB_ACTOR", "env-actor")

		auth, source := oci.GHCRAuth(withGHCREnvFallback(&schema.AtmosConfiguration{}))
		require.NotNil(t, auth)
		assert.Contains(t, source, "GITHUB_TOKEN")
		assert.Contains(t, source, "env-actor")
	})

	t.Run("a token without a username stays anonymous", func(t *testing.T) {
		clearGHCREnv(t)
		t.Setenv("GITHUB_TOKEN", "env-token")

		auth, _ := oci.GHCRAuth(withGHCREnvFallback(&schema.AtmosConfiguration{}))
		assert.Nil(t, auth)
	})

	t.Run("no credentials stay anonymous", func(t *testing.T) {
		clearGHCREnv(t)

		auth, _ := oci.GHCRAuth(withGHCREnvFallback(&schema.AtmosConfiguration{}))
		assert.Nil(t, auth)
	})
}
