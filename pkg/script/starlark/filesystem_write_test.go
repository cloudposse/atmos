package starlark

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/cloudposse/atmos/pkg/script"
	"github.com/stretchr/testify/require"
)

func TestWriteFileAtomicModesAndBytes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file")
	require.NoError(t, os.WriteFile(path, []byte("old"), 0o755))
	_, err := New().Execute(t.Context(), script.Spec{WorkingDirectory: dir, Env: map[string]string{"DATA": string([]byte{'n', 'e', 'w', 0, 255})}, Source: `fs.write_file("file", env["DATA"])`})
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
	_, err = New().Execute(t.Context(), script.Spec{WorkingDirectory: dir, Source: `fs.write_file("link", "bad")`})
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
