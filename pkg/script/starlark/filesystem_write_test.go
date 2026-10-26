package starlark

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/script"
)

func TestWriteFilePathsAndOverwrite(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "data.json")
	run := func(source string) (script.Result, error) {
		return New().Execute(t.Context(), script.Spec{WorkingDirectory: dir, Env: map[string]string{"ABS": path}, Source: source})
	}
	result, err := run(`fs.write_file("data.json", "café\n")
output = fs.read_file(env["ABS"])`)
	require.NoError(t, err)
	assert.Equal(t, "café\n", result.Value)
	info, err := os.Stat(path)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
	_, err = run(`fs.write_file(env["ABS"], "replacement")`)
	require.ErrorIs(t, err, os.ErrExist)
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "café\n", string(content))
	result, err = run(`fs.write_file(env["ABS"], "new", overwrite=True, mode=0o644)
output = fs.read_file("data.json")`)
	require.NoError(t, err)
	assert.Equal(t, "new", result.Value)
	info, err = os.Stat(path)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
	_, err = run(`fs.write_file("missing/child", "data")`)
	require.ErrorIs(t, err, os.ErrNotExist)
	for _, source := range []string{`fs.write_file("", "data")`, `fs.write_file("bad", "data", mode=-1)`, `fs.write_file("bad", "data", mode=0o1000)`, `fs.write_file("bad", 42)`} {
		_, err = run(source)
		require.Error(t, err)
	}
}

func TestWriteFileDryRunAndCancellation(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	source := `fs.write_file("unexpected", "secret")`
	_, err := New().Execute(t.Context(), script.Spec{WorkingDirectory: dir, Source: source, DryRun: true})
	require.NoError(t, err)
	require.NoFileExists(t, filepath.Join(dir, "unexpected"))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = New().Execute(ctx, script.Spec{WorkingDirectory: dir, Source: source})
	require.ErrorIs(t, err, context.Canceled)
	require.NoFileExists(t, filepath.Join(dir, "unexpected"))
}

func TestWriteFileAtomicModesAndBytes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file")
	require.NoError(t, os.WriteFile(path, []byte("old"), 0o755))
	_, err := New().Execute(t.Context(), script.Spec{WorkingDirectory: dir, Env: map[string]string{"DATA": string([]byte{'n', 'e', 'w', 0, 255})}, Source: `fs.write_file("file", env["DATA"], overwrite=True)`})
	require.NoError(t, err)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, []byte{'n', 'e', 'w', 0, 255}, data)
	info, err := os.Stat(path)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		require.Equal(t, os.FileMode(0o755), info.Mode().Perm())
	}
	if err := os.Symlink(path, filepath.Join(dir, "link")); err != nil {
		t.Skip(err)
	}
	_, err = New().Execute(t.Context(), script.Spec{WorkingDirectory: dir, Source: `fs.write_file("link", "bad", overwrite=True)`})
	require.ErrorContains(t, err, "regular file")
	data, err = os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, []byte{'n', 'e', 'w', 0, 255}, data)
}

func TestYAMLSyntaxOnly(t *testing.T) {
	for _, content := range []string{"", "---\na: !custom value\n---\nb: !env OTHER\n", "? [a,b]\n: c\n", "a: 1\na: 2\n"} {
		_, err := New().Execute(t.Context(), script.Spec{Env: map[string]string{"YAML": content}, Source: `yaml.validate(env["YAML"])`})
		require.NoError(t, err, content)
	}
	_, err := New().Execute(t.Context(), script.Spec{Source: `yaml.validate("a: valid\n---\na: [broken\n")`})
	require.Error(t, err)
}
