package atmos

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// setupWorkflowsTestEnv creates a minimal Atmos project with a workflows directory
// containing one manifest file with two workflows.
func setupWorkflowsTestEnv(t *testing.T) *schema.AtmosConfiguration {
	t.Helper()

	tmpDir := t.TempDir()
	workflowsDir := filepath.Join(tmpDir, "workflows")
	require.NoError(t, os.MkdirAll(workflowsDir, 0o755))

	manifest := `workflows:
  deploy-all:
    description: Deploy all components
    steps:
      - command: echo deploying vpc
      - command: echo deploying tgw
  destroy-all:
    description: Destroy all components
    steps:
      - command: echo destroying vpc
`
	require.NoError(t, os.WriteFile(filepath.Join(workflowsDir, "deploy.yaml"), []byte(manifest), 0o600))

	return &schema.AtmosConfiguration{
		BasePath: tmpDir,
		Workflows: schema.Workflows{
			BasePath: "workflows",
		},
	}
}

func TestDescribeWorkflowsTool_Interface(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	tool := NewDescribeWorkflowsTool(config)

	assert.Equal(t, "atmos_describe_workflows", tool.Name())
	assert.NotEmpty(t, tool.Description())
	assert.False(t, tool.RequiresPermission())
	assert.False(t, tool.IsRestricted())

	params := tool.Parameters()
	require.Len(t, params, 2)
	assert.Equal(t, "output_type", params[0].Name)
	assert.False(t, params[0].Required)
	assert.Equal(t, "list", params[0].Default)
	assert.Equal(t, "query", params[1].Name)
	assert.False(t, params[1].Required)
}

func TestDescribeWorkflowsTool_Execute_DefaultList(t *testing.T) {
	atmosConfig := setupWorkflowsTestEnv(t)
	tool := NewDescribeWorkflowsTool(atmosConfig)

	result, err := tool.Execute(context.Background(), map[string]interface{}{})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success)

	assert.Equal(t, "list", result.Data["output_type"])
	count, ok := result.Data["count"].(int)
	require.True(t, ok)
	assert.Equal(t, 2, count)

	workflows, ok := result.Data["workflows"].([]schema.DescribeWorkflowsItem)
	require.True(t, ok)
	require.Len(t, workflows, 2)

	assert.Contains(t, result.Output, "deploy-all")
	assert.Contains(t, result.Output, "destroy-all")
}

func TestDescribeWorkflowsTool_Execute_MapOutputType(t *testing.T) {
	atmosConfig := setupWorkflowsTestEnv(t)
	tool := NewDescribeWorkflowsTool(atmosConfig)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"output_type": "map",
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success)
	assert.Equal(t, "map", result.Data["output_type"])

	res, ok := result.Data["result"].(map[string][]string)
	require.True(t, ok)
	assert.Contains(t, res, "deploy.yaml")
}

func TestDescribeWorkflowsTool_Execute_AllOutputType(t *testing.T) {
	atmosConfig := setupWorkflowsTestEnv(t)
	tool := NewDescribeWorkflowsTool(atmosConfig)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"output_type": "all",
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success)
	assert.Equal(t, "all", result.Data["output_type"])

	res, ok := result.Data["result"].(map[string]schema.WorkflowManifest)
	require.True(t, ok)
	assert.Contains(t, res, "deploy.yaml")
}

func TestDescribeWorkflowsTool_Execute_InvalidOutputType(t *testing.T) {
	atmosConfig := setupWorkflowsTestEnv(t)
	tool := NewDescribeWorkflowsTool(atmosConfig)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"output_type": "bogus",
	})

	require.Error(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
}

func TestDescribeWorkflowsTool_Execute_NoWorkflowsDir(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
		Workflows: schema.Workflows{
			BasePath: "workflows",
		},
	}
	tool := NewDescribeWorkflowsTool(atmosConfig)

	result, err := tool.Execute(context.Background(), map[string]interface{}{})

	require.Error(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
}
