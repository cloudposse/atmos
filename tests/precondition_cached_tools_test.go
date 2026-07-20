package tests

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCachedTestToolBinaryNameForOS(t *testing.T) {
	tests := []struct {
		name   string
		binary string
		goos   string
		want   string
	}{
		{name: "windows appends exe", binary: "tofu", goos: "windows", want: "tofu.exe"},
		{name: "linux preserves name", binary: "tofu", goos: "linux", want: "tofu"},
		{name: "darwin preserves name", binary: "tofu", goos: "darwin", want: "tofu"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, cachedTestToolBinaryNameForOS(tt.binary, tt.goos))
		})
	}
}

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

func TestCachedTestToolBinaryPath(t *testing.T) {
	tests := []struct {
		name     string
		binary   string
		file     string
		wantPath string
		wantOK   bool
	}{
		{
			name:     "native binary",
			binary:   "atmos-fake",
			file:     "atmos-fake",
			wantPath: "atmos-fake",
			wantOK:   true,
		},
		{
			name:     "Windows executable",
			binary:   "atmos-fake",
			file:     "atmos-fake.exe",
			wantPath: "atmos-fake.exe",
			wantOK:   true,
		},
		{
			name:   "missing binary",
			binary: "atmos-fake",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			binDir := t.TempDir()
			if tt.file != "" {
				require.NoError(t, os.WriteFile(filepath.Join(binDir, tt.file), []byte("fake\n"), 0o755))
			}

			got, ok := cachedTestToolBinaryPath(binDir, tt.binary)
			assert.Equal(t, tt.wantOK, ok)
			if tt.wantOK {
				assert.Equal(t, filepath.Join(binDir, tt.wantPath), got)
			}
		})
	}
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

func TestCachedTestToolBinaryExists(t *testing.T) {
	t.Run("bare name exists", func(t *testing.T) {
		binDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(binDir, "faketool"), []byte("fake\n"), 0o755))
		assert.True(t, cachedTestToolBinaryExists(binDir, "faketool"))
	})

	t.Run("missing binary", func(t *testing.T) {
		assert.False(t, cachedTestToolBinaryExists(t.TempDir(), "faketool"))
	})

	t.Run("exe-suffixed name", func(t *testing.T) {
		binDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(binDir, "faketool.exe"), []byte("fake\n"), 0o755))
		// Toolchain installs write "<binary>.exe" on Windows, so only there does the
		// suffixed lookup apply; elsewhere the bare name is the only valid spelling.
		assert.Equal(t, runtime.GOOS == "windows", cachedTestToolBinaryExists(binDir, "faketool"))
	})
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
		require.NoError(t, os.WriteFile(filepath.Join(binDir, cachedTestToolBinaryName("atmos-fake-present")), []byte("fake\n"), 0o755))
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
		require.NoError(t, os.WriteFile(filepath.Join(binDir, cachedTestToolBinaryName("atmos-fake-emptypath")), []byte("fake\n"), 0o755))
		t.Cleanup(func() { os.RemoveAll(binDir) })

		t.Setenv("PATH", "")

		prependCachedTestTool("atmos-fake-emptypath")
		assert.Equal(t, binDir, os.Getenv("PATH"))
		assert.False(t, strings.Contains(os.Getenv("PATH"), string(os.PathListSeparator)))
	})
}
