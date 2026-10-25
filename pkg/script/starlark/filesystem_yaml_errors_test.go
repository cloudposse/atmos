package starlark

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"

	"github.com/cloudposse/atmos/pkg/script"
	"github.com/cloudposse/atmos/pkg/script/starlark/internal/threadcontext"
)

func TestFileAndYAMLBuiltinsRejectArgumentsAndCancellation(t *testing.T) {
	s := &session{engine: New()}
	for _, tc := range []struct {
		name string
		fn   func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error)
		args starlark.Tuple
	}{
		{"fs.write_file", s.writeFile, starlark.Tuple{starlark.String("file"), starlark.String("data")}},
		{"yaml.decode", decodeYAML, starlark.Tuple{starlark.String("key: value")}},
		{"yaml.validate", validateYAML, starlark.Tuple{starlark.String("key: value")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			thread := &starlark.Thread{Name: tc.name}
			threadcontext.Set(t.Context(), thread)
			builtin := starlark.NewBuiltin(tc.name, tc.fn)
			_, err := starlark.Call(thread, builtin, nil, nil)
			require.Error(t, err)
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			threadcontext.Set(ctx, thread)
			_, err = starlark.Call(thread, builtin, tc.args, nil)
			require.ErrorIs(t, err, context.Canceled)
		})
	}
}

func TestWriteFileCreationAndErrors(t *testing.T) {
	root := t.TempDir()
	_, err := New().Execute(t.Context(), script.Spec{WorkingDirectory: root, Source: `fs.write_file("new", "created\n")`})
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(root, "new"))
	require.NoError(t, err)
	require.Equal(t, "created\n", string(data))
	for _, tc := range []struct{ source, message string }{
		{`fs.write_file("", "data")`, "path must not be empty"},
		{`fs.write_file("\x00", "data")`, "invalid"},
		{`fs.write_file("missing/child", "data")`, ""},
	} {
		_, err := New().Execute(t.Context(), script.Spec{WorkingDirectory: root, Source: tc.source})
		require.Error(t, err)
		if tc.message != "" {
			require.ErrorContains(t, err, tc.message)
		}
	}
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	require.Len(t, entries, 1, "failed atomic writes must not leave temporary files")
}
