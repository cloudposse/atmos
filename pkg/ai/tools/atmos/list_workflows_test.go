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

func TestListWorkflowsTool_Interface(t *testing.T) {
	atmosConfig := setupWorkflowsTestEnv(t)
	tool := NewListWorkflowsTool(atmosConfig)

	assert.Equal(t, "atmos_list_workflows", tool.Name())
	assert.NotEmpty(t, tool.Description())
	assert.False(t, tool.RequiresPermission())
	assert.False(t, tool.IsRestricted())

	params := tool.Parameters()
	require.Len(t, params, 1)
	assert.Equal(t, "file", params[0].Name)
	assert.False(t, params[0].Required)
}

func TestListWorkflowsTool_Execute_ListsAll(t *testing.T) {
	atmosConfig := setupWorkflowsTestEnv(t)
	tool := NewListWorkflowsTool(atmosConfig)

	result, err := tool.Execute(context.Background(), map[string]interface{}{})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success)

	count, ok := result.Data["count"].(int)
	require.True(t, ok)
	assert.Equal(t, 2, count)

	workflows, ok := result.Data["workflows"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, workflows, 2)

	names := []string{}
	for _, w := range workflows {
		name, _ := w["workflow"].(string)
		names = append(names, name)
	}
	assert.Contains(t, names, "deploy-all")
	assert.Contains(t, names, "destroy-all")

	assert.Contains(t, result.Output, "deploy-all")
}

func TestListWorkflowsTool_Execute_EmptyWorkflowsDir(t *testing.T) {
	tmpDir := t.TempDir()
	workflowsDir := filepath.Join(tmpDir, "workflows")
	require.NoError(t, os.MkdirAll(workflowsDir, 0o755))

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tmpDir,
		Workflows: schema.Workflows{
			BasePath: "workflows",
		},
	}
	tool := NewListWorkflowsTool(atmosConfig)

	result, err := tool.Execute(context.Background(), map[string]interface{}{})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success)

	count, ok := result.Data["count"].(int)
	require.True(t, ok)
	assert.Equal(t, 0, count)
}

func TestListWorkflowsTool_Execute_FileFilter(t *testing.T) {
	atmosConfig := setupWorkflowsTestEnv(t)
	tool := NewListWorkflowsTool(atmosConfig)

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"file": filepath.Join(atmosConfig.BasePath, "workflows", "deploy.yaml"),
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success)

	count, ok := result.Data["count"].(int)
	require.True(t, ok)
	assert.Equal(t, 2, count)
	assert.Equal(t, filepath.Join(atmosConfig.BasePath, "workflows", "deploy.yaml"), result.Data["file"])
}
