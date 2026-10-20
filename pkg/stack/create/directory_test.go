package create

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	runnerstep "github.com/cloudposse/atmos/pkg/runner/step"
)

const directoryDefinition = `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: directory
spec:
  fields:
    - name: environment
      type: input
      required: true
  files:
    - path: _defaults.yaml
    - path: network.yaml
    - path: services.yaml
    - path: README.md
`

func directoryFixture(t *testing.T) {
	t.Helper()
	write(t, "recipe/scaffold.yaml", directoryDefinition)
	write(t, "recipe/_defaults.yaml", "# atmos:template\nvars:\n  stage: {{ .Config.environment | quote }}\n")
	write(t, "recipe/network.yaml", `# atmos:template
name: {{ .Config.stack_name | quote }}
import:
  - ./_defaults
{{ range .Config.stack_imports }}  - {{ . | quote }}
{{ end }}components:
  terraform:
{{ range $name, $_ := .Config.stack_components.terraform }}    {{ $name | quote }}:
      metadata:
        enabled: false
      vars: {}
{{ end }}`)
	write(t, "recipe/services.yaml", `# atmos:template
import:
  - ./_defaults
components:
  helmfile:
{{ range $name, $_ := .Config.stack_components.helmfile }}    {{ $name | quote }}:
      metadata:
        enabled: false
      vars: {}
{{ end }}`)
	write(t, "recipe/README.md", "Configure and enable these components.\n")
}

func directoryOptions() *Options {
	return &Options{Target: "stacks/deploy/dev", Template: "./recipe", Name: "dev", Imports: []string{"catalog/defaults"}, Components: []string{"terraform:network", "helmfile:services"}, Values: map[string]any{"environment": "dev"}, Defaults: true}
}

func TestDirectoryCreationSplitStack(t *testing.T) {
	captureChannels(t)
	cfg, renderer := namedFixture(t)
	directoryFixture(t)
	write(t, "stacks/catalog/defaults.yaml", "vars:\n  region: us-east-1\n")
	// An unrelated file in an existing target directory is preserved.
	write(t, "stacks/deploy/dev/notes.txt", "keep me")
	plan, err := Prepare(cfg, directoryOptions(), renderer)
	require.NoError(t, err)
	assert.Empty(t, plan.Manifest, "no primary manifest is required")
	assert.Equal(t, []string{"stacks/deploy/dev/network.yaml", "stacks/deploy/dev/services.yaml"}, plan.Manifests)
	assert.NoFileExists(t, "stacks/deploy/dev/network.yaml")
	assert.NoDirExists(t, ".atmos")
	require.NoError(t, plan.Preview(nil))
	assert.NoFileExists(t, "stacks/deploy/dev/services.yaml")
	require.NoError(t, plan.Apply(context.Background(), nil))
	for _, file := range plan.Generation.Files {
		actual, err := os.ReadFile(filepath.Join(plan.Root, file.Path))
		require.NoError(t, err)
		assert.Equal(t, file.Content, string(actual))
	}
	notes, err := os.ReadFile("stacks/deploy/dev/notes.txt")
	require.NoError(t, err)
	assert.Equal(t, "keep me", string(notes))
	record := plan.Generation.Files[len(plan.Generation.Files)-1]
	var history map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(record.Content), &history))
	assert.Equal(t, "stacks/deploy/dev", history["target"])
	assert.Equal(t, []any{"stacks/deploy/dev/network.yaml", "stacks/deploy/dev/services.yaml"}, history["manifests"])
	assert.NotContains(t, history, "manifest")
	assert.Contains(t, record.Path, "directory-")
	_, err = Prepare(cfg, directoryOptions(), renderer)
	require.Error(t, err)
}

func TestDirectoryMatrixAndNestedOutputs(t *testing.T) {
	cfg, renderer := namedFixture(t)
	definition := strings.ReplaceAll(matrixDefinition, "  stack:\n    manifest: '{{ .Config.stack_file }}'\n", "")
	definition = strings.ReplaceAll(definition, "stacks/deploy/extra-", "regions/")
	matrixFixture(t, "# atmos:template\nvars:\n  stage: {{ .matrix.region }}\ncomponents:\n  terraform:\n    app:\n      vars: {}\n")
	write(t, "scaffolds/matrix/scaffold.yaml", definition)
	plan, err := Prepare(cfg, &Options{Target: "stacks/deploy/group", Template: "matrix", Defaults: true}, renderer)
	require.NoError(t, err)
	assert.Equal(t, []string{"stacks/deploy/group/regions/east.yaml", "stacks/deploy/group/regions/west.yaml", "stacks/deploy/group/stack.yaml"}, plan.Manifests)
	assert.NoDirExists(t, "stacks/deploy/group")
	require.NoError(t, plan.Apply(context.Background(), nil))
	assert.FileExists(t, "stacks/deploy/group/regions/east.yaml")
	assert.FileExists(t, "stacks/deploy/group/regions/west.yaml")
}

