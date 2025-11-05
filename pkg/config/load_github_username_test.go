package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestGithubUsernameEnvVarPrecedence tests that github_username environment
// variables are bound in the correct precedence order:
// ATMOS_GITHUB_USERNAME > GITHUB_ACTOR > GITHUB_USERNAME.
func TestGithubUsernameEnvVarPrecedence(t *testing.T) {
	tests := []struct {
		name                     string
		atmosGithubUsername      string
		githubActor              string
		githubUsername           string
		expectedUsername         string
		expectUsernameConfigured bool
	}{
		{
			name:                     "ATMOS_GITHUB_USERNAME takes highest precedence",
			atmosGithubUsername:      "atmos-user",
			githubActor:              "actor-user",
			githubUsername:           "github-user",
			expectedUsername:         "atmos-user",
			expectUsernameConfigured: true,
		},
		{
			name:                     "GITHUB_ACTOR used when ATMOS_GITHUB_USERNAME not set",
			atmosGithubUsername:      "",
			githubActor:              "actor-user",
			githubUsername:           "github-user",
			expectedUsername:         "actor-user",
			expectUsernameConfigured: true,
		},
		{
			name:                     "GITHUB_USERNAME used when neither ATMOS nor ACTOR set",
			atmosGithubUsername:      "",
			githubActor:              "",
			githubUsername:           "github-user",
			expectedUsername:         "github-user",
			expectUsernameConfigured: true,
		},
		{
			name:                     "No username when all env vars empty",
			atmosGithubUsername:      "",
			githubActor:              "",
			githubUsername:           "",
			expectedUsername:         "",
			expectUsernameConfigured: false,
		},
		{
			name:                     "ATMOS_GITHUB_USERNAME overrides others even with empty ACTOR",
			atmosGithubUsername:      "atmos-priority",
			githubActor:              "",
			githubUsername:           "github-fallback",
			expectedUsername:         "atmos-priority",
			expectUsernameConfigured: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// t.Setenv automatically handles cleanup.

			// Set test environment variables.
			if tt.atmosGithubUsername != "" {
				t.Setenv("ATMOS_GITHUB_USERNAME", tt.atmosGithubUsername)
			}
			if tt.githubActor != "" {
				t.Setenv("GITHUB_ACTOR", tt.githubActor)
			}
			if tt.githubUsername != "" {
				t.Setenv("GITHUB_USERNAME", tt.githubUsername)
			}

			// Load configuration.
			configInfo := schema.ConfigAndStacksInfo{}
			atmosConfig, err := InitCliConfig(configInfo, false)
			require.NoError(t, err, "Should load config successfully")

			if tt.expectUsernameConfigured {
				assert.Equal(t, tt.expectedUsername, atmosConfig.Settings.GithubUsername,
					"Github username should match expected value")
			} else {
				assert.Empty(t, atmosConfig.Settings.GithubUsername,
					"Github username should be empty when no env vars set")
			}
		})
	}
}

// TestGithubUsernameWithGitHubActions simulates GitHub Actions environment
// where GITHUB_ACTOR is automatically available.
func TestGithubUsernameWithGitHubActions(t *testing.T) {
	// t.Setenv automatically handles cleanup.

	t.Setenv("GITHUB_ACTOR", "github-actions[bot]")
	t.Setenv("GITHUB_TOKEN", "ghp_github_actions_token")
	t.Setenv("CI", "true")

	configInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := InitCliConfig(configInfo, false)
	require.NoError(t, err)

	// GITHUB_ACTOR should be automatically picked up.
	assert.Equal(t, "github-actions[bot]", atmosConfig.Settings.GithubUsername,
		"Should use GITHUB_ACTOR in GitHub Actions environment")
	assert.Equal(t, "ghp_github_actions_token", atmosConfig.Settings.GithubToken,
		"Should use GITHUB_TOKEN in GitHub Actions environment")
}

// TestGithubUsernameOverrideInGitHubActions tests that ATMOS_GITHUB_USERNAME
// can override GITHUB_ACTOR even in GitHub Actions environment.
func TestGithubUsernameOverrideInGitHubActions(t *testing.T) {
	// t.Setenv automatically handles cleanup.

	t.Setenv("GITHUB_ACTOR", "github-actions[bot]")
	t.Setenv("ATMOS_GITHUB_USERNAME", "custom-user")
	t.Setenv("CI", "true")

	configInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := InitCliConfig(configInfo, false)
	require.NoError(t, err)

	// ATMOS_GITHUB_USERNAME should take precedence.
	assert.Equal(t, "custom-user", atmosConfig.Settings.GithubUsername,
		"ATMOS_GITHUB_USERNAME should override GITHUB_ACTOR")
}

// TestGithubUsernameWhitespace tests that whitespace in env vars is handled.
func TestGithubUsernameWhitespace(t *testing.T) {
	tests := []struct {
		name             string
		envValue         string
		expectedUsername string
	}{
		{
			name:             "Whitespace before and after",
			envValue:         "  username  ",
			expectedUsername: "  username  ", // Config loads as-is, trimming happens in getGHCRAuth
		},
		{
			name:             "Only whitespace",
			envValue:         "   ",
			expectedUsername: "   ", // Config loads as-is
		},
		{
			name:             "Valid username",
			envValue:         "valid-user",
			expectedUsername: "valid-user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ATMOS_GITHUB_USERNAME", tt.envValue)

			configInfo := schema.ConfigAndStacksInfo{}
			atmosConfig, err := InitCliConfig(configInfo, false)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedUsername, atmosConfig.Settings.GithubUsername)
		})
	}
}

// TestGithubUsernameSpecialCharacters tests usernames with special characters.
func TestGithubUsernameSpecialCharacters(t *testing.T) {
	tests := []struct {
		name     string
		username string
	}{
		{
			name:     "Username with hyphen",
			username: "user-name",
		},
		{
			name:     "Username with underscore",
			username: "user_name",
		},
		{
			name:     "Username with numbers",
			username: "user123",
		},
		{
			name:     "Username with dots",
			username: "user.name",
		},
		{
			name:     "Complex username",
			username: "user-name_123.test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ATMOS_GITHUB_USERNAME", tt.username)

			configInfo := schema.ConfigAndStacksInfo{}
			atmosConfig, err := InitCliConfig(configInfo, false)
			require.NoError(t, err)

			assert.Equal(t, tt.username, atmosConfig.Settings.GithubUsername)
		})
	}
}
