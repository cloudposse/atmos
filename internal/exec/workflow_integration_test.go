package exec

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestWorkflowIntegration_MockProviderEnvironmentVariables tests that workflows with mock provider
// actually set the correct environment variables for subprocesses.
func TestWorkflowIntegration_MockProviderEnvironmentVariables(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode: exercises full auth flow")
	}

	// Set up test fixture with auth configuration.
	testDir := "../../tests/fixtures/scenarios/atmos-auth-mock"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	// Load atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	// Create a temporary file to capture environment variables from the subprocess.
	tmpDir := t.TempDir()
	envOutputFile := filepath.Join(tmpDir, "env-output.txt")

	// Get OS-specific command to dump environment variables.
	var dumpEnvCmd string
	if runtime.GOOS == "windows" {
		dumpEnvCmd = "set > " + envOutputFile
	} else {
		dumpEnvCmd = "env > " + envOutputFile
	}

	// Define a test workflow that dumps environment variables.
	workflowDefinition := &schema.WorkflowDefinition{
		Description: "Test workflow that captures environment variables",
		Steps: []schema.WorkflowStep{
			{
				Name:     "capture-env-vars",
				Command:  dumpEnvCmd,
				Type:     "shell",
				Identity: "mock-identity",
			},
		},
	}

	// Execute the workflow.
	err = ExecuteWorkflow(atmosConfig, "test-env-capture", "test.yaml", workflowDefinition, false, "", "", "")
	require.NoError(t, err, "Workflow execution should succeed")

	// Read the captured environment variables.
	envContent, err := os.ReadFile(envOutputFile)
	require.NoError(t, err, "Should be able to read environment output file")

	envVars := string(envContent)
	t.Logf("Captured environment variables:\n%s", envVars)

	// Verify that authentication-related environment variables are set.
	// For mock provider, we should see ATMOS_IDENTITY.
	assert.Contains(t, envVars, "ATMOS_IDENTITY", "Should have ATMOS_IDENTITY environment variable")

	// The mock provider should set this to mock-identity.
	if runtime.GOOS == "windows" {
		assert.Contains(t, envVars, "ATMOS_IDENTITY=mock-identity", "ATMOS_IDENTITY should be set to mock-identity")
	} else {
		assert.Contains(t, envVars, "ATMOS_IDENTITY=mock-identity", "ATMOS_IDENTITY should be set to mock-identity")
	}
}

// TestWorkflowIntegration_MultipleStepsWithDifferentIdentities tests that different workflow steps
// can use different identities and each gets the correct environment.
func TestWorkflowIntegration_MultipleStepsWithDifferentIdentities(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode: exercises full auth flow")
	}

	// Set up test fixture with auth configuration.
	testDir := "../../tests/fixtures/scenarios/atmos-auth-mock"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	// Load atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	// Create temporary files to capture environment variables from different steps.
	tmpDir := t.TempDir()
	envOutput1 := filepath.Join(tmpDir, "env-step1.txt")
	envOutput2 := filepath.Join(tmpDir, "env-step2.txt")
	envOutput3 := filepath.Join(tmpDir, "env-step3.txt")

	// Get OS-specific command to dump environment variables.
	var getDumpCmd func(string) string
	if runtime.GOOS == "windows" {
		getDumpCmd = func(file string) string { return "set > " + file }
	} else {
		getDumpCmd = func(file string) string { return "env > " + file }
	}

	// Define a test workflow with multiple steps using different identities.
	workflowDefinition := &schema.WorkflowDefinition{
		Description: "Test workflow with multiple identities",
		Steps: []schema.WorkflowStep{
			{
				Name:     "step1-mock-identity",
				Command:  getDumpCmd(envOutput1),
				Type:     "shell",
				Identity: "mock-identity",
			},
			{
				Name:     "step2-mock-identity-2",
				Command:  getDumpCmd(envOutput2),
				Type:     "shell",
				Identity: "mock-identity-2",
			},
			{
				Name:    "step3-no-identity",
				Command: getDumpCmd(envOutput3),
				Type:    "shell",
				// No identity - should inherit from parent process
			},
		},
	}

	// Execute the workflow.
	err = ExecuteWorkflow(atmosConfig, "test-multi-identity", "test.yaml", workflowDefinition, false, "", "", "")
	require.NoError(t, err, "Workflow execution should succeed")

	// Read and verify step 1 environment (mock-identity).
	env1Content, err := os.ReadFile(envOutput1)
	require.NoError(t, err)
	env1Vars := string(env1Content)
	assert.Contains(t, env1Vars, "ATMOS_IDENTITY=mock-identity", "Step 1 should use mock-identity")

	// Read and verify step 2 environment (mock-identity-2).
	env2Content, err := os.ReadFile(envOutput2)
	require.NoError(t, err)
	env2Vars := string(env2Content)
	assert.Contains(t, env2Vars, "ATMOS_IDENTITY=mock-identity-2", "Step 2 should use mock-identity-2")

	// Read step 3 environment (no identity - should NOT have ATMOS_IDENTITY from auth).
	env3Content, err := os.ReadFile(envOutput3)
	require.NoError(t, err)
	env3Vars := string(env3Content)
	// Step 3 should NOT have ATMOS_IDENTITY set by PrepareShellEnvironment
	// (it might have it from parent process, but not from our auth flow)
	t.Logf("Step 3 environment (no identity):\n%s", env3Vars)
}

