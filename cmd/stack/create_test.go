package stack

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cockroach "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
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

// createProject points the stack command at a temporary project and returns the captured command output.
func createProject(t *testing.T) *bytes.Buffer {
	t.Helper()
	output := &bytes.Buffer{}
	ioCtx, err := ioLayer.NewContext(ioLayer.WithStreams(stackValidateTestStreams{output: output}))
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)
	t.Cleanup(data.Reset)
	t.Cleanup(ui.Reset)
	root := t.TempDir()
	t.Chdir(root)
	old := atmosConfigPtr
	SetAtmosConfig(&schema.AtmosConfiguration{BasePathAbsolute: root, StacksBaseAbsolutePath: filepath.Join(root, "stacks"), Stacks: schema.Stacks{IncludedPaths: []string{"**/*"}}})
	t.Cleanup(func() { SetAtmosConfig(old) })
	return output
}

func runCreate(t *testing.T, args ...string) error {
	t.Helper()
	cmd := newStackCreateCommand()
	require.NoError(t, cmd.ParseFlags(args))
	cmd.SetContext(context.Background())
	return cmd.RunE(cmd, nil)
}

func TestCreateRejectsMalformedSetEntries(t *testing.T) {
	createProject(t)
	for _, entry := range []string{"environment", "=dev", " =dev"} {
		err := runCreate(t, "--file", "stacks/dev.yaml", "--set", entry, "--defaults")
		require.ErrorIs(t, err, errUtils.ErrInvalidFlag, entry)
		assert.Contains(t, strings.Join(cockroach.GetAllHints(err), "\n"), "Use the format: --set key=value")
		assert.Contains(t, strings.Join(cockroach.GetAllDetails(err), "\n"), "Malformed --set flag")
		assert.NoFileExists(t, "stacks/dev.yaml")
	}
}

