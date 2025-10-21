package testhelpers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestNewMockCommand verifies that NewMockCommand creates a command with string flags.
func TestNewMockCommand(t *testing.T) {
	flags := map[string]string{
		"stack":  "dev",
		"format": "yaml",
	}

	cmd := NewMockCommand("test", flags)

	assert.NotNil(t, cmd)
	assert.Equal(t, "test", cmd.Use)

	// Verify flags were set.
	stackValue, err := cmd.Flags().GetString("stack")
	assert.NoError(t, err)
	assert.Equal(t, "dev", stackValue)

	formatValue, err := cmd.Flags().GetString("format")
	assert.NoError(t, err)
	assert.Equal(t, "yaml", formatValue)
}

// TestNewMockCommand_EmptyFlags verifies NewMockCommand works with no flags.
func TestNewMockCommand_EmptyFlags(t *testing.T) {
	cmd := NewMockCommand("test", map[string]string{})

	assert.NotNil(t, cmd)
	assert.Equal(t, "test", cmd.Use)
}

// TestNewMockCommandWithBool verifies that NewMockCommandWithBool creates a command with boolean flags.
func TestNewMockCommandWithBool(t *testing.T) {
	boolFlags := map[string]bool{
		"verbose": true,
		"quiet":   false,
	}

	cmd := NewMockCommandWithBool("test", boolFlags)

	assert.NotNil(t, cmd)
	assert.Equal(t, "test", cmd.Use)

	// Verify boolean flags were set.
	verboseValue, err := cmd.Flags().GetBool("verbose")
	assert.NoError(t, err)
	assert.True(t, verboseValue)

	quietValue, err := cmd.Flags().GetBool("quiet")
	assert.NoError(t, err)
	assert.False(t, quietValue)
}

// TestNewMockCommandWithMixed verifies that NewMockCommandWithMixed creates a command with both string and boolean flags.
func TestNewMockCommandWithMixed(t *testing.T) {
	stringFlags := map[string]string{
		"stack": "prod",
	}
	boolFlags := map[string]bool{
		"verbose": true,
	}

	cmd := NewMockCommandWithMixed("test", stringFlags, boolFlags)

	assert.NotNil(t, cmd)
	assert.Equal(t, "test", cmd.Use)

	// Verify string flags.
	stackValue, err := cmd.Flags().GetString("stack")
	assert.NoError(t, err)
	assert.Equal(t, "prod", stackValue)

	// Verify boolean flags.
	verboseValue, err := cmd.Flags().GetBool("verbose")
	assert.NoError(t, err)
	assert.True(t, verboseValue)
}

// TestNewMockCommandWithMixed_EmptyFlags verifies NewMockCommandWithMixed works with no flags.
func TestNewMockCommandWithMixed_EmptyFlags(t *testing.T) {
	cmd := NewMockCommandWithMixed("test", map[string]string{}, map[string]bool{})

	assert.NotNil(t, cmd)
	assert.Equal(t, "test", cmd.Use)
}
