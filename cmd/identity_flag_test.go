package cmd

import (
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestExtractIdentityFromArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected string
	}{
		{
			name:     "--identity with value (space-separated)",
			args:     []string{"auth", "login", "--identity", "myidentity"},
			expected: "myidentity",
		},
		{
			name:     "--identity=value (equals sign)",
			args:     []string{"auth", "login", "--identity=myidentity"},
			expected: "myidentity",
		},
		{
			name:     "-i with value (short flag, space-separated)",
			args:     []string{"auth", "login", "-i", "myidentity"},
			expected: "myidentity",
		},
		{
			name:     "-i=value (short flag, equals sign)",
			args:     []string{"auth", "login", "-i=myidentity"},
			expected: "myidentity",
		},
		{
			name:     "--identity without value (interactive selection)",
			args:     []string{"auth", "login", "--identity"},
			expected: cfg.IdentityFlagSelectValue,
		},
		{
			name:     "-i without value (interactive selection)",
			args:     []string{"auth", "login", "-i"},
			expected: cfg.IdentityFlagSelectValue,
		},
		{
			name:     "--identity= (empty value, interactive selection)",
			args:     []string{"auth", "login", "--identity="},
			expected: cfg.IdentityFlagSelectValue,
		},
		{
			name:     "-i= (empty value, interactive selection)",
			args:     []string{"auth", "login", "-i="},
			expected: cfg.IdentityFlagSelectValue,
		},
		{
			name:     "--identity with value after positional args",
			args:     []string{"terraform", "plan", "vpc", "--stack", "test", "--identity", "myid"},
			expected: "myid",
		},
		{
			name:     "--identity without value after positional args",
			args:     []string{"terraform", "plan", "vpc", "--stack", "test", "--identity"},
			expected: cfg.IdentityFlagSelectValue,
		},
		{
			name:     "no identity flag",
			args:     []string{"auth", "login"},
			expected: "",
		},
		{
			name:     "--identity followed by another flag (no value)",
			args:     []string{"auth", "login", "--identity", "--some-other-flag"},
			expected: cfg.IdentityFlagSelectValue,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := extractIdentityFromArgs(tc.args)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestGetIdentityFromFlags(t *testing.T) {
	tests := []struct {
		name     string
		osArgs   []string
		envVar   string
		envValue string
		expected string
	}{
		{
			name:     "explicit identity from args",
			osArgs:   []string{"atmos", "auth", "login", "--identity", "myid"},
			expected: "myid",
		},
		{
			name:     "interactive selection from args",
			osArgs:   []string{"atmos", "auth", "login", "--identity"},
			expected: cfg.IdentityFlagSelectValue,
		},
		{
			name:     "identity from environment variable",
			osArgs:   []string{"atmos", "auth", "login"},
			envVar:   "ATMOS_IDENTITY",
			envValue: "env-identity",
			expected: "env-identity",
		},
		{
			name:     "args override environment variable",
			osArgs:   []string{"atmos", "auth", "login", "--identity", "args-identity"},
			envVar:   "ATMOS_IDENTITY",
			envValue: "env-identity",
			expected: "args-identity",
		},
		{
			name:     "no identity specified",
			osArgs:   []string{"atmos", "auth", "login"},
			expected: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Reset Viper state for test isolation.
			viper.Reset()

			// Bind Viper to ATMOS_IDENTITY environment variable (mimics cmd/auth.go initialization).
			_ = viper.BindEnv("identity", "ATMOS_IDENTITY", "IDENTITY")

			// Set up environment variable if specified.
			if tc.envVar != "" {
				t.Setenv(tc.envVar, tc.envValue)
			}

			// Create a minimal command with identity flag.
			cmd := &cobra.Command{
				Use: "test",
			}
			cmd.Flags().StringP("identity", "i", "", "Identity flag")
			if identityFlag := cmd.Flags().Lookup("identity"); identityFlag != nil {
				identityFlag.NoOptDefVal = cfg.IdentityFlagSelectValue
			}

			// Parse flags to simulate Cobra's normal parsing.
			// Note: This will have the NoOptDefVal issue, but GetIdentityFromFlags should bypass it.
			_ = cmd.ParseFlags(tc.osArgs[2:]) // Skip "atmos" and command name.

			result := GetIdentityFromFlags(cmd, tc.osArgs)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestCreateAuthManagerFromIdentity_NoAuthConfigured(t *testing.T) {
	tests := []struct {
		name          string
		identityName  string
		authConfig    *schema.AuthConfig
		expectError   bool
		expectedError error
	}{
		{
			name:          "nil authConfig with identity specified - should return error",
			identityName:  "my-identity",
			authConfig:    nil,
			expectError:   true,
			expectedError: errUtils.ErrAuthNotConfigured,
		},
		{
			name:         "empty identities with identity specified - should return error",
			identityName: "my-identity",
			authConfig: &schema.AuthConfig{
				Identities: map[string]schema.Identity{},
			},
			expectError:   true,
			expectedError: errUtils.ErrAuthNotConfigured,
		},
		{
			name:         "nil authConfig without identity - should return nil",
			identityName: "",
			authConfig:   nil,
			expectError:  false,
		},
		{
			name:         "empty identities without identity - should return nil",
			identityName: "",
			authConfig: &schema.AuthConfig{
				Identities: map[string]schema.Identity{},
			},
			expectError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			authManager, err := CreateAuthManagerFromIdentity(tc.identityName, tc.authConfig)

			if tc.expectError {
				require.Error(t, err, "Expected error but got nil")
				assert.True(t, errors.Is(err, tc.expectedError), "Expected error to be %v, got %v", tc.expectedError, err)
				assert.Nil(t, authManager, "Expected authManager to be nil when error occurs")
			} else {
				require.NoError(t, err, "Expected no error but got: %v", err)
				assert.Nil(t, authManager, "Expected authManager to be nil when no identity specified")
			}
		})
	}
}
