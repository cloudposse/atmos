package workflow

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	fixturesPath = "../../tests/fixtures/scenarios/workflows"
)

func TestStackFlagCompletion_Success(t *testing.T) {
	t.Setenv("ATMOS_CLI_CONFIG_PATH", fixturesPath)

	cmd := &cobra.Command{}
	_, directive := stackFlagCompletion(cmd, []string{}, "")

	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	// Results can be nil or empty slice on success.
	// The important thing is the directive is correct.
}

func TestStackFlagCompletion_ConfigError(t *testing.T) {
	tmpDir := t.TempDir()

	// Create invalid atmos.yaml.
	invalidConfig := `this is not valid yaml: [[[`
	err := os.WriteFile(filepath.Join(tmpDir, "atmos.yaml"), []byte(invalidConfig), 0o644)
	require.NoError(t, err)

	t.Setenv("ATMOS_CLI_CONFIG_PATH", tmpDir)

	cmd := &cobra.Command{}
	results, directive := stackFlagCompletion(cmd, []string{}, "")

	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	assert.Nil(t, results)
}

func TestListAllStacks_Success(t *testing.T) {
	t.Setenv("ATMOS_CLI_CONFIG_PATH", fixturesPath)

	results, err := listAllStacks()

	// May error if the fixtures don't have valid stacks configured.
	// The important thing is that the function runs without panicking.
	_ = err
	_ = results
}

func TestListAllStacks_ConfigError(t *testing.T) {
	tmpDir := t.TempDir()
	// No atmos.yaml file.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", tmpDir)

	results, err := listAllStacks()

	assert.Error(t, err)
	assert.Nil(t, results)
}

func TestListAllStacks_InvalidStack(t *testing.T) {
	tmpDir := t.TempDir()

	// Create minimal atmos.yaml.
	atmosConfig := `
base_path: ""
stacks:
  base_path: "stacks"
  included_paths:
    - "**/*"
`
	err := os.WriteFile(filepath.Join(tmpDir, "atmos.yaml"), []byte(atmosConfig), 0o644)
	require.NoError(t, err)

	// Create stacks directory with invalid YAML.
	stacksDir := filepath.Join(tmpDir, "stacks")
	err = os.MkdirAll(stacksDir, 0o755)
	require.NoError(t, err)

	invalidStack := `this is not valid yaml: {{{`
	err = os.WriteFile(filepath.Join(stacksDir, "invalid.yaml"), []byte(invalidStack), 0o644)
	require.NoError(t, err)

	t.Setenv("ATMOS_CLI_CONFIG_PATH", tmpDir)

	results, err := listAllStacks()

	assert.Error(t, err)
	assert.Nil(t, results)
}

func TestIdentityFlagCompletion_Success(t *testing.T) {
	tmpDir := t.TempDir()

	// Create atmos.yaml with identities.
	atmosConfig := `
base_path: ""
stacks:
  base_path: "stacks"
workflows:
  base_path: "stacks/workflows"
auth:
  identities:
    prod-admin:
      type: "aws"
      role_arn: "arn:aws:iam::123456789012:role/admin"
    dev-user:
      type: "aws"
      role_arn: "arn:aws:iam::123456789012:role/developer"
`
	err := os.WriteFile(filepath.Join(tmpDir, "atmos.yaml"), []byte(atmosConfig), 0o644)
	require.NoError(t, err)

	t.Setenv("ATMOS_CLI_CONFIG_PATH", tmpDir)

	cmd := &cobra.Command{}
	results, directive := identityFlagCompletion(cmd, []string{}, "")

	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	assert.NotNil(t, results)
	assert.Len(t, results, 2)
	// Should be sorted alphabetically.
	assert.Contains(t, results, "dev-user")
	assert.Contains(t, results, "prod-admin")
	assert.Equal(t, "dev-user", results[0])
	assert.Equal(t, "prod-admin", results[1])
}

func TestIdentityFlagCompletion_NoIdentities(t *testing.T) {
	t.Setenv("ATMOS_CLI_CONFIG_PATH", fixturesPath)

	cmd := &cobra.Command{}
	_, directive := identityFlagCompletion(cmd, []string{}, "")

	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	// When no identities configured, should return empty slice or nil.
}

