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

func TestNormalizeIdentityValue(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Boolean false representations - should return __DISABLED__.
		{name: "lowercase false", input: "false", expected: cfg.IdentityFlagDisabledValue},
		{name: "capitalized False", input: "False", expected: cfg.IdentityFlagDisabledValue},
		{name: "uppercase FALSE", input: "FALSE", expected: cfg.IdentityFlagDisabledValue},
		{name: "mixed case FaLsE", input: "FaLsE", expected: cfg.IdentityFlagDisabledValue},
		{name: "zero string", input: "0", expected: cfg.IdentityFlagDisabledValue},
		{name: "lowercase no", input: "no", expected: cfg.IdentityFlagDisabledValue},
		{name: "capitalized No", input: "No", expected: cfg.IdentityFlagDisabledValue},
		{name: "uppercase NO", input: "NO", expected: cfg.IdentityFlagDisabledValue},
		{name: "lowercase off", input: "off", expected: cfg.IdentityFlagDisabledValue},
		{name: "capitalized Off", input: "Off", expected: cfg.IdentityFlagDisabledValue},
		{name: "uppercase OFF", input: "OFF", expected: cfg.IdentityFlagDisabledValue},

		// Non-boolean values - should return unchanged.
		{name: "empty string", input: "", expected: ""},
		{name: "identity name", input: "aws-sso", expected: "aws-sso"},
		{name: "select sentinel", input: cfg.IdentityFlagSelectValue, expected: cfg.IdentityFlagSelectValue},
		{name: "number 1", input: "1", expected: "1"},
		{name: "yes", input: "yes", expected: "yes"},
		{name: "true", input: "true", expected: "true"},
		{name: "on", input: "on", expected: "on"},
		{name: "false with suffix", input: "false-identity", expected: "false-identity"},
		{name: "prefix false", input: "myfalse", expected: "myfalse"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := normalizeIdentityValue(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

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
		// Disabled identity tests.
		{
			name:     "disabled via flag lowercase false",
			osArgs:   []string{"atmos", "auth", "login", "--identity", "false"},
			expected: cfg.IdentityFlagDisabledValue,
		},
		{
			name:     "disabled via flag uppercase FALSE",
			osArgs:   []string{"atmos", "auth", "login", "--identity", "FALSE"},
			expected: cfg.IdentityFlagDisabledValue,
		},
		{
			name:     "disabled via flag equals false",
			osArgs:   []string{"atmos", "auth", "login", "--identity=false"},
			expected: cfg.IdentityFlagDisabledValue,
		},
		{
			name:     "disabled via flag number 0",
			osArgs:   []string{"atmos", "auth", "login", "--identity", "0"},
			expected: cfg.IdentityFlagDisabledValue,
		},
		{
			name:     "disabled via flag no",
			osArgs:   []string{"atmos", "auth", "login", "--identity", "no"},
			expected: cfg.IdentityFlagDisabledValue,
		},
		{
			name:     "disabled via flag off",
			osArgs:   []string{"atmos", "auth", "login", "--identity", "off"},
			expected: cfg.IdentityFlagDisabledValue,
		},
		{
			name:     "disabled via environment variable",
			osArgs:   []string{"atmos", "auth", "login"},
			envVar:   "ATMOS_IDENTITY",
			envValue: "false",
			expected: cfg.IdentityFlagDisabledValue,
		},
		{
			name:     "flag disabled overrides env identity",
			osArgs:   []string{"atmos", "auth", "login", "--identity", "false"},
			envVar:   "ATMOS_IDENTITY",
			envValue: "some-identity",
			expected: cfg.IdentityFlagDisabledValue,
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
