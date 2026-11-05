package config

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/cloudposse/atmos/pkg/script/starlark/internal/threadcontext"
)

// Host I/O failures retain their cause and source filename in script diagnostics.
func TestConfigFileReadFailure(t *testing.T) {
	t.Parallel()
	failure := errors.New("test file read failed")
	filename := filepath.Join(t.TempDir(), "app.yaml")
	mod := New(Host{
		ResolvePath: func(_ *starlark.Thread, name string) string { return filename },
		ReadFile: func(path string) ([]byte, error) {
			assert.Equal(t, filename, path)
			return nil, failure
		},
	}).(*starlarkstruct.Module)
	result, err := starlark.Call(&starlark.Thread{}, mod.Members["load"], starlark.Tuple{starlark.String("app.yaml"), starlark.NewDict(0)}, nil)
	require.ErrorIs(t, err, failure)
	assert.ErrorContains(t, err, filename)
	assert.Nil(t, result)
}

// Cancellation must prevent I/O and inline validation even on a native caller's thread.
func TestConfigCanceledBeforeCall(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	thread := &starlark.Thread{}
	threadcontext.Set(ctx, thread)
	mod := New(Host{
		ResolvePath: func(_ *starlark.Thread, name string) string { return name },
		ReadFile:    func(string) ([]byte, error) { t.Error("unexpected file read"); return nil, nil },
	}).(*starlarkstruct.Module)
	for _, tc := range []struct {
		name string
		args starlark.Tuple
	}{
		{"load", starlark.Tuple{starlark.String("app.yaml"), starlark.NewDict(0)}},
		{"validate", starlark.Tuple{starlark.MakeInt(1), starlark.Universe["int"]}},
		{"validate", starlark.Tuple{starlark.MakeInt(1), starlark.String("schema.json")}},
	} {
		result, err := starlark.Call(thread, mod.Members[tc.name], tc.args, nil)
		require.ErrorIs(t, err, context.Canceled)
		assert.Nil(t, result)
	}
	_, err := parseConfig(ctx, []byte("port: 42"), "yaml")
	require.ErrorIs(t, err, context.Canceled)
}
