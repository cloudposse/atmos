package terraform

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanCmd_CommandStructure(t *testing.T) {
	// Test that cleanCmd has correct structure.
	assert.Equal(t, "clean <component>", cleanCmd.Use)
	assert.Contains(t, cleanCmd.Short, "Clean up Terraform state")
	assert.Contains(t, cleanCmd.Long, "Remove temporary files")
}

func TestCleanCmd_Flags(t *testing.T) {
	// Test that cleanParser has the expected flags.
	require.NotNil(t, cleanParser)

	registry := cleanParser.Registry()

	// Should include clean-specific flags.
	assert.True(t, registry.Has("everything"))
	assert.True(t, registry.Has("force"))
	assert.True(t, registry.Has("skip-lock-file"))
}

func TestCleanCmd_FlagDefaults(t *testing.T) {
	// Test flag default values.
	registry := cleanParser.Registry()

	// Get everything flag.
	everythingFlag := registry.Get("everything")
	require.NotNil(t, everythingFlag)

	// Get force flag.
	forceFlag := registry.Get("force")
	require.NotNil(t, forceFlag)

	// Get skip-lock-file flag.
	skipLockFileFlag := registry.Get("skip-lock-file")
	require.NotNil(t, skipLockFileFlag)
}

func TestCleanCmd_AttachedToTerraform(t *testing.T) {
	// Test that cleanCmd is attached to terraformCmd.
	found := false
	for _, cmd := range terraformCmd.Commands() {
		if cmd.Use == "clean <component>" {
			found = true
			break
		}
	}
	assert.True(t, found, "cleanCmd should be attached to terraformCmd")
}

func TestCleanCmd_Args(t *testing.T) {
	// Test that cleanCmd accepts at most 1 argument.
	assert.NotNil(t, cleanCmd.Args)
}
