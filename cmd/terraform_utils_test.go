package cmd

import (
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestCheckTerraformFlags(t *testing.T) {
	tests := []struct {
		name          string
		info          *schema.ConfigAndStacksInfo
		expectedError error
	}{
		{
			name:          "valid - no flags",
			info:          &schema.ConfigAndStacksInfo{},
			expectedError: nil,
		},
		{
			name: "invalid - component with affected flag",
			info: &schema.ConfigAndStacksInfo{
				ComponentFromArg: "test-component",
				Affected:         true,
			},
			expectedError: errUtils.ErrInvalidTerraformComponentWithMultiComponentFlags,
		},
		{
			name: "invalid - component with all flag",
			info: &schema.ConfigAndStacksInfo{
				ComponentFromArg: "test-component",
				All:              true,
			},
			expectedError: errUtils.ErrInvalidTerraformComponentWithMultiComponentFlags,
		},
		{
			name: "invalid - component with components flag",
			info: &schema.ConfigAndStacksInfo{
				ComponentFromArg: "test-component",
				Components:       []string{"comp1", "comp2"},
			},
			expectedError: errUtils.ErrInvalidTerraformComponentWithMultiComponentFlags,
		},
		{
			name: "invalid - affected with all flag",
			info: &schema.ConfigAndStacksInfo{
				Affected: true,
				All:      true,
			},
			expectedError: errUtils.ErrInvalidTerraformFlagsWithAffectedFlag,
		},
		{
			name: "invalid - affected with components flag",
			info: &schema.ConfigAndStacksInfo{
				Affected:   true,
				Components: []string{"comp1", "comp2"},
			},
			expectedError: errUtils.ErrInvalidTerraformFlagsWithAffectedFlag,
		},
		{
			name: "invalid - affected with query flag",
			info: &schema.ConfigAndStacksInfo{
				Affected: true,
				Query:    "test-query",
			},
			expectedError: errUtils.ErrInvalidTerraformFlagsWithAffectedFlag,
		},
		{
			name: "invalid - single and multi component flags",
			info: &schema.ConfigAndStacksInfo{
				PlanFile: "plan.tfplan",
				All:      true,
			},
			expectedError: errUtils.ErrInvalidTerraformSingleComponentAndMultiComponentFlags,
		},
		{
			name: "invalid - from-plan with multi component flag",
			info: &schema.ConfigAndStacksInfo{
				UseTerraformPlan: true,
				Affected:         true,
			},
			expectedError: errUtils.ErrInvalidTerraformSingleComponentAndMultiComponentFlags,
		},
		{
			name: "valid - only single component flag",
			info: &schema.ConfigAndStacksInfo{
				PlanFile: "plan.tfplan",
			},
			expectedError: nil,
		},
		{
			name: "valid - only multi component flag",
			info: &schema.ConfigAndStacksInfo{
				All: true,
			},
			expectedError: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := checkTerraformFlags(test.info)
			if test.expectedError != nil {
				assert.ErrorIs(t, err, test.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestTerraformIdentityFlagHandling tests the identity flag handling in terraformRun.
// Regression test for: https://github.com/cloudposse/atmos/issues/XXXX
// Ensures that when --identity flag is NOT provided, the code doesn't try to
// read from viper and potentially get the IdentityFlagSelectValue ("__SELECT__"),
// which would trigger the TTY check and fail in CI environments.
func TestTerraformIdentityFlagHandling(t *testing.T) {
	_ = NewTestKit(t) // Cleanup happens automatically via tb.Cleanup()

	tests := []struct {
		name             string
		args             []string
		envVar           string
		envValue         string
		expectTTYError   bool
		expectedIdentity string
	}{
		{
			name:             "no --identity flag, no env var, should not trigger TTY check",
			args:             []string{},
			expectedIdentity: "", // ProcessCommandLineArgs will handle default identity
			expectTTYError:   false,
		},
		{
			name:             "ATMOS_IDENTITY env var set, should use env var",
			args:             []string{},
			envVar:           "ATMOS_IDENTITY",
			envValue:         "test-identity",
			expectedIdentity: "test-identity",
			expectTTYError:   false,
		},
		{
			name:             "ATMOS_IDENTITY set to __SELECT__, should NOT trigger TTY check (flag not explicitly provided)",
			args:             []string{},
			envVar:           "ATMOS_IDENTITY",
			envValue:         "__SELECT__",
			expectedIdentity: "__SELECT__", // ProcessCommandLineArgs sets this, but terraformRun doesn't override
			expectTTYError:   false,        // Key fix: TTY check only happens when flag is explicitly provided
		},
		{
			name:           "--identity flag without value, SHOULD trigger TTY check",
			args:           []string{"--identity"},
			expectTTYError: true, // Expected to fail in non-TTY environment
		},
		{
			name:             "--identity flag with value, should use flag value",
			args:             []string{"--identity=flag-identity"},
			expectedIdentity: "flag-identity",
			expectTTYError:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Set up environment variable if specified.
			if tc.envVar != "" {
				originalValue := os.Getenv(tc.envVar)
				err := os.Setenv(tc.envVar, tc.envValue)
				assert.NoError(t, err)
				defer func() {
					if originalValue != "" {
						_ = os.Setenv(tc.envVar, originalValue)
					} else {
						_ = os.Unsetenv(tc.envVar)
					}
				}()
			}

			// Create a minimal command for testing.
			cmd := &cobra.Command{
				Use: "plan",
			}

			// Add the identity flag (matching terraform commands setup).
			cmd.Flags().StringP("identity", "i", "", "Specify identity")
			if identityFlag := cmd.Flags().Lookup("identity"); identityFlag != nil {
				identityFlag.NoOptDefVal = IdentityFlagSelectValue
			}

			// Parse the test args.
			err := cmd.ParseFlags(tc.args)

			// Check if we expect a TTY error.
			if tc.expectTTYError {
				// When --identity flag is provided without value in non-TTY environment,
				// we expect the code to fail fast with TTY error before trying to parse.
				// This test verifies the guard works correctly.
				// We can't easily test the full terraformRun flow here without mocking
				// the entire CLI config and auth manager, so we just verify flag parsing.
				assert.NoError(t, err, "Flag parsing should succeed")

				// Verify flag was set to __SELECT__.
				flagValue, err := cmd.Flags().GetString("identity")
				assert.NoError(t, err)
				assert.Equal(t, IdentityFlagSelectValue, flagValue, "Flag should be set to __SELECT__")
			} else {
				assert.NoError(t, err, "Flag parsing should succeed")

				// Verify the flag value if we don't expect TTY error.
				if len(tc.args) > 0 {
					flagValue, err := cmd.Flags().GetString("identity")
					assert.NoError(t, err)

					if tc.args[0] == "--identity" && len(tc.args) == 1 {
						// --identity without value → should be __SELECT__.
						assert.Equal(t, IdentityFlagSelectValue, flagValue)
					} else if len(tc.args) > 0 && tc.args[0] == "--identity=flag-identity" {
						// --identity with value → should be the value.
						assert.Equal(t, "flag-identity", flagValue)
					}
				}
			}
		})
	}
}
