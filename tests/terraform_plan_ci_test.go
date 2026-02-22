package tests

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/tests/testhelpers"
)

// TestTerraformPlanCI verifies that `atmos terraform plan mycomponent -s prod --ci`
// exits with code 0 and produces CI check run output on stderr.
func TestTerraformPlanCI(t *testing.T) {
	// Skip if terraform is not installed.
	RequireTerraform(t)

	// Build the atmos binary.
	runner := testhelpers.NewAtmosRunner("")
	require.NoError(t, runner.Build(), "Failed to build atmos binary")
	t.Cleanup(runner.Cleanup)

	// Resolve the fixture directory.
	repoRoot, err := testhelpers.FindRepoRoot()
	require.NoError(t, err)
	fixtureDir := filepath.Join(repoRoot, "tests", "fixtures", "scenarios", "native-ci")

	// Run `atmos terraform plan mycomponent -s prod --ci`.
	cmd := runner.Command("terraform", "plan", "mycomponent", "-s", "prod", "--ci")
	cmd.Dir = fixtureDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	require.NoError(t, err, "Expected exit code 0.\nStdout: %s\nStderr: %s", stdout.String(), stderr.String())

	// Verify CI check run messages appear on stderr.
	stderrOutput := stderr.String()
	assert.Contains(t, stderrOutput, "Check run created: atmos/plan: prod/mycomponent",
		"Expected check run creation message on stderr")
	assert.Contains(t, stderrOutput, "Check run completed: atmos/plan: prod/mycomponent",
		"Expected check run completion message on stderr")
}

// TestTerraformPlanCIUploadAndPlanfileList verifies that running
// `atmos terraform plan mycomponent -s prod --ci` uploads the planfile
// to the local store, and `atmos terraform planfile list` shows the record.
func TestTerraformPlanCIUploadAndPlanfileList(t *testing.T) {
	// Unset GITHUB_ACTIONS to avoid using the GitHub Actions provider for atmos CI hooks
	os.Unsetenv("GITHUB_ACTIONS")

	// Skip if terraform is not installed.
	RequireTerraform(t)

	// Build the atmos binary.
	runner := testhelpers.NewAtmosRunner("")
	require.NoError(t, runner.Build(), "Failed to build atmos binary")
	t.Cleanup(runner.Cleanup)

	// Resolve the fixture directory.
	repoRoot, err := testhelpers.FindRepoRoot()
	require.NoError(t, err)
	fixtureDir := filepath.Join(repoRoot, "tests", "fixtures", "scenarios", "native-ci")

	// Set up a sandbox so we don't modify the fixture.
	sandbox, err := testhelpers.SetupSandbox(t, fixtureDir)
	require.NoError(t, err)
	t.Cleanup(sandbox.Cleanup)

	// Build environment with sandbox overrides.
	sandboxEnv := sandbox.GetEnvironmentVariables()

	// Clean up any existing planfiles from previous runs.
	planfileStore := filepath.Join(fixtureDir, ".atmos", "planfiles")
	_ = os.RemoveAll(planfileStore)

	// Step 1: Run `atmos terraform plan mycomponent -s prod --ci`.
	planCmd := runner.Command("terraform", "plan", "mycomponent", "-s", "prod", "--ci")
	planCmd.Dir = fixtureDir
	for k, v := range sandboxEnv {
		planCmd.Env = append(planCmd.Env, k+"="+v)
	}
	var planStdout, planStderr bytes.Buffer
	planCmd.Stdout = &planStdout
	planCmd.Stderr = &planStderr

	err = planCmd.Run()
	require.NoError(t, err, "atmos terraform plan failed.\nStdout: %s\nStderr: %s", planStdout.String(), planStderr.String())

	// Step 2: Verify the planfile was created in the component working dir.
	componentsDir := sandboxEnv["ATMOS_COMPONENTS_TERRAFORM_BASE_PATH"]
	if componentsDir == "" {
		componentsDir = filepath.Join(fixtureDir, "components", "terraform")
	}
	planfilePattern := filepath.Join(componentsDir, "mock", "prod-mycomponent.planfile")
	_, err = os.Stat(planfilePattern)
	assert.NoError(t, err, "Planfile should exist in the component working directory: %s", planfilePattern)

	// Step 3: Check if planfile was uploaded to the store by listing.
	listCmd := runner.Command("terraform", "planfile", "list")
	listCmd.Dir = fixtureDir
	for k, v := range sandboxEnv {
		listCmd.Env = append(listCmd.Env, k+"="+v)
	}
	var listStdout, listStderr bytes.Buffer
	listCmd.Stdout = &listStdout
	listCmd.Stderr = &listStderr

	err = listCmd.Run()
	require.NoError(t, err, "atmos terraform planfile list failed.\nStdout: %s\nStderr: %s", listStdout.String(), listStderr.String())

	listOutput := listStdout.String()

	// The CI upload should have stored the planfile in the local store.
	// If --ci properly uploads, we expect a record containing the component/stack info.
	assert.True(t, strings.Contains(listOutput, "tfplan") || strings.Contains(listOutput, "prod"),
		"Expected planfile list to contain a record after terraform plan --ci.\nStdout: %s\nStderr: %s",
		listOutput, listStderr.String())
}
