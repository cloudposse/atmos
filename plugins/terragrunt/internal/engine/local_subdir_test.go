package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLocalModuleSubdirPreservesRootAndIdentity(t *testing.T) {
	request := nativeUnit(t, "")
	base, err := filepath.EvalSymlinks(request.BasePath)
	require.NoError(t, err)
	root := filepath.Join(base, "repository")
	module := filepath.Join(root, "modules", "app")
	require.NoError(t, os.MkdirAll(module, 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "modules", "shared"), 0o755))
	source := "./repository//modules/app"
	encoded, err := json.Marshal(source)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(request.Path, []byte("terraform { source = "+string(encoded)+" }"), 0o644))

	result, err := Resolve(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, root+"//modules/app", result.Source)
	require.Contains(t, result.Folders, root)
	require.NotEqual(t, root, result.WorkingDir)

	request.Phase = "inventory"
	inventory, err := Resolve(t.Context(), request)
	require.NoError(t, err)
	require.Len(t, inventory.Inventory, 1)
	require.Equal(t, root+"//modules/app", inventory.Inventory[0].Config.Source)
}

func TestLocalModuleSubdirRejectsTraversal(t *testing.T) {
	request := nativeUnit(t, "")
	root := filepath.Join(request.BasePath, "repository")
	require.NoError(t, os.MkdirAll(root, 0o755))
	for _, source := range []string{"./repository//../outside", "./repository//modules/../outside", "./repository///absolute"} {
		encoded, err := json.Marshal(source)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(request.Path, []byte("terraform { source = "+string(encoded)+" }"), 0o644))
		_, err = Resolve(t.Context(), request)
		require.ErrorContains(t, err, "subdirectory")
	}
}
