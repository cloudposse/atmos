package starlark

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/script"
)

func TestReadFileWorkingDirectoryAndParallel(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "data.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"name":"café"}`), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "lib"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "lib", "read.star"), []byte(`def read():
    return json.decode(fs.read_file(path="data.json"))
`), 0o600))
	result, err := New().Execute(context.Background(), script.Spec{
		WorkingDirectory: dir, Env: map[string]string{"ABSOLUTE": path}, Source: `
load("lib/read.star", "read")
def absolute():
    return json.decode(fs.read_file(env["ABSOLUTE"]))
output = steps.parallel(functions=[read, absolute])
`,
	})
	require.NoError(t, err)
	assert.JSONEq(t, `[{"name":"café"},{"name":"café"}]`, result.Value)
}

func TestReadFileFailuresAndDryRun(t *testing.T) {
	t.Parallel()
	_, err := runSource(t, `fs.read_file("missing")`, WithReadFile(func(string) ([]byte, error) {
		return nil, os.ErrNotExist
	}))
	require.ErrorIs(t, err, errUtils.ErrStarlark)
	assert.Contains(t, err.Error(), "cannot read file")
	for _, source := range []string{`fs.read_file()`, `fs.read_file(1)`, `fs.read_file("a", unexpected=True)`} {
		_, err = runSource(t, source)
		require.ErrorIs(t, err, errUtils.ErrStarlark, source)
	}
	_, err = New(WithReadFile(func(string) ([]byte, error) {
		t.Fatal("dry run must not read files")
		return nil, nil
	})).Execute(context.Background(), script.Spec{Source: `fs.read_file("missing")`, DryRun: true})
	require.NoError(t, err)
}

func TestReadFileCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := New(WithReadFile(func(string) ([]byte, error) {
		cancel()
		return []byte("ignored"), nil
	})).Execute(ctx, script.Spec{Source: `output = fs.read_file("data")`})
	require.ErrorIs(t, err, context.Canceled)
}
