package toolchain

import (
	"os"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"

	httpClient "github.com/cloudposse/atmos/pkg/http"
)

// TestGitHubTokenEnvBinding tests that environment variables are properly bound to Viper in TestMain.
func TestGitHubTokenEnvBinding(t *testing.T) {
	// The TestMain function in main_test.go should have already bound the environment variables.
	// This test verifies that the binding is working correctly.

	t.Run("GITHUB_TOKEN from CI is accessible", func(t *testing.T) {
		// Simulate CI environment setting GITHUB_TOKEN.
		testToken := "test-ci-github-token"
		t.Setenv("GITHUB_TOKEN", testToken)

		// Since TestMain already bound the env vars, we need to create a new Viper instance
		// or reset the bound value to test properly.
		v := viper.New()
		v.BindEnv("github-token", "ATMOS_GITHUB_TOKEN", "GITHUB_TOKEN")

		// Verify it's available through the new Viper instance.
		token := v.GetString("github-token")
		assert.Equal(t, testToken, token, "GITHUB_TOKEN should be accessible through viper")
	})

	t.Run("ATMOS_GITHUB_TOKEN takes precedence", func(t *testing.T) {
		// Set both tokens.
		t.Setenv("GITHUB_TOKEN", "github-token")
		t.Setenv("ATMOS_GITHUB_TOKEN", "atmos-github-token")

		// Create a new Viper instance to test binding.
		v := viper.New()
		v.BindEnv("github-token", "ATMOS_GITHUB_TOKEN", "GITHUB_TOKEN")

		// Verify ATMOS_GITHUB_TOKEN takes precedence.
		token := v.GetString("github-token")
		assert.Equal(t, "atmos-github-token", token, "ATMOS_GITHUB_TOKEN should take precedence over GITHUB_TOKEN")
	})

	t.Run("GitHub API functions work with environment token", func(t *testing.T) {
		// If GITHUB_TOKEN is set in the environment (as it would be in CI),
		// GetGitHubTokenFromEnv should return it.
		if envToken := os.Getenv("GITHUB_TOKEN"); envToken != "" {
			token := httpClient.GetGitHubTokenFromEnv()
			// The token should be accessible (either the env token or empty if not set).
			assert.NotEmpty(t, token, "GetGitHubTokenFromEnv should not return empty when GITHUB_TOKEN is set")
		}

		// Test that a new GitHub API client can be created.
		client := NewGitHubAPIClient()
		assert.NotNil(t, client, "NewGitHubAPIClient should return a valid client")
	})

	t.Run("TestMain binds environment correctly", func(t *testing.T) {
		// This test verifies that the binding in TestMain is working.
		// If GITHUB_TOKEN is set in environment, it should be accessible.
		if envToken := os.Getenv("GITHUB_TOKEN"); envToken != "" {
			// The global viper instance should have the binding from TestMain.
			viperToken := viper.GetString("github-token")
			assert.Equal(t, envToken, viperToken, "TestMain should have bound GITHUB_TOKEN to viper")
		} else if envToken := os.Getenv("ATMOS_GITHUB_TOKEN"); envToken != "" {
			// Or if ATMOS_GITHUB_TOKEN is set, it should be accessible.
			viperToken := viper.GetString("github-token")
			assert.Equal(t, envToken, viperToken, "TestMain should have bound ATMOS_GITHUB_TOKEN to viper")
		}
	})
}