func TestDirectoryRejectsInvalidLayouts(t *testing.T) {
	for _, tc := range []struct{ name, path, content, want string }{
		{"no entrypoint", "recipe/scaffold.yaml", strings.ReplaceAll(directoryDefinition, "    - path: network.yaml\n    - path: services.yaml\n", ""), "at least one stack manifest"},
		{"bad yaml", "recipe/services.yaml", "components: [\n", "generated stack"},
		{"missing import", "recipe/services.yaml", "import: [missing]\nvars: {stage: dev}\n", "import"},
		{"collision", "recipe/services.yaml", "vars: {stage: dev}\ncomponents:\n  terraform:\n    network:\n      vars: {}\n", "more than one"},
		{"ignored name", "recipe/network.yaml", "vars: {stage: dev}\n", "--name"},
		{"ignored component", "recipe/services.yaml", "vars: {stage: dev}\n", "--component"},
		{"ignored import", "recipe/network.yaml", "# atmos:template\nname: {{ .Config.stack_name }}\nvars: {stage: dev}\n", "--import"},
		{"missing primary", "recipe/scaffold.yaml", strings.Replace(directoryDefinition, "spec:\n", "spec:\n  stack:\n    manifest: absent.yaml\n", 1), "did not generate"},
		{"excluded primary", "recipe/scaffold.yaml", strings.Replace(directoryDefinition, "spec:\n", "spec:\n  stack:\n    manifest: _defaults.yaml\n", 1), "excluded_paths"},
		{"traversal", "recipe/scaffold.yaml", strings.Replace(directoryDefinition, "    - path: README.md", "    - path: README.md\n      target: ../escaped.md", 1), "path"},
		{"duplicate path", "recipe/scaffold.yaml", strings.Replace(directoryDefinition, "    - path: README.md", "    - path: README.md\n      target: network.yaml", 1), "same"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, renderer := namedFixture(t)
			directoryFixture(t)
			write(t, "stacks/catalog/defaults.yaml", "vars: {}\n")
			write(t, tc.path, tc.content)
			opts := directoryOptions()
			if tc.name == "no entrypoint" {
				cfg.Stacks.IncludedPaths = []string{"absent/**/*"}
			}
			if tc.name == "collision" || tc.name == "missing import" {
				opts.Components = []string{"terraform:network"}
			}
			_, err := Prepare(cfg, opts, renderer)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
			assert.NoDirExists(t, "stacks/deploy/dev")
			assert.NoDirExists(t, ".atmos")
		})
	}
}

func TestDirectoryDestinationAndDefaultTemplate(t *testing.T) {
	for _, tc := range []struct{ name, file, target string }{
		{"both", "stacks/deploy/dev.yaml", "stacks/deploy/dev"},
		{"outside project", "", "../elsewhere"},
		{"outside stacks", "", "components/dev"},
		{"existing file", "", "stacks/deploy/file"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, renderer := fixture(t)
			write(t, "stacks/deploy/file", "existing")
			_, err := Prepare(cfg, &Options{File: tc.file, Target: tc.target, Defaults: true}, renderer)
			require.Error(t, err)
			assert.NoDirExists(t, ".atmos")
		})
	}
	t.Run("absolute target from another directory", func(t *testing.T) {
		cfg, renderer := fixture(t)
		t.Chdir(t.TempDir())
		target := filepath.Join(cfg.BasePathAbsolute, "stacks", "deploy", "dev")
		plan, err := Prepare(cfg, &Options{Target: target, Defaults: true}, renderer)
		require.NoError(t, err)
		assert.Equal(t, "stacks/deploy/dev/stack.yaml", filepath.ToSlash(plan.Manifest))
		assert.Equal(t, []string{"stacks/deploy/dev/stack.yaml"}, plan.Manifests)
		require.NoError(t, plan.Apply(context.Background(), nil))
		assert.FileExists(t, filepath.Join(target, "stack.yaml"))
	})
}

