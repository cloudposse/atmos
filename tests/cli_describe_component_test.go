package tests

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/cmd"
	"github.com/cloudposse/atmos/tests/testhelpers"
)

func TestExecuteDescribeComponentCmd_Success_YAMLWithPager(t *testing.T) {
	_ = testhelpers.NewRootCmdTestKit(t) // MANDATORY: Clean up RootCmd state.

	// Get absolute path to test fixture directory.
	fixtureDir, err := filepath.Abs("./fixtures/scenarios/atmos-include-yaml-function")
	require.NoError(t, err, "Failed to get absolute path for fixture directory")

	// Set ATMOS_CLI_CONFIG_PATH and ATMOS_BASE_PATH to point to test fixture directory.
	// This prevents Atmos from searching up the directory tree and finding parent config.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", fixtureDir)
	t.Setenv("ATMOS_BASE_PATH", fixtureDir)

	// Use SetArgs for Cobra command testing.
	cmd.RootCmd.SetArgs([]string{"describe", "component", "component-1", "--stack", "nonprod", "--pager=more", "--format", "yaml"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("failed to execute command: %v", err)
	}
}
