package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
)

// TestProcessArgsAndFlags_IdentityFlag tests the identity flag parsing logic in processArgsAndFlags.
// This is a unit test for the fix that handles `--identity` without a value.
func TestProcessArgsAndFlags_IdentityFlag(t *testing.T) {
	tests := []struct {
		name             string
		args             []string
		expectedIdentity string
		expectError      bool
		description      string
	}{
		{
			name:             "--identity without value should set to __SELECT__",
			args:             []string{"plan", "vpc", "--stack", "test-stack", "--identity"},
			expectedIdentity: cfg.IdentityFlagSelectValue,
			expectError:      false,
			description:      "Interactive selection mode",
		},
		{
			name:             "--identity with value should use that value",
			args:             []string{"plan", "vpc", "--stack", "test-stack", "--identity", "test-identity"},
			expectedIdentity: "test-identity",
			expectError:      false,
			description:      "Explicit identity specified",
		},
		{
			name:             "--identity=value should use that value",
			args:             []string{"plan", "vpc", "--stack", "test-stack", "--identity=test-identity"},
			expectedIdentity: "test-identity",
			expectError:      false,
			description:      "Explicit identity with = syntax",
		},
		{
			name:             "--identity= should set to __SELECT__",
			args:             []string{"plan", "vpc", "--stack", "test-stack", "--identity="},
			expectedIdentity: cfg.IdentityFlagSelectValue,
			expectError:      false,
			description:      "Empty value means interactive selection",
		},
		{
			name:             "no --identity flag should have empty identity",
			args:             []string{"plan", "vpc", "--stack", "test-stack"},
			expectedIdentity: "",
			expectError:      false,
			description:      "No identity flag provided",
		},
		{
			name:             "--identity followed by another flag",
			args:             []string{"plan", "vpc", "--identity", "--stack", "test-stack"},
			expectedIdentity: cfg.IdentityFlagSelectValue,
			expectError:      false,
			description:      "--identity without value, next arg is another flag",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Call processArgsAndFlags directly
			info, err := processArgsAndFlags("terraform", tc.args)

			// Check error expectation
			if tc.expectError {
				assert.Error(t, err, tc.description)
				return
			}
			require.NoError(t, err, tc.description)

			// Verify identity was parsed correctly
			assert.Equal(t, tc.expectedIdentity, info.Identity, tc.description)
			t.Logf("Parsed identity: %q (expected: %q)", info.Identity, tc.expectedIdentity)
		})
	}
}

// TestProcessArgsAndFlags_IdentityFlagHelmfile tests identity flag parsing for helmfile commands.
func TestProcessArgsAndFlags_IdentityFlagHelmfile(t *testing.T) {
	tests := []struct {
		name             string
		args             []string
		expectedIdentity string
		description      string
	}{
		{
			name:             "--identity without value",
			args:             []string{"sync", "nginx", "--stack", "test-stack", "--identity"},
			expectedIdentity: cfg.IdentityFlagSelectValue,
			description:      "Interactive selection mode",
		},
		{
			name:             "--identity with value",
			args:             []string{"sync", "nginx", "--stack", "test-stack", "--identity", "test-identity"},
			expectedIdentity: "test-identity",
			description:      "Explicit identity specified",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			info, err := processArgsAndFlags("helmfile", tc.args)
			require.NoError(t, err, tc.description)
			assert.Equal(t, tc.expectedIdentity, info.Identity, tc.description)
		})
	}
}

// TestProcessArgsAndFlags_IdentityFlagPacker tests identity flag parsing for packer commands.
func TestProcessArgsAndFlags_IdentityFlagPacker(t *testing.T) {
	tests := []struct {
		name             string
		args             []string
		expectedIdentity string
		description      string
	}{
		{
			name:             "--identity without value",
			args:             []string{"build", "example", "--stack", "test-stack", "--identity"},
			expectedIdentity: cfg.IdentityFlagSelectValue,
			description:      "Interactive selection mode",
		},
		{
			name:             "--identity with value",
			args:             []string{"build", "example", "--stack", "test-stack", "--identity", "test-identity"},
			expectedIdentity: "test-identity",
			description:      "Explicit identity specified",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			info, err := processArgsAndFlags("packer", tc.args)
			require.NoError(t, err, tc.description)
			assert.Equal(t, tc.expectedIdentity, info.Identity, tc.description)
		})
	}
}
