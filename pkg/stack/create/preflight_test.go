package create

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cockroach "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

const existingDev = "vars:\n  stage: dev\ncomponents:\n  terraform:\n    network:\n      vars:\n        name: dev-network\n"

// namedFixture is fixture with the naming convention that broke real projects: every stack is named by vars.stage.
func namedFixture(t *testing.T) (*schema.AtmosConfiguration, Renderer) {
	t.Helper()
	cfg, renderer := fixture(t)
	cfg.Stacks.NameTemplate = "{{ .vars.stage }}"
	return cfg, renderer
}

func hints(err error) string { return strings.Join(cockroach.GetAllHints(err), "\n") }

// assertNothingWritten proves a rejected creation left no manifest or generation record behind.
func assertNothingWritten(t *testing.T, cfg *schema.AtmosConfiguration, manifest string) {
	t.Helper()
	assert.NoFileExists(t, filepath.Join(cfg.BasePathAbsolute, manifest))
	assert.NoDirExists(t, filepath.Join(cfg.BasePathAbsolute, ".atmos"))
}

func TestPreflightRejectsStackWithoutResolvableName(t *testing.T) {
	cfg, renderer := namedFixture(t)
	write(t, filepath.Join(cfg.StacksBaseAbsolutePath, "deploy", "dev.yaml"), existingDev)
	opts := &Options{File: "stacks/deploy/bare.yaml", Components: []string{"terraform:vpc"}, Defaults: true}
	_, err := Prepare(cfg, opts, renderer)
	require.ErrorIs(t, err, errUtils.ErrInvalidStackManifest)
	assert.Contains(t, err.Error(), `map has no entry for key "stage"`)
	assert.Contains(t, err.Error(), "deploy/bare.yaml")
	assert.Contains(t, hints(err), "--name")
	assert.Contains(t, hints(err), "stacks.name_template")
	assertNothingWritten(t, cfg, opts.File)
	// The same request succeeds once the stack carries a name the project can resolve.
	opts.Name = "bare"
	plan, err := Prepare(cfg, opts, renderer)
	require.NoError(t, err)
	require.NoError(t, plan.Apply(context.Background(), nil))
}

func TestPreflightRejectsMissingImport(t *testing.T) {
	cfg, renderer := namedFixture(t)
	write(t, filepath.Join(cfg.StacksBaseAbsolutePath, "deploy", "dev.yaml"), existingDev)
	opts := &Options{File: "stacks/deploy/imp.yaml", Name: "imp", Imports: []string{"catalog/does-not-exist"}, Defaults: true}
	_, err := Prepare(cfg, opts, renderer)
	require.ErrorIs(t, err, errUtils.ErrInvalidStackManifest)
	assert.Contains(t, err.Error(), "catalog/does-not-exist")
	assert.Contains(t, hints(err), "--import")
	assertNothingWritten(t, cfg, opts.File)
	write(t, filepath.Join(cfg.StacksBaseAbsolutePath, "catalog", "does-not-exist.yaml"), "vars:\n  region: us-east-1\n")
	_, err = Prepare(cfg, opts, renderer)
	require.NoError(t, err)
}

func TestPreflightRejectsNameCollisionWithExistingManifest(t *testing.T) {
	cfg, renderer := namedFixture(t)
	customFixture(t, customDefinition)
	write(t, filepath.Join(cfg.StacksBaseAbsolutePath, "deploy", "dev.yaml"), existingDev)
	opts := &Options{File: "stacks/deploy/dev2.yaml", Template: "application-stack", Defaults: true, Values: map[string]any{"environment": "dev"}}
	_, err := Prepare(cfg, opts, renderer)
	require.ErrorIs(t, err, errUtils.ErrInvalidStackManifest)
	// The message names the stack, the colliding component and the manifest that already owns the stack.
	assert.Contains(t, err.Error(), `stack "dev"`)
	assert.Contains(t, err.Error(), `"network"`)
	assert.Contains(t, err.Error(), "deploy/dev,")
	assert.Contains(t, hints(err), "atmos stack config set")
	assertNothingWritten(t, cfg, opts.File)
	// A different stage is a different stack and is accepted.
	opts.Values["environment"] = "staging"
	_, err = Prepare(cfg, opts, renderer)
	require.NoError(t, err)
}

