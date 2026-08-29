package generator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRemoveGenerated tests the RemoveGenerated function.
func TestRemoveGenerated(t *testing.T) {
	t.Run("removes an existing file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "providers_override.tf.json")
		require.NoError(t, os.WriteFile(path, []byte(`{"provider":{}}`), 0o600))

		require.NoError(t, RemoveGenerated(dir, "providers_override.tf.json"))

		_, err := os.Stat(path)
		assert.True(t, os.IsNotExist(err), "file should be removed")
	})

	t.Run("missing file is not an error", func(t *testing.T) {
		require.NoError(t, RemoveGenerated(t.TempDir(), "providers_override.tf.json"))
	})

	t.Run("leaves sibling files alone", func(t *testing.T) {
		dir := t.TempDir()
		sibling := filepath.Join(dir, "backend.tf.json")
		require.NoError(t, os.WriteFile(sibling, []byte(`{"terraform":{}}`), 0o600))

		require.NoError(t, RemoveGenerated(dir, "providers_override.tf.json"))

		_, err := os.Stat(sibling)
		assert.NoError(t, err, "unrelated generated files must be preserved")
	})

	t.Run("returns error when the target is a non-empty directory", func(t *testing.T) {
		dir := t.TempDir()
		nested := filepath.Join(dir, "providers_override.tf.json")
		require.NoError(t, os.Mkdir(nested, 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(nested, "child"), []byte("x"), 0o600))

		assert.Error(t, RemoveGenerated(dir, "providers_override.tf.json"))
	})
}
