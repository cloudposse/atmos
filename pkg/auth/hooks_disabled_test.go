package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestIsAuthenticationDisabled(t *testing.T) {
	tests := []struct {
		name     string
		identity string
		expected bool
	}{
		{
			name:     "disabled sentinel value",
			identity: cfg.IdentityFlagDisabledValue,
			expected: true,
		},
		{
			name:     "normal identity name",
			identity: "aws-sso",
			expected: false,
		},
		{
			name:     "empty string",
			identity: "",
			expected: false,
		},
		{
			name:     "select sentinel value",
			identity: cfg.IdentityFlagSelectValue,
			expected: false,
		},
		{
			name:     "literal string false",
			identity: "false",
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := isAuthenticationDisabled(tc.identity)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestTerraformPreHook_DisabledIdentity(t *testing.T) {
	tests := []struct {
		name        string
		identity    string
		authConfig  map[string]any
		expectError bool
		description string
	}{
		{
			name:     "authentication disabled with auth configured",
			identity: cfg.IdentityFlagDisabledValue,
			authConfig: map[string]any{
				"identities": map[string]any{
					"test-identity": map[string]any{
						"kind":    "aws/user",
						"default": true,
					},
				},
			},
			expectError: false,
			description: "Should skip authentication and return nil without error",
		},
		{
			name:        "authentication disabled without auth configured",
			identity:    cfg.IdentityFlagDisabledValue,
			authConfig:  map[string]any{},
			expectError: false,
			description: "Should skip authentication even when no auth is configured",
		},
		{
			name:     "normal identity with auth configured",
			identity: "test-identity",
			authConfig: map[string]any{
				"identities": map[string]any{
					"test-identity": map[string]any{
						"kind": "aws/user",
					},
				},
				"providers": map[string]any{
					"test-provider": map[string]any{
						"kind": "aws",
					},
				},
			},
			expectError: true, // Will fail due to incomplete mock setup, but that's expected.
			description: "Should attempt authentication with normal identity",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create minimal atmos config.
			atmosConfig := &schema.AtmosConfiguration{
				Logs: schema.Logs{
					Level: "error", // Suppress logs during tests.
				},
			}

			// Create stack info with identity and auth config.
			stackInfo := &schema.ConfigAndStacksInfo{
				Identity:             tc.identity,
				ComponentAuthSection: tc.authConfig,
			}

			// Call TerraformPreHook.
			err := TerraformPreHook(atmosConfig, stackInfo)

			if tc.expectError {
				// For disabled identity, we expect NO error.
				// For normal identity with incomplete setup, we expect error.
				if tc.identity == cfg.IdentityFlagDisabledValue {
					require.NoError(t, err, "TerraformPreHook should not error when authentication is disabled")
				} else {
					// Normal identity will fail due to incomplete mock setup - that's expected.
					// The important thing is that disabled identity does NOT reach this code path.
					assert.Error(t, err, "Expected error for normal identity without complete setup")
				}
			} else {
				require.NoError(t, err, tc.description)
			}
		})
	}
}

func TestTerraformPreHook_DisabledIdentitySkipsAuthentication(t *testing.T) {
	// This test verifies that when identity is disabled, TerraformPreHook returns early
	// WITHOUT calling authenticateAndWriteEnv, which would fail without proper provider setup.

	// Create minimal atmos config.
	atmosConfig := &schema.AtmosConfiguration{
		Logs: schema.Logs{
			Level: "error", // Suppress logs during tests.
		},
	}

	// Create stack info with disabled identity and auth config that WOULD work if reached.
	stackInfo := &schema.ConfigAndStacksInfo{
		Identity: cfg.IdentityFlagDisabledValue,
		ComponentAuthSection: map[string]any{
			"identities": map[string]any{
				"test-identity": map[string]any{
					"kind":    "aws/user",
					"default": true,
				},
			},
			"providers": map[string]any{
				"aws-provider": map[string]any{
					"kind": "aws",
				},
			},
		},
	}

	// Call TerraformPreHook - should return nil WITHOUT attempting authentication.
	err := TerraformPreHook(atmosConfig, stackInfo)

	// Verify no error occurred (authentication was skipped).
	require.NoError(t, err, "TerraformPreHook should skip authentication and return nil when identity is disabled")

	// Verify ComponentEnvSection was NOT populated (authentication didn't run).
	assert.Nil(t, stackInfo.ComponentEnvSection, "ComponentEnvSection should remain nil when authentication is skipped")
}

func TestTerraformPreHook_DisabledIdentityWithNoAuthConfig(t *testing.T) {
	// Verify that disabled identity works even when there's NO auth configuration at all.

	atmosConfig := &schema.AtmosConfiguration{
		Logs: schema.Logs{
			Level: "error",
		},
	}

	stackInfo := &schema.ConfigAndStacksInfo{
		Identity:             cfg.IdentityFlagDisabledValue,
		ComponentAuthSection: map[string]any{}, // No providers or identities.
	}

	// Should return nil without error - both "no auth config" and "disabled" exit early.
	err := TerraformPreHook(atmosConfig, stackInfo)
	require.NoError(t, err, "Should skip authentication when disabled, even with no auth config")
}