const matrixDefinition = `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: matrix
spec:
  stack:
    manifest: '{{ .Config.stack_file }}'
  files:
    - path: stack.yaml
      target: '{{ .Config.stack_file }}'
    - path: extra.yaml
      target: 'stacks/deploy/extra-{{ .matrix.region }}.yaml'
      matrix:
        region: [east, west]
`

func matrixFixture(t *testing.T, extra string) {
	t.Helper()
	write(t, "atmos.yaml", "scaffold:\n  templates:\n    matrix:\n      source: ./scaffolds/matrix\n")
	write(t, "scaffolds/matrix/scaffold.yaml", matrixDefinition)
	write(t, "scaffolds/matrix/stack.yaml", "vars:\n  stage: matrixed\n")
	write(t, "scaffolds/matrix/extra.yaml", extra)
}

func TestPreflightCoversEveryRenderedStackFile(t *testing.T) {
	opts := &Options{File: "stacks/deploy/matrix.yaml", Template: "matrix", Defaults: true}
	for _, tc := range []struct {
		name, extra, want string
	}{
		{name: "schema violation", extra: "# atmos:template\nvars:\n  stage: {{ .matrix.region }}\ncomponents:\n  terraform: not-a-map\n", want: "generated stack stacks/deploy/extra-east.yaml"},
		{name: "unresolvable name", extra: "# atmos:template\ncomponents:\n  terraform:\n    app:\n      vars:\n        region: {{ .matrix.region }}\n", want: `map has no entry for key "stage"`},
		{name: "colliding stacks", extra: "# atmos:template\nvars:\n  stage: shared\ncomponents:\n  terraform:\n    app:\n      vars:\n        region: {{ .matrix.region }}\n", want: `stack "shared"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, renderer := namedFixture(t)
			matrixFixture(t, tc.extra)
			_, err := Prepare(cfg, opts, renderer)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
			assertNothingWritten(t, cfg, opts.File)
			assert.NoFileExists(t, filepath.Join(cfg.BasePathAbsolute, "stacks", "deploy", "extra-east.yaml"))
		})
	}
	t.Run("valid", func(t *testing.T) {
		cfg, renderer := namedFixture(t)
		matrixFixture(t, "# atmos:template\nvars:\n  stage: {{ .matrix.region }}\ncomponents:\n  terraform:\n    app:\n      vars: {}\n")
		plan, err := Prepare(cfg, opts, renderer)
		require.NoError(t, err)
		require.NoError(t, plan.Apply(context.Background(), nil))
		assert.FileExists(t, filepath.Join(cfg.BasePathAbsolute, "stacks", "deploy", "extra-west.yaml"))
	})
}

func TestPreflightLeavesNoTemporaryFilesBehind(t *testing.T) {
	cfg, renderer := namedFixture(t)
	write(t, filepath.Join(cfg.StacksBaseAbsolutePath, "deploy", "dev.yaml"), existingDev)
	scratch := t.TempDir()
	for _, name := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(name, scratch)
	}
	_, err := Prepare(cfg, &Options{File: "stacks/deploy/bare.yaml", Components: []string{"terraform:vpc"}, Defaults: true}, renderer)
	require.Error(t, err)
	_, err = Prepare(cfg, &Options{File: "stacks/deploy/ok.yaml", Name: "ok", Defaults: true}, renderer)
	require.NoError(t, err)
	entries, err := os.ReadDir(scratch)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestPreflightBlamesExistingBrokenStacksNotTheScaffold(t *testing.T) {
	cfg, renderer := namedFixture(t)
	write(t, filepath.Join(cfg.StacksBaseAbsolutePath, "deploy", "broken.yaml"), "components:\n  terraform:\n    app:\n      vars: {}\n")
	_, err := Prepare(cfg, &Options{File: "stacks/deploy/ok.yaml", Name: "ok", Defaults: true}, renderer)
	require.ErrorIs(t, err, errUtils.ErrInvalidStackManifest)
	assert.Contains(t, err.Error(), "existing stacks already fail to process")
	assert.Contains(t, hints(err), "atmos validate stacks")
}
