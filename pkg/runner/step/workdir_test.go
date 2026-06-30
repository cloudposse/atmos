package step

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestWorkdirHandlerExecuteProvisionsLocalSource(t *testing.T) {
	sourceDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "README.md"), []byte("demo\n"), 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(sourceDir, "nested"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "nested", "file.txt"), []byte("nested\n"), 0o644))

	targetDir := filepath.Join(t.TempDir(), "workdir")
	handler := &WorkdirHandler{BaseHandler: NewBaseHandler(schema.TaskTypeWorkdir, CategoryCommand, false)}

	result, err := handler.Execute(context.Background(), &schema.WorkflowStep{
		Name:   "fixture",
		Type:   schema.TaskTypeWorkdir,
		Source: sourceDir,
		Path:   targetDir,
		Reset:  true,
	}, NewVariables())
	require.NoError(t, err)
	assert.Equal(t, targetDir, result.Value)
	assert.Equal(t, sourceDir, result.Metadata["source"])

	content, err := os.ReadFile(filepath.Join(targetDir, "nested", "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "nested\n", string(content))
}

func TestWorkdirHandlerExecuteRequiresResetForExistingTarget(t *testing.T) {
	sourceDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "README.md"), []byte("demo\n"), 0o644))

	targetDir := t.TempDir()
	handler := &WorkdirHandler{BaseHandler: NewBaseHandler(schema.TaskTypeWorkdir, CategoryCommand, false)}

	_, err := handler.Execute(context.Background(), &schema.WorkflowStep{
		Name:   "fixture",
		Type:   schema.TaskTypeWorkdir,
		Source: map[string]any{"uri": sourceDir},
		Path:   targetDir,
	}, NewVariables())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "set reset: true")
}
