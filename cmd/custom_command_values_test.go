package cmd

import (
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestCustomCommandIntegration_ValuesRejectsInvalidFlagValue verifies a flag's `values:`
// constraint is enforced with the same error format pkg/flags' built-in-command
// `valid_values:` already produces.
func TestCustomCommandIntegration_ValuesRejectsInvalidFlagValue(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode")
	}

	testDir := "../tests/fixtures/scenarios/atmos-auth-mock"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	_ = NewTestKit(t)

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	ranFile := filepath.Join(tmpDir, "ran.txt")

	atmosConfig.Commands = []schema.Command{
		{
			Name: "vv-deploy",
			Flags: []schema.CommandFlag{
				{Name: "environment", Type: "string", Values: []string{"dev", "staging", "prod"}},
			},
			Steps: schema.Tasks{{Type: "shell", Command: "echo ran >> " + ranFile}},
		},
	}

	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd)
	require.NoError(t, err)

	var deployCmd *cobra.Command
	for _, c := range RootCmd.Commands() {
		if c.Name() == "vv-deploy" {
			deployCmd = c
			break
		}
	}
	require.NotNil(t, deployCmd, "vv-deploy command should be registered")

	require.NoError(t, deployCmd.PersistentFlags().Set("environment", "typo"))

	originalOsExit := errUtils.OsExit
	t.Cleanup(func() { errUtils.OsExit = originalOsExit })
	errUtils.OsExit = func(code int) { panic(code) }

	assert.Panics(t, func() {
		deployCmd.Run(deployCmd, []string{})
	}, "an invalid values: choice must hard-fail")

	assert.NoFileExists(t, ranFile, "steps must not run when a values: constraint is violated")
}

// TestCustomCommandIntegration_ValuesAcceptsValidFlagValue verifies a valid values: choice
// passes through untouched and the command's steps still run.
func TestCustomCommandIntegration_ValuesAcceptsValidFlagValue(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode")
	}

	testDir := "../tests/fixtures/scenarios/atmos-auth-mock"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	_ = NewTestKit(t)

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	ranFile := filepath.Join(tmpDir, "ran.txt")

	atmosConfig.Commands = []schema.Command{
		{
			Name: "vv-deploy-ok",
			Flags: []schema.CommandFlag{
				{Name: "environment", Type: "string", Values: []string{"dev", "staging", "prod"}},
			},
			Steps: schema.Tasks{{Type: "shell", Command: "echo ran >> " + ranFile}},
		},
	}

	err = processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd)
	require.NoError(t, err)

	var deployCmd *cobra.Command
	for _, c := range RootCmd.Commands() {
		if c.Name() == "vv-deploy-ok" {
			deployCmd = c
			break
		}
	}
	require.NotNil(t, deployCmd, "vv-deploy-ok command should be registered")

	require.NoError(t, deployCmd.PersistentFlags().Set("environment", "prod"))

	deployCmd.Run(deployCmd, []string{})

	assert.FileExists(t, ranFile, "steps must run when the values: choice is valid")
}
