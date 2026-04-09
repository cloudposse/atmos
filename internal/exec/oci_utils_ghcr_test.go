package exec

import (
	"testing"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestGetGHCRAuth tests the getGHCRAuth function which provides authentication
// for GitHub Container Registry (ghcr.io).
func TestGetGHCRAuth(t *testing.T) {
	tests := []struct {
		name             string
		atmosConfig      *schema.AtmosConfiguration
		expectAuth       bool
		expectedUsername string
		expectedPassword string
		expectedSource   string
	}{
		{
			name: "ATMOS_GITHUB_TOKEN with username - should authenticate",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AtmosGithubToken: "ghp_atmos_token_12345",
					GithubUsername:   "testuser",
				},
			},
			expectAuth:       true,
			expectedUsername: "testuser",
			expectedPassword: "ghp_atmos_token_12345",
			expectedSource:   "environment variable (ATMOS_GITHUB_TOKEN with username testuser)",
		},
		{
			name: "GITHUB_TOKEN fallback with username",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					GithubToken:    "ghp_standard_token_67890",
					GithubUsername: "anotheruser",
				},
			},
			expectAuth:       true,
			expectedUsername: "anotheruser",
			expectedPassword: "ghp_standard_token_67890",
			expectedSource:   "environment variable (GITHUB_TOKEN with username anotheruser)",
		},
		{
			name: "Token exists but no username - should return nil",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AtmosGithubToken: "ghp_token_no_user",
					// GithubUsername is empty
				},
			},
			expectAuth: false,
		},
		{
			name: "No token, has username - should return nil",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					GithubUsername: "useronly",
					// No tokens set
				},
			},
			expectAuth: false,
		},
		{
			name: "No token, no username - should return nil",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{},
			},
			expectAuth: false,
		},
		{
			name: "ATMOS_GITHUB_TOKEN takes precedence over GITHUB_TOKEN",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AtmosGithubToken: "ghp_atmos_priority",
					GithubToken:      "ghp_standard_fallback",
					GithubUsername:   "testuser",
				},
			},
			expectAuth:       true,
			expectedUsername: "testuser",
			expectedPassword: "ghp_atmos_priority",
			expectedSource:   "environment variable (ATMOS_GITHUB_TOKEN with username testuser)",
		},
		{
			name: "Empty ATMOS_GITHUB_TOKEN falls back to GITHUB_TOKEN",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AtmosGithubToken: "",
					GithubToken:      "ghp_fallback_token",
					GithubUsername:   "fallbackuser",
				},
			},
			expectAuth:       true,
			expectedUsername: "fallbackuser",
			expectedPassword: "ghp_fallback_token",
			expectedSource:   "environment variable (GITHUB_TOKEN with username fallbackuser)",
		},
		{
			name: "Username with special characters",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					GithubToken:    "ghp_token_special",
					GithubUsername: "user-name_123",
				},
			},
			expectAuth:       true,
			expectedUsername: "user-name_123",
			expectedPassword: "ghp_token_special",
			expectedSource:   "environment variable (GITHUB_TOKEN with username user-name_123)",
		},
		{
			name: "Long token value",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					GithubToken:    "ghp_" + string(make([]byte, 100)),
					GithubUsername: "longtoken",
				},
			},
			expectAuth:       true,
			expectedUsername: "longtoken",
		},
		{
			name: "Empty strings for both tokens - no username",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AtmosGithubToken: "",
					GithubToken:      "",
					GithubUsername:   "",
				},
			},
			expectAuth: false,
		},
		{
			name: "Whitespace token should be treated as empty",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					GithubToken:    "   ",
					GithubUsername: "user",
				},
			},
			expectAuth: false,
		},
		{
			name: "Whitespace username should be treated as empty",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					GithubToken:    "ghp_token",
					GithubUsername: "   ",
				},
			},
			expectAuth: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth, source := getGHCRAuth(tt.atmosConfig)

			if !tt.expectAuth {
				assert.Nil(t, auth, "Should return nil when authentication not possible")
				assert.Empty(t, source, "Source should be empty when no auth")
				return
			}

			require.NotNil(t, auth, "Should return auth method")
			assert.NotEmpty(t, source, "Source should not be empty")

			if tt.expectedSource != "" {
				assert.Equal(t, tt.expectedSource, source, "Auth source should match expected")
			}

			// Verify Basic auth structure.
			basic, ok := auth.(*authn.Basic)
			require.True(t, ok, "Auth should be *authn.Basic")
			assert.Equal(t, tt.expectedUsername, basic.Username, "Username should match")

			if tt.expectedPassword != "" {
				assert.Equal(t, tt.expectedPassword, basic.Password, "Password should match")
			}
		})
	}
}

