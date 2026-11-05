package create

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	cockroach "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/generator/engine"
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
	// The processor lists the manifests in map order, so accept either order.
	// Manifests are sorted, so the message is stable across runs.
	assert.Contains(t, err.Error(), "(deploy/dev, deploy/dev2)")
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

// useTempDir points every platform's temporary directory variable at dir.
func useTempDir(t *testing.T, dir string) {
	t.Helper()
	for _, name := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(name, dir)
	}
}

// skipWithoutFileModeSupport skips tests that rely on a regular file shadowing a directory component.
func skipWithoutFileModeSupport(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Windows reports a path below a regular file as missing instead of failing")
	}
}

func preflightProjectConfig(t *testing.T) (*schema.AtmosConfiguration, string) {
	t.Helper()
	cfg, _ := namedFixture(t)
	return cfg, cfg.BasePathAbsolute
}

func TestPreflightSkipsProjectsWhenNothingLandsUnderStacks(t *testing.T) {
	cfg, root := preflightProjectConfig(t)
	scratch := t.TempDir()
	useTempDir(t, scratch)
	files := []engine.File{{Path: filepath.Join("components", "terraform", "vpc", "main.tf"), Content: "# stub\n"}}

	require.NoError(t, preflightProject(cfg, files, root))
	entries, err := os.ReadDir(scratch)
	require.NoError(t, err)
	assert.Empty(t, entries, "no overlay is built when no rendered file lands under stacks.base_path")
}

func TestPreflightReportsUnusableTemporaryDirectory(t *testing.T) {
	cfg, root := preflightProjectConfig(t)
	useTempDir(t, filepath.Join(t.TempDir(), "missing"))
	files := []engine.File{{Path: filepath.Join("stacks", "deploy", "new.yaml"), Content: "vars:\n  stage: new\n"}}

	err := preflightProject(cfg, files, root)
	require.ErrorIs(t, err, errUtils.ErrCreateTempDir)
	assert.NoFileExists(t, filepath.Join(root, "stacks", "deploy", "new.yaml"))
}

func TestPreflightReportsOverlayFailures(t *testing.T) {
	for _, tc := range []struct {
		name     string
		existing map[string]string
		dirs     []string
		rendered string
		want     error
	}{
		{
			name:     "rendered directory component is a regular file",
			existing: map[string]string{"deploy": "not a directory"},
			rendered: filepath.Join("stacks", "deploy", "new.yaml"),
			want:     errUtils.ErrCreateDirectory,
		},
		{
			name:     "rendered file shadows an existing directory",
			dirs:     []string{filepath.Join("deploy", "new.yaml")},
			rendered: filepath.Join("stacks", "deploy", "new.yaml"),
			want:     errUtils.ErrFileWrite,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, root := preflightProjectConfig(t)
			for name, content := range tc.existing {
				write(t, filepath.Join(cfg.StacksBaseAbsolutePath, name), content)
			}
			for _, dir := range tc.dirs {
				require.NoError(t, os.MkdirAll(filepath.Join(cfg.StacksBaseAbsolutePath, dir), 0o755))
			}
			err := preflightProject(cfg, []engine.File{{Path: tc.rendered, Content: "vars:\n  stage: new\n"}}, root)
			require.ErrorIs(t, err, tc.want)
		})
	}
}

func TestPreflightReportsUnreadableStacksDirectory(t *testing.T) {
	skipWithoutFileModeSupport(t)
	cfg, root := preflightProjectConfig(t)
	// A regular file as the parent of stacks.base_path makes the directory impossible to inspect.
	write(t, filepath.Join(root, "blocker"), "file")
	cfg.StacksBaseAbsolutePath = filepath.Join(root, "blocker", "stacks")
	files := []engine.File{{Path: filepath.Join("blocker", "stacks", "deploy", "new.yaml"), Content: "vars:\n  stage: new\n"}}

	err := preflightProject(cfg, files, root)
	require.ErrorIs(t, err, errUtils.ErrReadFile)
}

func TestStacksBasePathFallsBackToConfiguredBasePath(t *testing.T) {
	cfg := &schema.AtmosConfiguration{BasePathAbsolute: filepath.Join(t.TempDir(), "project"), Stacks: schema.Stacks{BasePath: "stacks"}}
	assert.Equal(t, filepath.Join(cfg.BasePathAbsolute, "stacks"), stacksBasePath(cfg))
	cfg.StacksBaseAbsolutePath = filepath.Join(t.TempDir(), "elsewhere", "stacks")
	assert.Equal(t, cfg.StacksBaseAbsolutePath, stacksBasePath(cfg))
}