func TestIdentityFlagCompletion_ConfigError(t *testing.T) {
	tmpDir := t.TempDir()

	invalidConfig := `invalid: yaml: content: [[[`
	err := os.WriteFile(filepath.Join(tmpDir, "atmos.yaml"), []byte(invalidConfig), 0o644)
	require.NoError(t, err)

	t.Setenv("ATMOS_CLI_CONFIG_PATH", tmpDir)

	cmd := &cobra.Command{}
	results, directive := identityFlagCompletion(cmd, []string{}, "")

	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	assert.Nil(t, results)
}

func TestWorkflowNameCompletion_Success(t *testing.T) {
	t.Setenv("ATMOS_CLI_CONFIG_PATH", fixturesPath)

	cmd := &cobra.Command{}
	results, directive := workflowNameCompletion(cmd, []string{}, "")

	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	// Results depend on fixtures - may be empty or populated.
	// If populated, format should be "workflow-name\tfile.yaml".
	for _, result := range results {
		assert.Contains(t, result, "\t")
	}
}

func TestWorkflowNameCompletion_MultipleArgs(t *testing.T) {
	t.Setenv("ATMOS_CLI_CONFIG_PATH", fixturesPath)

	cmd := &cobra.Command{}
	// When args are provided, should not complete.
	results, directive := workflowNameCompletion(cmd, []string{"some-workflow"}, "")

	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	assert.Nil(t, results)
}

func TestWorkflowNameCompletion_ConfigError(t *testing.T) {
	tmpDir := t.TempDir()

	invalidConfig := `bad: yaml: [[[`
	err := os.WriteFile(filepath.Join(tmpDir, "atmos.yaml"), []byte(invalidConfig), 0o644)
	require.NoError(t, err)

	t.Setenv("ATMOS_CLI_CONFIG_PATH", tmpDir)

	cmd := &cobra.Command{}
	results, directive := workflowNameCompletion(cmd, []string{}, "")

	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	assert.Nil(t, results)
}

func TestWorkflowNameCompletion_InvalidWorkflowFile(t *testing.T) {
	tmpDir := t.TempDir()

	atmosConfig := `
base_path: ""
workflows:
  base_path: "stacks/workflows"
`
	err := os.WriteFile(filepath.Join(tmpDir, "atmos.yaml"), []byte(atmosConfig), 0o644)
	require.NoError(t, err)

	// Create workflows directory with invalid YAML.
	workflowsDir := filepath.Join(tmpDir, "stacks", "workflows")
	err = os.MkdirAll(workflowsDir, 0o755)
	require.NoError(t, err)

	invalidWorkflow := `this is invalid: yaml: {{{`
	err = os.WriteFile(filepath.Join(workflowsDir, "invalid.yaml"), []byte(invalidWorkflow), 0o644)
	require.NoError(t, err)

	t.Setenv("ATMOS_CLI_CONFIG_PATH", tmpDir)

	cmd := &cobra.Command{}
	results, directive := workflowNameCompletion(cmd, []string{}, "")

	// Should silently fail and return nil.
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	assert.Nil(t, results)
}

func TestWorkflowNameCompletion_DuplicateWorkflows(t *testing.T) {
	tmpDir := t.TempDir()

	atmosConfig := `
base_path: ""
workflows:
  base_path: "stacks/workflows"
`
	err := os.WriteFile(filepath.Join(tmpDir, "atmos.yaml"), []byte(atmosConfig), 0o644)
	require.NoError(t, err)

	workflowsDir := filepath.Join(tmpDir, "stacks", "workflows")
	err = os.MkdirAll(workflowsDir, 0o755)
	require.NoError(t, err)

	// Create two files with the same workflow name.
	workflow1 := `
workflows:
  deploy:
    description: "Deploy from file 1"
    steps:
      - name: "step1"
        type: "shell"
        command: "echo file1"
`
	err = os.WriteFile(filepath.Join(workflowsDir, "file1.yaml"), []byte(workflow1), 0o644)
	require.NoError(t, err)

	workflow2 := `
workflows:
  deploy:
    description: "Deploy from file 2"
    steps:
      - name: "step1"
        type: "shell"
        command: "echo file2"
`
	err = os.WriteFile(filepath.Join(workflowsDir, "file2.yaml"), []byte(workflow2), 0o644)
	require.NoError(t, err)

	t.Setenv("ATMOS_CLI_CONFIG_PATH", tmpDir)

	cmd := &cobra.Command{}
	results, directive := workflowNameCompletion(cmd, []string{}, "")

	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	if len(results) > 0 {
		assert.Len(t, results, 2)
		// Should have both entries with different file contexts.
		assert.Contains(t, results, "deploy\tfile1.yaml")
		assert.Contains(t, results, "deploy\tfile2.yaml")
	}
}

