package exec

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestWorkflowWithIdentity_ShellCommand(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode: spawns actual shell process")
	}

	// Get OS-specific shell and commands.
	var exitCmd string
	if runtime.GOOS == "windows" {
		exitCmd = "/c"
	} else {
		exitCmd = "-c"
	}

	// Set up test fixture with auth configuration.
	testDir := "../../tests/fixtures/scenarios/atmos-auth-mock"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	// Load atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	// Define a test workflow with identity.
	workflowDefinition := &schema.WorkflowDefinition{
		Description: "Test workflow with identity",
		Steps: []schema.WorkflowStep{
			{
				Name:     "test-step-with-identity",
				Command:  exitCmd + " exit 0",
				Type:     "shell",
				Identity: "mock-identity",
			},
		},
	}

	// Execute the workflow.
	err = ExecuteWorkflow(atmosConfig, "test-workflow", "test-workflow.yaml", workflowDefinition, false, "", "", "")

	// Should succeed with mock identity.
	assert.NoError(t, err)
}

func TestWorkflowWithIdentity_MultipleSteps(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode: spawns actual shell process")
	}

	// Get OS-specific shell and commands.
	var exitCmd string
	if runtime.GOOS == "windows" {
		exitCmd = "/c"
	} else {
		exitCmd = "-c"
	}

	// Set up test fixture with auth configuration.
	testDir := "../../tests/fixtures/scenarios/atmos-auth-mock"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	// Load atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	// Define a test workflow with multiple steps, some with identity, some without.
	workflowDefinition := &schema.WorkflowDefinition{
		Description: "Test workflow with mixed identity usage",
		Steps: []schema.WorkflowStep{
			{
				Name:    "step-without-identity",
				Command: exitCmd + " exit 0",
				Type:    "shell",
			},
			{
				Name:     "step-with-mock-identity",
				Command:  exitCmd + " exit 0",
				Type:     "shell",
				Identity: "mock-identity",
			},
			{
				Name:     "step-with-mock-identity-2",
				Command:  exitCmd + " exit 0",
				Type:     "shell",
				Identity: "mock-identity-2",
			},
		},
	}

	// Execute the workflow.
	err = ExecuteWorkflow(atmosConfig, "test-workflow-mixed", "test-workflow.yaml", workflowDefinition, false, "", "", "")

	// Should succeed with mock identities.
	assert.NoError(t, err)
}

func TestWorkflowWithIdentity_InvalidIdentity(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode: requires auth configuration")
	}

	// Get OS-specific shell and commands.
	var exitCmd string
	if runtime.GOOS == "windows" {
		exitCmd = "/c"
	} else {
		exitCmd = "-c"
	}

	// Set up test fixture with auth configuration.
	testDir := "../../tests/fixtures/scenarios/atmos-auth-mock"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	// Load atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	// Define a test workflow with an invalid identity.
	workflowDefinition := &schema.WorkflowDefinition{
		Description: "Test workflow with invalid identity",
		Steps: []schema.WorkflowStep{
			{
				Name:     "step-with-invalid-identity",
				Command:  exitCmd + " exit 0",
				Type:     "shell",
				Identity: "nonexistent-identity",
			},
		},
	}

	// Execute the workflow.
	err = ExecuteWorkflow(atmosConfig, "test-workflow-invalid", "test-workflow.yaml", workflowDefinition, false, "", "", "")

	// Should fail with authentication error.
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "authentication failed")
}

func TestWorkflowWithIdentity_AtmosCommand(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode: spawns actual atmos subprocess")
	}

	// Set up test fixture with auth configuration.
	testDir := "../../tests/fixtures/scenarios/atmos-auth-mock"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	// Load atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	// Define a test workflow with identity for atmos command.
	workflowDefinition := &schema.WorkflowDefinition{
		Description: "Test workflow with identity for atmos command",
		Steps: []schema.WorkflowStep{
			{
				Name:     "test-atmos-with-identity",
				Command:  "version",
				Type:     "atmos",
				Identity: "mock-identity",
			},
		},
	}

	// Execute the workflow.
	err = ExecuteWorkflow(atmosConfig, "test-workflow-atmos", "test-workflow.yaml", workflowDefinition, false, "", "", "")

	// Should succeed with mock identity.
	assert.NoError(t, err)
}

func TestWorkflowWithIdentity_DryRun(t *testing.T) {
	// Set up test fixture with auth configuration.
	testDir := "../../tests/fixtures/scenarios/atmos-auth-mock"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	// Load atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	// Define a test workflow with identity.
	workflowDefinition := &schema.WorkflowDefinition{
		Description: "Test workflow with identity dry run",
		Steps: []schema.WorkflowStep{
			{
				Name:     "test-step-with-identity",
				Command:  "echo test",
				Type:     "shell",
				Identity: "mock-identity",
			},
		},
	}

	// Execute the workflow in dry-run mode.
	err = ExecuteWorkflow(atmosConfig, "test-workflow-dryrun", "test-workflow.yaml", workflowDefinition, true, "", "", "")

	// Should succeed in dry-run mode.
	assert.NoError(t, err)
}

func TestWorkflowWithIdentity_CommandLineOverride(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode: spawns actual shell process")
	}

	// Get OS-specific shell and commands.
	var exitCmd string
	if runtime.GOOS == "windows" {
		exitCmd = "/c"
	} else {
		exitCmd = "-c"
	}

	// Set up test fixture with auth configuration.
	testDir := "../../tests/fixtures/scenarios/atmos-auth-mock"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	// Load atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	// Define a test workflow WITHOUT identity in steps.
	workflowDefinition := &schema.WorkflowDefinition{
		Description: "Test workflow with command-line identity",
		Steps: []schema.WorkflowStep{
			{
				Name:    "step-without-identity",
				Command: exitCmd + " exit 0",
				Type:    "shell",
				// No identity specified in step
			},
		},
	}

	// Execute the workflow with command-line identity.
	err = ExecuteWorkflow(atmosConfig, "test-workflow-cmdline", "test-workflow.yaml", workflowDefinition, false, "", "", "mock-identity")

	// Should succeed with command-line identity applied to steps without their own identity.
	assert.NoError(t, err)
}
