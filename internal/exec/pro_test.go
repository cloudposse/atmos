package exec

import (
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestParseLockCliArgs_MissingFlags(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("component", "", "")
	cmd.Flags().String("stack", "", "")
	cmd.Flags().String("lock-message", "", "")
	cmd.Flags().String("lock-ttl", "", "")

	// Test missing component
	err := cmd.Flags().Set("stack", "test-stack")
	assert.NoError(t, err)

	result, err := parseLockCliArgs(cmd, []string{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "both '--component' and '--stack' flag must be provided")
	assert.Empty(t, result)

	// Test missing stack
	cmd.Flags().Set("component", "test-component")
	cmd.Flags().Set("stack", "")

	result, err = parseLockCliArgs(cmd, []string{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "both '--component' and '--stack' flag must be provided")
	assert.Empty(t, result)
}

func TestParseLockCliArgs_ValidInput(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("component", "", "")
	cmd.Flags().String("stack", "", "")
	cmd.Flags().String("lock-message", "", "")
	cmd.Flags().String("lock-ttl", "", "")

	err := cmd.Flags().Set("component", "test-component")
	assert.NoError(t, err)
	err = cmd.Flags().Set("stack", "test-stack")
	assert.NoError(t, err)
	err = cmd.Flags().Set("lock-message", "test message")
	assert.NoError(t, err)
	err = cmd.Flags().Set("lock-ttl", "300")
	assert.NoError(t, err)

	result, err := parseLockCliArgs(cmd, []string{})
	assert.NoError(t, err)
	assert.Equal(t, "test-component", result.Component)
	assert.Equal(t, "test-stack", result.Stack)
	assert.Equal(t, "test message", result.LockMessage)
	assert.Equal(t, int32(300), result.LockTTL)
	assert.NotNil(t, result.Logger)
}

func TestParseUnlockCliArgs_MissingFlags(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("component", "", "")
	cmd.Flags().String("stack", "", "")

	// Test missing component
	err := cmd.Flags().Set("stack", "test-stack")
	assert.NoError(t, err)

	result, err := parseUnlockCliArgs(cmd, []string{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "both '--component' and '--stack' flag must be provided")
	assert.Empty(t, result)

	// Test missing stack
	cmd.Flags().Set("component", "test-component")
	cmd.Flags().Set("stack", "")

	result, err = parseUnlockCliArgs(cmd, []string{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "both '--component' and '--stack' flag must be provided")
	assert.Empty(t, result)
}

func TestParseUnlockCliArgs_ValidInput(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("component", "", "")
	cmd.Flags().String("stack", "", "")

	err := cmd.Flags().Set("component", "test-component")
	assert.NoError(t, err)
	err = cmd.Flags().Set("stack", "test-stack")
	assert.NoError(t, err)

	result, err := parseUnlockCliArgs(cmd, []string{})
	assert.NoError(t, err)
	assert.Equal(t, "test-component", result.Component)
	assert.Equal(t, "test-stack", result.Stack)
	assert.NotNil(t, result.Logger)
}

func TestProLockUnlockCmdArgs(t *testing.T) {
	// Test struct initialization
	cmdArgs := ProLockUnlockCmdArgs{
		Component:   "test-component",
		Stack:       "test-stack",
		Logger:      nil,
		AtmosConfig: schema.AtmosConfiguration{},
	}

	assert.Equal(t, "test-component", cmdArgs.Component)
	assert.Equal(t, "test-stack", cmdArgs.Stack)
}