func TestCreateKeepsCommasAndBracesInRepeatedFlags(t *testing.T) {
	createProject(t)
	for _, name := range []string{"a", "b"} {
		require.NoError(t, os.MkdirAll(filepath.Join("stacks", "catalog"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join("stacks", "catalog", name+".yaml"), []byte("vars:\n  "+name+": 1\n"), 0o644))
	}
	require.NoError(t, runCreate(t, "--file", "stacks/dev.yaml", "--name", "dev", "--import", "catalog/{a,b}", "--import", "catalog/a", "--component", "terraform:network", "--defaults"))
	content, err := os.ReadFile("stacks/dev.yaml")
	require.NoError(t, err)
	var manifest struct {
		Import []string `yaml:"import"`
	}
	require.NoError(t, yaml.Unmarshal(content, &manifest))
	assert.Equal(t, []string{"catalog/{a,b}", "catalog/a"}, manifest.Import)
}

func TestCreateReportsEveryCreatedFile(t *testing.T) {
	output := createProject(t)
	require.NoError(t, runCreate(t, "--file", "stacks/dev.yaml", "--name", "dev", "--defaults"))
	assert.Contains(t, output.String(), "Created stack manifest stacks/dev.yaml")
	assert.Contains(t, output.String(), "Recorded the generation in .atmos/stack-generations/")
	assert.NotContains(t, output.String(), "would create")
	assert.NotContains(t, output.String(), "Would generate")
}

func TestCreateDryRunWritesNothingAndSeparatesFiles(t *testing.T) {
	output := createProject(t)
	require.NoError(t, runCreate(t, "--file", "stacks/dev.yaml", "--name", "dev", "--defaults", "--dry-run"))
	assert.NoFileExists(t, "stacks/dev.yaml")
	assert.NoDirExists(t, ".atmos")
	assert.Contains(t, output.String(), "# stacks/dev.yaml")
	assert.Contains(t, output.String(), "Would record the generation in .atmos/stack-generations/")
	assert.NotContains(t, output.String(), "answers")
}

func TestCreateRefusesRefForBuiltInScaffold(t *testing.T) {
	createProject(t)
	err := runCreate(t, "--file", "stacks/dev.yaml", "--ref", "v1", "--defaults")
	require.ErrorIs(t, err, errUtils.ErrInvalidFlag)
	assert.NoFileExists(t, "stacks/dev.yaml")
}

const createRecipeWithExtraFile = `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: extra
spec:
  stack:
    manifest: '{{ .Config.stack_file }}'
  files:
    - path: stack.yaml
      target: '{{ .Config.stack_file }}'
    - path: main.tf
      target: components/terraform/network/main.tf
`

func writeCreateRecipe(t *testing.T, definition string) {
	t.Helper()
	require.NoError(t, os.MkdirAll("recipe", 0o755))
	require.NoError(t, os.WriteFile(filepath.Join("recipe", "scaffold.yaml"), []byte(definition), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join("recipe", "stack.yaml"), []byte("vars:\n  stage: dev\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join("recipe", "main.tf"), []byte("# network\n"), 0o644))
}

func TestCreateRequiresDestination(t *testing.T) {
	createProject(t)
	err := runCreate(t, "--defaults")
	require.ErrorIs(t, err, errUtils.ErrInvalidFlag)
	assert.Contains(t, err.Error(), "--file or --target is required")
	assert.NoDirExists(t, ".atmos")
}

func TestCreateReportsScaffoldFilesOtherThanManifestAndRecord(t *testing.T) {
	output := createProject(t)
	writeCreateRecipe(t, createRecipeWithExtraFile)
	require.NoError(t, runCreate(t, "--file", "stacks/dev.yaml", "--template", "./recipe", "--defaults"))
	text := output.String()
	assert.Contains(t, text, "Created stack manifest stacks/dev.yaml")
	assert.Contains(t, text, "Created components/terraform/network/main.tf")
	assert.Contains(t, text, "Recorded the generation in .atmos/stack-generations/")
	assert.FileExists(t, filepath.Join("components", "terraform", "network", "main.tf"))
}

func TestCreateFailingScaffoldHookPreventsCreation(t *testing.T) {
	output := createProject(t)
	definition := createRecipeWithExtraFile + `  hooks:
    gate:
      events: [before.scaffold.generate]
      kind: step
      type: stack-create-cmd-unregistered-hook
      with:
        message: gate
`
	writeCreateRecipe(t, definition)
	require.Error(t, runCreate(t, "--file", "stacks/dev.yaml", "--template", "./recipe", "--defaults"))
	assert.NoFileExists(t, "stacks/dev.yaml")
	assert.NoDirExists(t, ".atmos")
	assert.NotContains(t, output.String(), "Created stack manifest")

	// Negative path: skipping every hook lets the same creation through.
	require.NoError(t, runCreate(t, "--file", "stacks/dev.yaml", "--template", "./recipe", "--defaults", "--skip-hooks"))
	assert.FileExists(t, "stacks/dev.yaml")
}

func TestCreateLoadsProjectConfigurationWhenNoneIsPreloaded(t *testing.T) {
	output := createProject(t)
	root, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile("atmos.yaml", []byte("base_path: .\nstacks:\n  base_path: stacks\n  included_paths: [\"**/*\"]\n"), 0o644))
	t.Setenv("ATMOS_CLI_CONFIG_PATH", root)
	t.Setenv("ATMOS_BASE_PATH", root)
	SetAtmosConfig(nil)

	config, err := stackCreationConfig()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "stacks"), config.StacksBaseAbsolutePath)

	require.NoError(t, runCreate(t, "--file", "stacks/dev.yaml", "--name", "dev", "--defaults"))
	assert.FileExists(t, filepath.Join(root, "stacks", "dev.yaml"))
	assert.Contains(t, output.String(), "Created stack manifest stacks/dev.yaml")
}

func TestCreateReportsUnloadableProjectConfiguration(t *testing.T) {
	createProject(t)
	root, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile("atmos.yaml", []byte("base_path: [unclosed\n"), 0o644))
	t.Setenv("ATMOS_CLI_CONFIG_PATH", root)
	SetAtmosConfig(nil)

	err = runCreate(t, "--file", "stacks/dev.yaml", "--defaults")
	require.Error(t, err)
	assert.NotErrorIs(t, err, errUtils.ErrInvalidFlag, "the configuration failure is reported, not a flag problem")
	assert.NoFileExists(t, filepath.Join(root, "stacks", "dev.yaml"))
}