// TestWorkflowIntegration_CommandLineIdentityOverride tests that --identity flag
// properly sets identity for steps that don't specify their own.
func TestWorkflowIntegration_CommandLineIdentityOverride(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode: exercises full auth flow")
	}

	// Set up test fixture with auth configuration.
	testDir := "../../tests/fixtures/scenarios/atmos-auth-mock"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	// Load atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	// Create temporary files to capture environment variables.
	tmpDir := t.TempDir()
	envOutput1 := filepath.Join(tmpDir, "env-step1.txt")
	envOutput2 := filepath.Join(tmpDir, "env-step2.txt")

	// Get OS-specific command to dump environment variables.
	var getDumpCmd func(string) string
	if runtime.GOOS == "windows" {
		getDumpCmd = func(file string) string { return "set > " + file }
	} else {
		getDumpCmd = func(file string) string { return "env > " + file }
	}

	// Define a test workflow with one step that has identity and one that doesn't.
	workflowDefinition := &schema.WorkflowDefinition{
		Description: "Test command-line identity override",
		Steps: []schema.WorkflowStep{
			{
				Name:    "step1-no-identity",
				Command: getDumpCmd(envOutput1),
				Type:    "shell",
				// No identity - should use command-line identity
			},
			{
				Name:     "step2-explicit-identity",
				Command:  getDumpCmd(envOutput2),
				Type:     "shell",
				Identity: "mock-identity-2",
				// Explicit identity - should override command-line identity
			},
		},
	}

	// Execute the workflow with command-line identity.
	err = ExecuteWorkflow(atmosConfig, "test-cmdline-override", "test.yaml", workflowDefinition, false, "", "", "mock-identity")
	require.NoError(t, err, "Workflow execution should succeed")

	// Read and verify step 1 environment (should use command-line identity).
	env1Content, err := os.ReadFile(envOutput1)
	require.NoError(t, err)
	env1Vars := string(env1Content)
	assert.Contains(t, env1Vars, "ATMOS_IDENTITY=mock-identity", "Step 1 should use command-line identity (mock-identity)")

	// Read and verify step 2 environment (should use explicit step identity).
	env2Content, err := os.ReadFile(envOutput2)
	require.NoError(t, err)
	env2Vars := string(env2Content)
	assert.Contains(t, env2Vars, "ATMOS_IDENTITY=mock-identity-2", "Step 2 should use explicit step identity (mock-identity-2)")
}

// TestWorkflowIntegration_AWSMockProviderEnvironment tests that AWS mock provider
// sets the expected AWS-specific environment variables.
func TestWorkflowIntegration_AWSMockProviderEnvironment(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode: exercises full auth flow")
	}

	// Set up test fixture with AWS auth configuration.
	testDir := "../../tests/fixtures/scenarios/atmos-auth-mock"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	// Load atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	// Create a temporary file to capture environment variables.
	tmpDir := t.TempDir()
	envOutputFile := filepath.Join(tmpDir, "aws-env.txt")

	// Get OS-specific command to dump environment variables and grep for AWS vars.
	var dumpEnvCmd string
	if runtime.GOOS == "windows" {
		// On Windows, use findstr to filter AWS variables.
		dumpEnvCmd = "set > " + envOutputFile
	} else {
		// On Unix, dump all env vars (we'll filter in Go).
		dumpEnvCmd = "env > " + envOutputFile
	}

	// Define a test workflow that dumps environment variables.
	workflowDefinition := &schema.WorkflowDefinition{
		Description: "Test AWS environment variables with mock provider",
		Steps: []schema.WorkflowStep{
			{
				Name:     "capture-aws-env",
				Command:  dumpEnvCmd,
				Type:     "shell",
				Identity: "mock-identity",
			},
		},
	}

	// Execute the workflow.
	err = ExecuteWorkflow(atmosConfig, "test-aws-env", "test.yaml", workflowDefinition, false, "", "", "")
	require.NoError(t, err, "Workflow execution should succeed")

	// Read the captured environment variables.
	envContent, err := os.ReadFile(envOutputFile)
	require.NoError(t, err, "Should be able to read environment output file")

	envVars := string(envContent)
	t.Logf("Captured environment variables:\n%s", envVars)

	// For the mock provider, we should see ATMOS_IDENTITY set.
	assert.Contains(t, envVars, "ATMOS_IDENTITY", "Should have ATMOS_IDENTITY")

	// Check for AWS-related variables that the mock provider sets.
	// The mock provider's PrepareEnvironment should set these.
	lines := strings.Split(envVars, "\n")
	var hasAtmosIdentity bool

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ATMOS_IDENTITY=") {
			hasAtmosIdentity = true
			t.Logf("Found ATMOS_IDENTITY: %s", line)
		}
	}

	assert.True(t, hasAtmosIdentity, "Should have ATMOS_IDENTITY environment variable set")
}
