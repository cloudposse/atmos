package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWorkflowWithoutStacksConfig tests that workflows execute successfully
// when no stacks configuration is provided and --stack flag is not used.
// This is a critical requirement: workflows should only require stacks config
// when the --stack flag is explicitly passed.
func TestWorkflowWithoutStacksConfig(t *testing.T) {
	// Create temp directory with ONLY workflow config (no stacks).
	tmpDir := t.TempDir()
	workflowsDir := filepath.Join(tmpDir, "workflows")
	err := os.MkdirAll(workflowsDir, 0o755)
	require.NoError(t, err)

	// Minimal atmos.yaml - NO stacks section.
	atmosConfig := `
base_path: "."
workflows:
  base_path: "workflows"
`
	err = os.WriteFile(filepath.Join(tmpDir, "atmos.yaml"), []byte(atmosConfig), 0o644)
	require.NoError(t, err)

	// Simple shell workflow.
	workflow := `
workflows:
  simple:
    description: A simple workflow that requires no stacks
    steps:
      - name: step1
        type: shell
        command: echo hello
`
	err = os.WriteFile(filepath.Join(workflowsDir, "test.yaml"), []byte(workflow), 0o644)
	require.NoError(t, err)

	t.Setenv("ATMOS_CLI_CONFIG_PATH", tmpDir)
	t.Setenv("ATMOS_BASE_PATH", tmpDir)

	cmd := createWorkflowCmdForTest()
	err = cmd.ParseFlags([]string{"--file", "test.yaml"})
	require.NoError(t, err)

	// Should succeed WITHOUT stacks configured because --stack was not provided.
	err = ExecuteWorkflowCmd(cmd, []string{"simple"})
	assert.NoError(t, err)
}

// TestWorkflowWithStackFlagRequiresStacksConfig tests that when --stack flag
// is provided, stacks configuration must be present.
func TestWorkflowWithStackFlagRequiresStacksConfig(t *testing.T) {
	// Create temp directory with ONLY workflow config (no stacks).
	tmpDir := t.TempDir()
	workflowsDir := filepath.Join(tmpDir, "workflows")
	err := os.MkdirAll(workflowsDir, 0o755)
	require.NoError(t, err)

	// Minimal atmos.yaml - NO stacks section.
	atmosConfig := `
base_path: "."
workflows:
  base_path: "workflows"
`
	err = os.WriteFile(filepath.Join(tmpDir, "atmos.yaml"), []byte(atmosConfig), 0o644)
	require.NoError(t, err)

	// Simple shell workflow.
	workflow := `
workflows:
  simple:
    description: A simple workflow
    steps:
      - name: step1
        type: shell
        command: echo hello
`
	err = os.WriteFile(filepath.Join(workflowsDir, "test.yaml"), []byte(workflow), 0o644)
	require.NoError(t, err)

	t.Setenv("ATMOS_CLI_CONFIG_PATH", tmpDir)
	t.Setenv("ATMOS_BASE_PATH", tmpDir)

	cmd := createWorkflowCmdForTest()
	err = cmd.ParseFlags([]string{"--file", "test.yaml", "--stack", "dev"})
	require.NoError(t, err)

	// Should fail because --stack was provided but stacks not configured.
	err = ExecuteWorkflowCmd(cmd, []string{"simple"})
	assert.Error(t, err)
	// The error should indicate stacks configuration is required.
	assert.Contains(t, err.Error(), "stack")
}

// TestWorkflowWithStackFlagAndStacksConfigured tests that when --stack flag
// is provided with proper stacks configuration, the workflow executes.
func TestWorkflowWithStackFlagAndStacksConfigured(t *testing.T) {
	// Create temp directory with workflow and stacks config.
	tmpDir := t.TempDir()
	workflowsDir := filepath.Join(tmpDir, "workflows")
	stacksDir := filepath.Join(tmpDir, "stacks", "deploy")
	err := os.MkdirAll(workflowsDir, 0o755)
	require.NoError(t, err)
	err = os.MkdirAll(stacksDir, 0o755)
	require.NoError(t, err)

	// Full atmos.yaml WITH stacks section.
	atmosConfig := `
base_path: "."
stacks:
  base_path: "stacks"
  included_paths:
    - "deploy/**/*"
  name_pattern: "{stage}"
workflows:
  base_path: "workflows"
`
	err = os.WriteFile(filepath.Join(tmpDir, "atmos.yaml"), []byte(atmosConfig), 0o644)
	require.NoError(t, err)

	// Create a minimal stack file.
	stackContent := `components: {}`
	err = os.WriteFile(filepath.Join(stacksDir, "dev.yaml"), []byte(stackContent), 0o644)
	require.NoError(t, err)

	// Simple shell workflow.
	workflow := `
workflows:
  simple:
    description: A simple workflow
    steps:
      - name: step1
        type: shell
        command: echo hello
`
	err = os.WriteFile(filepath.Join(workflowsDir, "test.yaml"), []byte(workflow), 0o644)
	require.NoError(t, err)

	t.Setenv("ATMOS_CLI_CONFIG_PATH", tmpDir)
	t.Setenv("ATMOS_BASE_PATH", tmpDir)

	cmd := createWorkflowCmdForTest()
	err = cmd.ParseFlags([]string{"--file", "test.yaml", "--stack", "dev"})
	require.NoError(t, err)

	// Should succeed because --stack was provided AND stacks are configured.
	err = ExecuteWorkflowCmd(cmd, []string{"simple"})
	assert.NoError(t, err)
}

// createWorkflowCmdForTest creates a cobra command with workflow flags for testing.
// This is a local helper to avoid dependency on the main createWorkflowCmd.
func createWorkflowCmdForTest() *cobra.Command {
	cmd := &cobra.Command{
		Use: "workflow",
	}
	// Workflow-specific flags.
	cmd.PersistentFlags().StringP("file", "f", "", "Workflow file")
	cmd.PersistentFlags().Bool("dry-run", false, "Dry run")
	cmd.PersistentFlags().StringP("stack", "s", "", "Stack")
	cmd.PersistentFlags().String("from-step", "", "From step")
	cmd.PersistentFlags().String("identity", "", "Identity")

	// Flags expected by ProcessCommandLineArgs.
	cmd.PersistentFlags().String("base-path", "", "Base path")
	cmd.PersistentFlags().StringSlice("config", []string{}, "Config files")
	cmd.PersistentFlags().StringSlice("config-path", []string{}, "Config paths")
	cmd.PersistentFlags().StringSlice("profile", []string{}, "Configuration profile")

	return cmd
}
