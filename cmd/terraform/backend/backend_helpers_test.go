package backend

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
)

// TestParseCommonFlags tests the ParseCommonFlags function.
// Note: This test mutates the global Viper instance because ParseCommonFlags
// uses viper.GetViper() internally (standard pattern for CLI flag handling).
// The setupViperForTest helper ensures proper cleanup via t.Cleanup().
func TestParseCommonFlags(t *testing.T) {
	tests := []struct {
		name        string
		stack       string
		identity    string
		expectError bool
		expectedErr error
	}{
		{
			name:        "valid stack and identity",
			stack:       "dev",
			identity:    "test-identity",
			expectError: false,
		},
		{
			name:        "valid stack without identity",
			stack:       "prod",
			identity:    "",
			expectError: false,
		},
		{
			name:        "missing stack",
			stack:       "",
			identity:    "test-identity",
			expectError: true,
			expectedErr: errUtils.ErrRequiredFlagNotProvided,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use helper to safely set up and restore global Viper state.
			setupViperForTest(t, map[string]any{
				"stack":    tt.stack,
				"identity": tt.identity,
			})

			// Create a test command.
			cmd := &cobra.Command{
				Use: "test",
			}

			// Create parser with common flags.
			parser := flags.NewStandardParser(
				flags.WithStringFlag("stack", "s", "", "Stack name"),
				flags.WithStringFlag("identity", "i", "", "Identity"),
			)

			// Register flags with command.
			parser.RegisterFlags(cmd)

			// Bind to viper.
			err := parser.BindToViper(viper.GetViper())
			require.NoError(t, err)

			// Parse common flags.
			opts, err := ParseCommonFlags(cmd, parser)

			if tt.expectError {
				assert.Error(t, err)
				if tt.expectedErr != nil {
					assert.ErrorIs(t, err, tt.expectedErr)
				}
				assert.Nil(t, opts)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, opts)
				assert.Equal(t, tt.stack, opts.Stack)
				assert.Equal(t, tt.identity, opts.Identity)
			}
		})
	}
}

func TestCreateDescribeComponentFunc(t *testing.T) {
	t.Run("creates function with nil auth", func(t *testing.T) {
		// Create the describe function with nil auth manager.
		describeFunc := CreateDescribeComponentFunc(nil)

		// Verify it returns a non-nil function.
		assert.NotNil(t, describeFunc)

		// Note: We cannot test the actual execution without mocking ExecuteDescribeComponent.
		// This would require significant test infrastructure.
		// This test verifies the function creation logic works correctly.
	})
}

func TestCommonOptions(t *testing.T) {
	t.Run("CommonOptions struct initialization", func(t *testing.T) {
		opts := &CommonOptions{
			Stack:    "test-stack",
			Identity: "test-identity",
		}

		assert.Equal(t, "test-stack", opts.Stack)
		assert.Equal(t, "test-identity", opts.Identity)
	})
}