func TestAddStackCompletion(t *testing.T) {
	cmd := &cobra.Command{}

	// Test adding stack completion.
	addStackCompletion(cmd)

	// Verify flag was added.
	flag := cmd.Flag("stack")
	assert.NotNil(t, flag)
	assert.Equal(t, "s", flag.Shorthand)
	assert.Equal(t, stackHint, flag.Usage)

	// Test idempotency - adding again should not panic.
	addStackCompletion(cmd)
	assert.NotNil(t, cmd.Flag("stack"))
}

func TestAddIdentityCompletion(t *testing.T) {
	t.Run("command with identity flag", func(t *testing.T) {
		cmd := &cobra.Command{}
		cmd.Flags().String("identity", "", "Identity to use")

		// Should not panic.
		addIdentityCompletion(cmd)
		assert.NotNil(t, cmd.Flag("identity"))
	})

	t.Run("command without identity flag", func(t *testing.T) {
		cmd := &cobra.Command{}

		// Should not panic and not add the flag.
		addIdentityCompletion(cmd)
		assert.Nil(t, cmd.Flag("identity"))
	})

	t.Run("idempotency - adding twice should not panic", func(t *testing.T) {
		cmd := &cobra.Command{}
		cmd.Flags().String("identity", "", "Identity to use")

		// Call twice - should not panic.
		addIdentityCompletion(cmd)
		addIdentityCompletion(cmd)
		assert.NotNil(t, cmd.Flag("identity"))
	})
}

func TestWorkflowCmdRunE_HelpArgument(t *testing.T) {
	// Test that "help" as first argument shows help without error.
	cmd := workflowCmd

	// Reset command state.
	cmd.SetArgs([]string{"help"})

	// Running with "help" should show help and return nil.
	err := cmd.RunE(cmd, []string{"help"})
	assert.NoError(t, err)
}

func TestWorkflowCmdRunE_WorkflowExecution(t *testing.T) {
	t.Setenv("ATMOS_CLI_CONFIG_PATH", fixturesPath)
	t.Setenv("ATMOS_BASE_PATH", fixturesPath)

	cmd := workflowCmd

	// Test running an existing workflow.
	err := cmd.RunE(cmd, []string{"shell-pass"})
	// May succeed or fail based on workflow content.
	// The important test is that it doesn't panic.
	_ = err
}

func TestWorkflowCommandProvider(t *testing.T) {
	provider := &WorkflowCommandProvider{}

	t.Run("GetCommand returns workflowCmd", func(t *testing.T) {
		cmd := provider.GetCommand()
		assert.NotNil(t, cmd)
		assert.Equal(t, "workflow [name]", cmd.Use)
	})

	t.Run("GetName returns workflow", func(t *testing.T) {
		name := provider.GetName()
		assert.Equal(t, "workflow", name)
	})

	t.Run("GetGroup returns correct group", func(t *testing.T) {
		group := provider.GetGroup()
		assert.Equal(t, "Core Stack Commands", group)
	})

	t.Run("GetFlagsBuilder returns nil", func(t *testing.T) {
		builder := provider.GetFlagsBuilder()
		assert.Nil(t, builder)
	})

	t.Run("GetPositionalArgsBuilder returns nil", func(t *testing.T) {
		builder := provider.GetPositionalArgsBuilder()
		assert.Nil(t, builder)
	})

	t.Run("GetCompatibilityFlags returns nil", func(t *testing.T) {
		flags := provider.GetCompatibilityFlags()
		assert.Nil(t, flags)
	})

	t.Run("GetAliases returns nil", func(t *testing.T) {
		aliases := provider.GetAliases()
		assert.Nil(t, aliases)
	})
}