func TestDirectoryRecordsRequireAllPreviousOutputsAbsent(t *testing.T) {
	cfg, renderer := fixture(t)
	opts := &Options{Target: "stacks/deploy/dev", Defaults: true}
	plan, err := Prepare(cfg, opts, renderer)
	require.NoError(t, err)
	require.NoError(t, plan.Apply(context.Background(), nil))
	recordPath := plan.Generation.Files[len(plan.Generation.Files)-1].Path
	// A changed recipe must not replace history while an old output still exists.
	write(t, "recipe/scaffold.yaml", "apiVersion: atmos/v1\nkind: AtmosScaffoldConfig\nmetadata: {name: replacement}\nspec:\n  files:\n    - path: replacement.yaml\n")
	write(t, "recipe/replacement.yaml", "vars: {}\n")
	opts.Template = "./recipe"
	_, err = Prepare(cfg, opts, renderer)
	require.ErrorIs(t, err, errUtils.ErrFileExists)
	assert.Contains(t, err.Error(), "previous directory generation")
	require.NoError(t, os.Remove("stacks/deploy/dev/stack.yaml"))
	replacement, err := Prepare(cfg, opts, renderer)
	require.NoError(t, err)
	require.NoError(t, replacement.Apply(context.Background(), nil))
	assert.Equal(t, recordPath, replacement.Generation.Files[len(replacement.Generation.Files)-1].Path)
	// A second target has independent history.
	opts.Target = "stacks/deploy/prod"
	other, err := Prepare(cfg, opts, renderer)
	require.NoError(t, err)
	assert.NotEqual(t, recordPath, other.Generation.Files[len(other.Generation.Files)-1].Path)
}

func TestDirectoryApplyRechecksTargetBoundary(t *testing.T) {
	cfg, renderer := fixture(t)
	write(t, "recipe/scaffold.yaml", "apiVersion: atmos/v1\nkind: AtmosScaffoldConfig\nmetadata: {name: nested}\nspec:\n  files:\n    - path: stack.yaml\n      target: nested/stack.yaml\n")
	write(t, "recipe/stack.yaml", "vars: {}\n")
	plan, err := Prepare(cfg, &Options{Target: "stacks/deploy/dev", Template: "./recipe", Defaults: true}, renderer)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll("stacks/deploy/dev", 0o755))
	require.NoError(t, os.MkdirAll("elsewhere", 0o755))
	if err := os.Symlink(filepath.Join(cfg.BasePathAbsolute, "elsewhere"), "stacks/deploy/dev/nested"); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	require.ErrorIs(t, plan.Apply(context.Background(), nil), errUtils.ErrPathTraversal)
	assert.NoFileExists(t, "elsewhere/stack.yaml")
	assert.NoDirExists(t, ".atmos")
}

func TestDirectoryHooksReceiveTargetAndSurroundWrites(t *testing.T) {
	cfg, renderer := fixture(t)
	captureChannels(t)
	events := []string{}
	runnerstep.Register(&observationHook{BaseHandler: runnerstep.NewBaseHandler("directory-create-observe", runnerstep.CategoryOutput, false), events: &events, manifest: "stacks/deploy/dev/stack.yaml"})
	write(t, "recipe/scaffold.yaml", `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: hooks
spec:
  files:
    - path: stack.yaml
  hooks:
    before:
      events: [before.scaffold.generate]
      kind: step
      type: directory-create-observe
      with:
        content: 'before {{ .TargetPath }}'
    after:
      events: [after.scaffold.generate]
      kind: step
      type: directory-create-observe
      with:
        content: 'after {{ .TargetPath }}'
`)
	write(t, "recipe/stack.yaml", "vars: {}\n")
	plan, err := Prepare(cfg, &Options{Target: "stacks/deploy/dev", Template: "./recipe", Defaults: true}, renderer)
	require.NoError(t, err)
	require.NoError(t, plan.Preview(nil))
	assert.Empty(t, events)
	assert.NoDirExists(t, "stacks/deploy/dev")
	require.NoError(t, plan.Apply(context.Background(), nil))
	target := filepath.Join(cfg.BasePathAbsolute, "stacks", "deploy", "dev")
	assert.Equal(t, []string{"before " + target + " absent", "after " + target + " exists"}, events)
}
