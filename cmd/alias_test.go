package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommandAliases(t *testing.T) {
	_ = NewTestKit(t)

	testDir := "../tests/fixtures/scenarios/subcommand-alias"

	// Change to test directory (t.Chdir automatically restores on cleanup).
	t.Chdir(testDir)

	// Load the atmos config to trigger alias registration.
	RootCmd.SetArgs([]string{"version"})
	err := Execute()
	require.NoError(t, err)

	tests := []struct {
		name      string
		aliasName string
		aliasFor  string
	}{
		{
			name:      "terraform plan alias 'tp'",
			aliasName: "tp",
			aliasFor:  "terraform plan",
		},
		{
			name:      "terraform alias 'tr'",
			aliasName: "tr",
			aliasFor:  "terraform",
		},
		{
			name:      "terraform apply alias 'ta'",
			aliasName: "ta",
			aliasFor:  "terraform apply",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = NewTestKit(t)

			// Verify the alias command exists.
			cmd, _, err := RootCmd.Find([]string{tt.aliasName})
			require.NoError(t, err, "%s alias should be registered", tt.aliasName)
			assert.Equal(t, tt.aliasName, cmd.Use, "%s command should exist", tt.aliasName)
			assert.Contains(t, cmd.Short, "alias for", "%s should be an alias command", tt.aliasName)
		})
	}
}

func TestDevcontainerAliases(t *testing.T) {
	_ = NewTestKit(t)

	testDir := "../examples/devcontainer"

	// Change to test directory (t.Chdir automatically restores on cleanup).
	t.Chdir(testDir)

	// Load the atmos config to trigger alias registration.
	RootCmd.SetArgs([]string{"version"})
	err := Execute()
	require.NoError(t, err)

	// Verify the 'shell' alias command exists.
	shellCmd, _, err := RootCmd.Find([]string{"shell"})
	require.NoError(t, err, "shell alias should be registered")
	assert.Equal(t, "shell", shellCmd.Use, "shell command should exist")
	assert.Contains(t, shellCmd.Short, "alias for", "shell should be an alias command")
}
