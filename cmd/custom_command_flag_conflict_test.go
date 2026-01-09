package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestCustomCommandFlagConflictWithGlobalFlag(t *testing.T) {
	// Test that custom commands with conflicting flag names are rejected gracefully.

	_ = NewTestKit(t) // Clean RootCmd state

	atmosConfig := schema.AtmosConfiguration{
		Commands: []schema.Command{
			{
				Name:        "test-conflict",
				Description: "Test command with conflicting flag",
				Flags: []schema.CommandFlag{
					{
						Name:  "verbose", // Conflicts with global --verbose
						Usage: "Test flag",
					},
				},
				Steps: schema.Tasks{{Command: "echo test"}},
			},
		},
	}

	err := processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd, true)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrReservedFlagName)
	// The error contains the flag name in the formatted output.
	formattedErr := errUtils.Format(err, errUtils.DefaultFormatterConfig())
	assert.Contains(t, formattedErr, "verbose")
}

func TestCustomCommandShorthandConflictWithGlobalFlag(t *testing.T) {
	// Test that custom commands with conflicting shorthands are rejected.

	_ = NewTestKit(t)

	atmosConfig := schema.AtmosConfiguration{
		Commands: []schema.Command{
			{
				Name:        "test-shorthand-conflict",
				Description: "Test command with conflicting shorthand",
				Flags: []schema.CommandFlag{
					{
						Name:      "verbose-mode",
						Shorthand: "C", // Conflicts with global -C (chdir)
						Usage:     "Test flag",
					},
				},
				Steps: schema.Tasks{{Command: "echo test"}},
			},
		},
	}

	err := processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd, true)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrReservedFlagName)
	// The error contains the shorthand info in the formatted output.
	formattedErr := errUtils.Format(err, errUtils.DefaultFormatterConfig())
	assert.Contains(t, formattedErr, "shorthand")
}

func TestCustomCommandValidFlagsWork(t *testing.T) {
	// Ensure valid custom command flags still work.

	_ = NewTestKit(t)

	atmosConfig := schema.AtmosConfiguration{
		Commands: []schema.Command{
			{
				Name:        "test-valid",
				Description: "Test command with valid flag",
				Flags: []schema.CommandFlag{
					{
						Name:      "my-custom-flag",
						Shorthand: "m",
						Usage:     "A valid custom flag",
					},
				},
				Steps: schema.Tasks{{Command: "echo test"}},
			},
		},
	}

	err := processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd, true)
	require.NoError(t, err)
}