func TestCopyTreeSemantics(t *testing.T) {
	t.Run("missing source yields an empty directory", func(t *testing.T) {
		dst := filepath.Join(t.TempDir(), "out")
		require.NoError(t, copyTree(filepath.Join(t.TempDir(), "missing"), dst, 0))
		assert.DirExists(t, dst)
	})
	t.Run("a regular file source copies nothing", func(t *testing.T) {
		src := filepath.Join(t.TempDir(), "file.yaml")
		write(t, src, "vars: {}\n")
		dst := filepath.Join(t.TempDir(), "out")
		require.NoError(t, copyTree(src, dst, 0))
		assert.NoDirExists(t, dst)
	})
	t.Run("symlink nesting beyond the limit is not followed", func(t *testing.T) {
		src := t.TempDir()
		write(t, filepath.Join(src, "a.yaml"), "vars: {}\n")
		dst := filepath.Join(t.TempDir(), "out")
		require.NoError(t, copyTree(src, dst, maxSymlinkDepth+1))
		assert.NoDirExists(t, dst)
		// Negative path: within the limit the same tree is copied.
		require.NoError(t, copyTree(src, dst, maxSymlinkDepth))
		assert.FileExists(t, filepath.Join(dst, "a.yaml"))
	})
	t.Run("destination below a regular file", func(t *testing.T) {
		skipWithoutFileModeSupport(t)
		src := t.TempDir()
		write(t, filepath.Join(src, "a.yaml"), "vars: {}\n")
		blocker := filepath.Join(t.TempDir(), "blocker")
		write(t, blocker, "file")
		require.ErrorIs(t, copyTree(src, filepath.Join(blocker, "out"), 0), errUtils.ErrCreateDirectory)
	})
	t.Run("a copy failure of one entry aborts the tree", func(t *testing.T) {
		src, dst := t.TempDir(), t.TempDir()
		write(t, filepath.Join(src, "a.yaml"), "vars: {}\n")
		// The destination already holds a directory where the file must be written.
		require.NoError(t, os.MkdirAll(filepath.Join(dst, "a.yaml"), 0o755))
		require.ErrorIs(t, copyTree(src, dst, 0), errUtils.ErrFileWrite)
	})
	t.Run("symlinked directories are followed and dangling links are ignored", func(t *testing.T) {
		src, shared := t.TempDir(), t.TempDir()
		write(t, filepath.Join(shared, "common.yaml"), "vars:\n  region: us-east-1\n")
		if err := os.Symlink(shared, filepath.Join(src, "shared")); err != nil {
			t.Skipf("symlinks are unavailable: %v", err)
		}
		require.NoError(t, os.Symlink(filepath.Join(src, "missing"), filepath.Join(src, "dangling")))
		dst := filepath.Join(t.TempDir(), "out")
		require.NoError(t, copyTree(src, dst, 0))
		copied, err := os.ReadFile(filepath.Join(dst, "shared", "common.yaml"))
		require.NoError(t, err)
		assert.Equal(t, "vars:\n  region: us-east-1\n", string(copied))
		assert.NoFileExists(t, filepath.Join(dst, "dangling"))
		assert.NoDirExists(t, filepath.Join(dst, "dangling"))
	})
}

func TestCopyFileReportsEachFailure(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.yaml")
	write(t, source, "vars: {}\n")

	require.ErrorIs(t, copyFile(filepath.Join(dir, "missing.yaml"), filepath.Join(dir, "out.yaml")), errUtils.ErrReadFile)
	require.ErrorIs(t, copyFile(source, filepath.Join(dir, "no-such-dir", "out.yaml")), errUtils.ErrFileWrite)
	// Reading a directory fails after the destination was opened.
	require.ErrorIs(t, copyFile(dir, filepath.Join(t.TempDir(), "out.yaml")), errUtils.ErrFileWrite)

	require.NoError(t, copyFile(source, filepath.Join(dir, "out.yaml")))
	copied, err := os.ReadFile(filepath.Join(dir, "out.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "vars: {}\n", string(copied))
}

func TestPreflightErrorFallsBackToGenericGuidance(t *testing.T) {
	rendered := []renderedStack{{relative: filepath.Join("deploy", "new.yaml"), content: "vars: {}\n"}}
	err := preflightError(errors.New("unexpected processor failure"), rendered, "/overlay", "/stacks")
	require.ErrorIs(t, err, errUtils.ErrInvalidStackManifest)
	assert.Contains(t, err.Error(), "creating deploy/new.yaml would break stack processing: unexpected processor failure")
	assert.Equal(t, "Fix the scaffold output or its answers; nothing was written", hints(err))
}
