package stack

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/generator/engine"
	generatorui "github.com/cloudposse/atmos/pkg/generator/ui"
	"github.com/cloudposse/atmos/pkg/hooks"
	ioLayer "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	stackcreate "github.com/cloudposse/atmos/pkg/stack/create"
	"github.com/cloudposse/atmos/pkg/ui"
)

func TestCreateCommandThroughScaffold(t *testing.T) {
	captureLifecycle(t)
	root := t.TempDir()
	t.Chdir(root)
	old := atmosConfigPtr
	SetAtmosConfig(&schema.AtmosConfiguration{BasePathAbsolute: root, StacksBaseAbsolutePath: filepath.Join(root, "stacks"), Stacks: schema.Stacks{IncludedPaths: []string{"**/*"}}})
	t.Cleanup(func() { SetAtmosConfig(old) })
	cmd := newStackCreateCommand()
	args := []string{"--file", "stacks/dev.yaml", "--name", "development", "--component", "terraform:network", "--defaults"}
	require.NoError(t, cmd.ParseFlags(args))
	cmd.SetContext(context.Background())
	require.NoError(t, cmd.RunE(cmd, nil))
	content, err := os.ReadFile("stacks/dev.yaml")
	require.NoError(t, err)
	assert.Contains(t, string(content), `name: "development"`)
	assert.Contains(t, string(content), `"network":`)
	require.Error(t, cmd.RunE(cmd, nil))
	assert.True(t, stackDeleteCmd.Hidden)
	assert.NotEmpty(t, stackDeleteCmd.Deprecated)
}

func TestCreatePreviewShowsContentsAndHooks(t *testing.T) {
	output := &bytes.Buffer{}
	ctx, err := ioLayer.NewContext(ioLayer.WithStreams(stackValidateTestStreams{output: output}))
	require.NoError(t, err)
	data.InitWriter(ctx)
	ui.InitFormatter(ctx)
	t.Cleanup(data.Reset)
	t.Cleanup(ui.Reset)
	plan := &stackcreate.Plan{Generation: &generatorui.CreationPlan{Files: []engine.File{{Path: "stacks/dev.yaml", Content: "vars: {}\n"}}}, Hooks: map[string]hooks.Hook{
		"build": {Events: []string{"after.scaffold.generate"}, Kind: "step", Type: "shell", With: map[string]any{"command": "echo generated"}},
	}}
	require.NoError(t, previewStackCreation(plan, nil))
	assert.Contains(t, output.String(), "vars: {}")
	assert.Contains(t, output.String(), "after.scaffold.generate")
	assert.Contains(t, output.String(), "echo generated")
	output.Reset()
	require.NoError(t, previewStackCreation(plan, func(string) bool { return true }))
	assert.Contains(t, output.String(), "Would skip scaffold hook build")
	assert.NotContains(t, output.String(), "echo generated")
}

func TestCreateCommandPassesScaffoldAnswers(t *testing.T) {
	captureLifecycle(t)
	root := t.TempDir()
	t.Chdir(root)
	old := atmosConfigPtr
	SetAtmosConfig(&schema.AtmosConfiguration{BasePathAbsolute: root, StacksBaseAbsolutePath: filepath.Join(root, "stacks"), Stacks: schema.Stacks{IncludedPaths: []string{"**/*"}}})
	t.Cleanup(func() { SetAtmosConfig(old) })
	require.NoError(t, os.MkdirAll("recipe", 0o755))
	require.NoError(t, os.WriteFile("recipe/scaffold.yaml", []byte(`apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: custom
spec:
  stack:
    manifest: '{{ .Config.stack_file }}'
  fields:
    - name: environment
      type: input
      required: true
  files:
    - path: stack.yaml
      target: '{{ .Config.stack_file }}'
`), 0o644))
	require.NoError(t, os.WriteFile("recipe/stack.yaml", []byte("# atmos:template\nvars:\n  stage: {{ .Config.environment | quote }}\n"), 0o644))
	cmd := newStackCreateCommand()
	require.NoError(t, cmd.ParseFlags([]string{"--file", "stacks/dev.yaml", "--template", "./recipe", "--set", "environment=dev", "--defaults"}))
	cmd.SetContext(context.Background())
	require.NoError(t, cmd.RunE(cmd, nil))
	content, err := os.ReadFile("stacks/dev.yaml")
	require.NoError(t, err)
	assert.Contains(t, string(content), `stage: "dev"`)
}
