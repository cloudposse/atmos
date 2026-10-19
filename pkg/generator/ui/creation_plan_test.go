package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/generator/templates"
)

func TestCreationPlanUsesMatricesWithoutWritingOrProjectHistory(t *testing.T) {
	ui := createTestUI(t)
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".atmos"), 0o755))
	// A malformed unrelated record must not affect a new generation.
	require.NoError(t, os.WriteFile(filepath.Join(root, ".atmos", "scaffold.yaml"), []byte("unrelated"), 0o644))
	template := &templates.Configuration{Name: "matrix", Files: []templates.File{
		{Path: "scaffold.yaml", Content: `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: matrix
spec:
  fields:
    - name: envs
      type: multiselect
      options: [dev, prod]
      default: [dev, prod]
  files:
    - path: stack.yaml
      target: 'stacks/{{ .matrix.env }}.yaml'
      matrix:
        env: answers.envs
`},
		{Path: "stack.yaml", Content: "name: {{ .matrix.env }}\n", IsTemplate: true, Permissions: 0o644},
	}}
	plan, err := ui.PlanCreation(template, root, true, nil)
	require.NoError(t, err)
	require.Len(t, plan.Files, 2)
	assert.Equal(t, "stacks/dev.yaml", filepath.ToSlash(plan.Files[0].Path))
	assert.Equal(t, "name: dev\n", plan.Files[0].Content)
	assert.Equal(t, "stacks/prod.yaml", filepath.ToSlash(plan.Files[1].Path))
	assert.Equal(t, "name: prod\n", plan.Files[1].Content)
	assert.NoDirExists(t, filepath.Join(root, "stacks"))
	assert.Equal(t, []any{"dev", "prod"}, plan.Values["envs"])
	assert.False(t, ui.processor.DryRun)
	assert.Nil(t, ui.processor.PreviewFile)
}
