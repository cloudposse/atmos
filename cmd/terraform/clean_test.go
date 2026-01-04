package terraform

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCleanCommandSetup verifies that the clean command is properly configured.
func TestCleanCommandSetup(t *testing.T) {
	// Verify command is registered.
	require.NotNil(t, cleanCmd)

	// Verify it's attached to terraformCmd.
	found := false
	for _, cmd := range terraformCmd.Commands() {
		if cmd.Name() == "clean" {
			found = true
			break
		}
	}
	assert.True(t, found, "clean should be registered as a subcommand of terraformCmd")

	// Verify command short and long descriptions.
	assert.Contains(t, cleanCmd.Short, "Clean")
	assert.Contains(t, cleanCmd.Long, "Terraform")
}

// TestCleanParserSetup verifies that the clean parser is properly configured.
func TestCleanParserSetup(t *testing.T) {
	require.NotNil(t, cleanParser, "cleanParser should be initialized")

	// Verify the parser has the clean-specific flags.
	registry := cleanParser.Registry()

	expectedFlags := []string{
		"everything",
		"force",
		"skip-lock-file",
		"cache",
	}

	for _, flagName := range expectedFlags {
		assert.True(t, registry.Has(flagName), "cleanParser should have %s flag registered", flagName)
	}
}

// TestCleanFlagSetup verifies that clean command has correct flags registered.
func TestCleanFlagSetup(t *testing.T) {
	// Verify clean-specific flags are registered on the command.
	cleanFlags := []string{
		"everything",
		"force",
		"skip-lock-file",
		"cache",
	}

	for _, flagName := range cleanFlags {
		flag := cleanCmd.Flags().Lookup(flagName)
		assert.NotNil(t, flag, "%s flag should be registered on clean command", flagName)
	}
}

// TestCleanFlagDefaults verifies that clean command flags have correct default values.
func TestCleanFlagDefaults(t *testing.T) {
	v := viper.New()

	// Bind parser to fresh viper instance.
	err := cleanParser.BindToViper(v)
	require.NoError(t, err)

	// Verify default values.
	assert.False(t, v.GetBool("everything"), "everything should default to false")
	assert.False(t, v.GetBool("force"), "force should default to false")
	assert.False(t, v.GetBool("skip-lock-file"), "skip-lock-file should default to false")
	assert.False(t, v.GetBool("cache"), "cache should default to false")
}

// TestCleanFlagEnvVars verifies that clean command flags have environment variable bindings.
func TestCleanFlagEnvVars(t *testing.T) {
	registry := cleanParser.Registry()

	// Expected env var bindings.
	expectedEnvVars := map[string]string{
		"everything":     "ATMOS_TERRAFORM_CLEAN_EVERYTHING",
		"force":          "ATMOS_TERRAFORM_CLEAN_FORCE",
		"skip-lock-file": "ATMOS_TERRAFORM_CLEAN_SKIP_LOCK_FILE",
		"cache":          "ATMOS_TERRAFORM_CLEAN_CACHE",
	}

	for flagName, expectedEnvVar := range expectedEnvVars {
		require.True(t, registry.Has(flagName), "cleanParser should have %s flag registered", flagName)
		flag := registry.Get(flagName)
		require.NotNil(t, flag, "cleanParser should have info for %s flag", flagName)
		envVars := flag.GetEnvVars()
		assert.Contains(t, envVars, expectedEnvVar, "%s should be bound to %s", flagName, expectedEnvVar)
	}
}

// TestCleanCommandArgs verifies that clean command accepts the correct number of arguments.
func TestCleanCommandArgs(t *testing.T) {
	// The command should accept 0 or 1 argument (component name is optional).
	require.NotNil(t, cleanCmd.Args)

	// Verify with no args.
	err := cleanCmd.Args(cleanCmd, []string{})
	assert.NoError(t, err, "clean command should accept 0 arguments")

	// Verify with one arg.
	err = cleanCmd.Args(cleanCmd, []string{"my-component"})
	assert.NoError(t, err, "clean command should accept 1 argument")

	// Verify with two args (should fail).
	err = cleanCmd.Args(cleanCmd, []string{"arg1", "arg2"})
	assert.Error(t, err, "clean command should reject more than 1 argument")
}
