package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestCustomCommandWithIdentity(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode: spawns actual shell process")
	}

	// Set up test fixture with auth configuration.
	testDir := "../tests/fixtures/scenarios/atmos-auth-mock"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	// Load atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	// Verify that custom commands with identity were loaded.
	assert.NotEmpty(t, atmosConfig.Commands)

	foundTestIdentityCmd := false
	foundTestIdentityMultiCmd := false

	for _, cmd := range atmosConfig.Commands {
		if cmd.Name == "test-identity" {
			foundTestIdentityCmd = true
			assert.Equal(t, "mock-identity", cmd.Identity)
			assert.NotEmpty(t, cmd.Steps)
		}
		if cmd.Name == "test-identity-multi" {
			foundTestIdentityMultiCmd = true
			assert.Equal(t, "mock-identity-2", cmd.Identity)
			assert.Len(t, cmd.Steps, 2)
		}
	}

	assert.True(t, foundTestIdentityCmd, "test-identity command should be loaded")
	assert.True(t, foundTestIdentityMultiCmd, "test-identity-multi command should be loaded")
}

func TestCustomCommandIdentityAuthentication(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode: requires auth system")
	}

	// Set up test fixture with auth configuration.
	testDir := "../tests/fixtures/scenarios/atmos-auth-mock"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	// Create a test kit to ensure clean RootCmd state.
	_ = NewTestKit(t)

	// Load atmos configuration to register custom commands.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	// Process custom commands to register them with RootCmd.
	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd, true)
	require.NoError(t, err)

	// Find the registered test-identity command.
	var testIdentityCmd *schema.Command
	for _, cmd := range atmosConfig.Commands {
		if cmd.Name == "test-identity" {
			testIdentityCmd = &cmd
			break
		}
	}
	require.NotNil(t, testIdentityCmd, "test-identity command should be registered")

	// Verify that identity field is correctly set.
	assert.Equal(t, "mock-identity", testIdentityCmd.Identity)
}

func TestCustomCommandIdentityFlagOverride(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode: requires auth system")
	}

	// Set up test fixture with auth configuration.
	testDir := "../tests/fixtures/scenarios/atmos-auth-mock"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	// Create a test kit to ensure clean RootCmd state.
	_ = NewTestKit(t)

	// Load atmos configuration to register custom commands.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	// Process custom commands to register them with RootCmd.
	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd, true)
	require.NoError(t, err)

	// Find the registered test-identity command.
	var cobraCmd *cobra.Command
	for _, cmd := range RootCmd.Commands() {
		if cmd.Name() == "test-identity" {
			cobraCmd = cmd
			break
		}
	}
	require.NotNil(t, cobraCmd, "test-identity command should be registered as a Cobra command")

	// Verify that --identity flag is automatically added by the flag parser.
	// With the unified flag parser, flags are registered on Flags() not PersistentFlags().
	identityFlag := cobraCmd.Flags().Lookup("identity")
	require.NotNil(t, identityFlag, "--identity flag should be automatically added to custom commands")
	assert.Equal(t, "string", identityFlag.Value.Type())
	assert.Contains(t, identityFlag.Usage, "Identity to use for authentication")
}
