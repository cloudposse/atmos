package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCachedTestToolForBinary(t *testing.T) {
	require.NotEmpty(t, cachedTestTools, "cachedTestTools must be populated for this test to be meaningful")

	for _, tool := range cachedTestTools {
		t.Run("found_"+tool.Binary, func(t *testing.T) {
			got, ok := cachedTestToolForBinary(tool.Binary)
			require.True(t, ok)
			assert.Equal(t, tool.Repo, got.Repo)
			assert.Equal(t, tool.Version, got.Version)
			assert.Equal(t, tool.Binary, got.Binary)
		})
	}

	t.Run("not found", func(t *testing.T) {
		_, ok := cachedTestToolForBinary("definitely-not-a-known-tool")
		assert.False(t, ok)
	})
}

// withFakeCachedTool appends a fake tool to cachedTestTools for the duration of the test,
// resets its path lock so the sync.Once fires fresh, and returns the expected bin directory.
func withFakeCachedTool(t *testing.T, repo, version, binary string) string {
	t.Helper()

	orig := cachedTestTools
	cachedTestTools = append(append([]cachedTestTool{}, orig...), cachedTestTool{Repo: repo, Version: version, Binary: binary})
	t.Cleanup(func() { cachedTestTools = orig })

	// Ensure the per-binary sync.Once is fresh even under -count>1.
	testToolPathLocks.Delete(binary)
	t.Cleanup(func() { testToolPathLocks.Delete(binary) })

	cacheDir, err := os.UserCacheDir()
	require.NoError(t, err)
	return filepath.Join(cacheDir, "atmos", "test-toolchain", "bin", repo, version)
}

func TestPrependCachedTestTool(t *testing.T) {
	t.Run("unknown binary is a no-op", func(t *testing.T) {
		origPath := os.Getenv("PATH")
		t.Cleanup(func() { os.Setenv("PATH", origPath) })

		prependCachedTestTool("definitely-not-a-known-tool")
		assert.Equal(t, origPath, os.Getenv("PATH"))
	})

	t.Run("missing cached binary leaves PATH unchanged", func(t *testing.T) {
		_ = withFakeCachedTool(t, "atmos-precond-fake-missing", "v0", "atmos-fake-missing")

		origPath := os.Getenv("PATH")
		t.Cleanup(func() { os.Setenv("PATH", origPath) })

		// The binary file is never created, so the os.Stat guard returns early.
		prependCachedTestTool("atmos-fake-missing")
		assert.Equal(t, origPath, os.Getenv("PATH"))
	})

	t.Run("present cached binary is prepended to non-empty PATH", func(t *testing.T) {
		binDir := withFakeCachedTool(t, "atmos-precond-fake-present", "v0", "atmos-fake-present")
		require.NoError(t, os.MkdirAll(binDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(binDir, "atmos-fake-present"), []byte("fake\n"), 0o755))
		t.Cleanup(func() { os.RemoveAll(binDir) })

		// t.Setenv records the original PATH and restores it after the test; the function
		// under test mutates PATH via os.Setenv, which the restore undoes. Use a temp dir
		// for the existing PATH entry so the expectation is OS-agnostic.
		existingPath := filepath.Join(t.TempDir(), "existing-bin")
		require.NoError(t, os.MkdirAll(existingPath, 0o755))
		t.Setenv("PATH", existingPath)

		prependCachedTestTool("atmos-fake-present")
		assert.Equal(t, binDir+string(os.PathListSeparator)+existingPath, os.Getenv("PATH"))
	})

	t.Run("present cached binary sets PATH when PATH is empty", func(t *testing.T) {
		binDir := withFakeCachedTool(t, "atmos-precond-fake-emptypath", "v0", "atmos-fake-emptypath")
		require.NoError(t, os.MkdirAll(binDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(binDir, "atmos-fake-emptypath"), []byte("fake\n"), 0o755))
		t.Cleanup(func() { os.RemoveAll(binDir) })

		t.Setenv("PATH", "")

		prependCachedTestTool("atmos-fake-emptypath")
		assert.Equal(t, binDir, os.Getenv("PATH"))
		assert.False(t, strings.Contains(os.Getenv("PATH"), string(os.PathListSeparator)))
	})
}
