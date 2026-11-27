package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPackerRunVariableIsolation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dev-image.packer.vars.json")
	require.NoError(t, os.WriteFile(path, []byte("owned by user"), 0o600))
	first, err := writePackerRunVariables(path, map[string]any{"target": "first"})
	require.NoError(t, err)
	second, err := writePackerRunVariables(path, map[string]any{"target": "second"})
	require.NoError(t, err)
	require.NotEqual(t, first, second)
	raw, err := os.ReadFile(first)
	require.NoError(t, err)
	require.JSONEq(t, `{"target":"first"}`, string(raw))
	raw, err = os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "owned by user", string(raw))
	_, err = writePackerRunVariables(filepath.Join(path, "child"), nil)
	require.Error(t, err)
	_, err = writePackerRunVariables(path, map[string]any{"bad": make(chan int)})
	require.Error(t, err)
}
