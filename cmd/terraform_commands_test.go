package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetTerraformCommands(t *testing.T) {
	commands := getTerraformCommands()

	// Test that we have commands
	require.NotEmpty(t, commands, "should return terraform commands")

	// Test specific expected commands
	expectedCommands := []string{
		"plan",
		"apply",
		"destroy",
		"init",
		"validate",
		"workspace",
		"clean",
		"deploy",
		"shell",
		"version",
	}

	commandMap := make(map[string]bool)
	for _, cmd := range commands {
		commandMap[cmd.Use] = true
	}

	for _, expectedCmd := range expectedCommands {
		assert.True(t, commandMap[expectedCmd], "should contain %s command", expectedCmd)
	}

	// Test that commands have required fields
	for _, cmd := range commands {
		assert.NotEmpty(t, cmd.Use, "command should have Use field")
		assert.NotEmpty(t, cmd.Short, "command %s should have Short description", cmd.Use)
	}
}

func TestAttachTerraformCommands(t *testing.T) {
	// Create a test parent command
	parentCmd := terraformCmd

	// Verify persistent flags are set
	flags := parentCmd.PersistentFlags()

	// Test some important flags
	flag := flags.Lookup("append-user-agent")
	assert.NotNil(t, flag, "should have append-user-agent flag")

	flag = flags.Lookup("skip-init")
	assert.NotNil(t, flag, "should have skip-init flag")

	flag = flags.Lookup("process-templates")
	assert.NotNil(t, flag, "should have process-templates flag")

	flag = flags.Lookup("process-functions")
	assert.NotNil(t, flag, "should have process-functions flag")

	flag = flags.Lookup("query")
	assert.NotNil(t, flag, "should have query flag")

	flag = flags.Lookup("components")
	assert.NotNil(t, flag, "should have components flag")

	flag = flags.Lookup("dry-run")
	assert.NotNil(t, flag, "should have dry-run flag")

	// Test affected-related flags
	flag = flags.Lookup("repo-path")
	assert.NotNil(t, flag, "should have repo-path flag")

	flag = flags.Lookup("ref")
	assert.NotNil(t, flag, "should have ref flag")

	flag = flags.Lookup("include-dependents")
	assert.NotNil(t, flag, "should have include-dependents flag")

	// Verify subcommands are attached
	assert.True(t, parentCmd.HasSubCommands(), "parent command should have subcommands")

	// Test that plan command has expected flags
	planCmd, _, err := parentCmd.Find([]string{"plan"})
	require.NoError(t, err)
	assert.Equal(t, "plan", planCmd.Use)

	// Test that apply command has expected flags
	applyCmd, _, err := parentCmd.Find([]string{"apply"})
	require.NoError(t, err)
	assert.Equal(t, "apply", applyCmd.Use)
}

func TestCommandMaps(t *testing.T) {
	// Test that commandMaps has expected entries
	assert.Contains(t, commandMaps, "plan")
	assert.Contains(t, commandMaps, "apply")
	assert.Contains(t, commandMaps, "deploy")
	assert.Contains(t, commandMaps, "clean")
	assert.Contains(t, commandMaps, "plan-diff")
}