// TestGetGHCRAuth_TokenPrecedence specifically tests the precedence order
// of token resolution: ATMOS_GITHUB_TOKEN > GITHUB_TOKEN.
func TestGetGHCRAuth_TokenPrecedence(t *testing.T) {
	tests := []struct {
		name          string
		atmosToken    string
		standardToken string
		expectedToken string
		expectAuth    bool
	}{
		{
			name:          "Both tokens set - ATMOS takes precedence",
			atmosToken:    "atmos_token",
			standardToken: "standard_token",
			expectedToken: "atmos_token",
			expectAuth:    true,
		},
		{
			name:          "Only ATMOS token set",
			atmosToken:    "atmos_only",
			standardToken: "",
			expectedToken: "atmos_only",
			expectAuth:    true,
		},
		{
			name:          "Only standard token set",
			atmosToken:    "",
			standardToken: "standard_only",
			expectedToken: "standard_only",
			expectAuth:    true,
		},
		{
			name:          "Neither token set",
			atmosToken:    "",
			standardToken: "",
			expectedToken: "",
			expectAuth:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AtmosGithubToken: tt.atmosToken,
					GithubToken:      tt.standardToken,
					GithubUsername:   "testuser",
				},
			}

			auth, source := getGHCRAuth(atmosConfig)

			if !tt.expectAuth {
				assert.Nil(t, auth)
				assert.Empty(t, source)
				return
			}

			require.NotNil(t, auth)
			basic, ok := auth.(*authn.Basic)
			require.True(t, ok)
			assert.Equal(t, tt.expectedToken, basic.Password)
		})
	}
}

// TestGetGHCRAuth_UsernameRequirement tests that username is required
// for GHCR authentication even when a token is present.
func TestGetGHCRAuth_UsernameRequirement(t *testing.T) {
	tests := []struct {
		name       string
		username   string
		expectAuth bool
		reason     string
	}{
		{
			name:       "Valid username - should authenticate",
			username:   "validuser",
			expectAuth: true,
			reason:     "Username is provided",
		},
		{
			name:       "Empty username - should not authenticate",
			username:   "",
			expectAuth: false,
			reason:     "GHCR requires username",
		},
		{
			name:       "Whitespace username - should not authenticate",
			username:   "   ",
			expectAuth: false,
			reason:     "Whitespace username is invalid",
		},
		{
			name:       "Username with valid characters",
			username:   "user-name_123",
			expectAuth: true,
			reason:     "Username contains valid characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					GithubToken:    "ghp_test_token",
					GithubUsername: tt.username,
				},
			}

			auth, source := getGHCRAuth(atmosConfig)

			if !tt.expectAuth {
				assert.Nil(t, auth, "Should return nil: %s", tt.reason)
				assert.Empty(t, source)
				return
			}

			require.NotNil(t, auth, "Should authenticate: %s", tt.reason)
			assert.NotEmpty(t, source)
			basic, ok := auth.(*authn.Basic)
			require.True(t, ok)
			assert.Equal(t, tt.username, basic.Username)
		})
	}
}

// TestGetGHCRAuth_AuthSourceFormat tests that the auth source string
// is formatted correctly for debugging purposes.
func TestGetGHCRAuth_AuthSourceFormat(t *testing.T) {
	tests := []struct {
		name           string
		atmosToken     string
		standardToken  string
		username       string
		expectedSource string
	}{
		{
			name:           "ATMOS token source includes username",
			atmosToken:     "token1",
			standardToken:  "",
			username:       "user1",
			expectedSource: "environment variable (ATMOS_GITHUB_TOKEN with username user1)",
		},
		{
			name:           "Standard token source includes username",
			atmosToken:     "",
			standardToken:  "token2",
			username:       "user2",
			expectedSource: "environment variable (GITHUB_TOKEN with username user2)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AtmosGithubToken: tt.atmosToken,
					GithubToken:      tt.standardToken,
					GithubUsername:   tt.username,
				},
			}

			auth, source := getGHCRAuth(atmosConfig)

			require.NotNil(t, auth)
			assert.Equal(t, tt.expectedSource, source,
				"Auth source should be formatted correctly for debugging")
		})
	}
}

// TestGetGHCRAuth_NilConfig tests that the function handles nil configuration gracefully.
func TestGetGHCRAuth_NilConfig(t *testing.T) {
	// This test ensures we don't panic with nil config.
	// Note: In practice, this shouldn't happen, but defensive programming is good.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("getGHCRAuth panicked with nil config: %v", r)
		}
	}()

	auth, source := getGHCRAuth(&schema.AtmosConfiguration{})
	assert.Nil(t, auth, "Should return nil for empty config")
	assert.Empty(t, source, "Should return empty source for empty config")
}

// TestGetGHCRAuth_Consistency tests that multiple calls with the same config
// return consistent results (deterministic behavior).
func TestGetGHCRAuth_Consistency(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			GithubToken:    "ghp_consistent_token",
			GithubUsername: "consistent_user",
		},
	}

	// Call the function multiple times.
	for i := 0; i < 10; i++ {
		auth, source := getGHCRAuth(atmosConfig)

		require.NotNil(t, auth, "Iteration %d: should return auth", i)
		assert.Equal(t, "environment variable (GITHUB_TOKEN with username consistent_user)", source)

		basic, ok := auth.(*authn.Basic)
		require.True(t, ok)
		assert.Equal(t, "consistent_user", basic.Username)
		assert.Equal(t, "ghp_consistent_token", basic.Password)
	}
}
