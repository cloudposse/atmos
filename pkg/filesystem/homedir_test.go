package filesystem

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/config/homedir"
)

func TestNewOSHomeDirProvider(t *testing.T) {
	provider := NewOSHomeDirProvider()
	assert.NotNil(t, provider)
}

func TestOSHomeDirProvider_Dir(t *testing.T) {
	provider := NewOSHomeDirProvider()

	t.Run("returns home directory", func(t *testing.T) {
		dir, err := provider.Dir()
		require.NoError(t, err)
		assert.NotEmpty(t, dir)

		// Verify it's a real directory.
		info, err := os.Stat(dir)
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	})

	t.Run("returns consistent value", func(t *testing.T) {
		dir1, err := provider.Dir()
		require.NoError(t, err)

		dir2, err := provider.Dir()
		require.NoError(t, err)

		assert.Equal(t, dir1, dir2)
	})
}

func TestOSHomeDirProvider_Expand(t *testing.T) {
	provider := NewOSHomeDirProvider()

	t.Run("expands tilde to home directory", func(t *testing.T) {
		expanded, err := provider.Expand("~/test")
		require.NoError(t, err)

		homeDir, err := provider.Dir()
		require.NoError(t, err)

		expected := filepath.Join(homeDir, "test")
		assert.Equal(t, expected, expanded)
	})

	t.Run("returns path unchanged if no tilde", func(t *testing.T) {
		var testPath string
		if runtime.GOOS == "windows" {
			testPath = "C:\\some\\path"
		} else {
			testPath = "/some/path"
		}

		expanded, err := provider.Expand(testPath)
		require.NoError(t, err)
		assert.Equal(t, testPath, expanded)
	})

	t.Run("handles tilde at start only", func(t *testing.T) {
		homeDir, err := provider.Dir()
		require.NoError(t, err)

		// Path with tilde in middle should not be expanded.
		path := "/path/to/~something"
		expanded, err := provider.Expand(path)
		require.NoError(t, err)

		// The tilde in the middle should remain.
		assert.Equal(t, path, expanded)
		assert.NotContains(t, expanded, homeDir)
	})

	t.Run("handles just tilde", func(t *testing.T) {
		expanded, err := provider.Expand("~")
		require.NoError(t, err)

		homeDir, err := provider.Dir()
		require.NoError(t, err)

		assert.Equal(t, homeDir, expanded)
	})

	t.Run("handles tilde with separator", func(t *testing.T) {
		expanded, err := provider.Expand("~/")
		require.NoError(t, err)

		homeDir, err := provider.Dir()
		require.NoError(t, err)

		// Should be home dir with trailing separator or just home dir.
		assert.True(t, strings.HasPrefix(expanded, homeDir))
	})
}

func TestExpandHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	homedir.Reset()
	t.Cleanup(homedir.Reset)

	abs := filepath.Join(home, "abs")

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "tilde alone", in: "~", want: home},
		{name: "tilde slash path", in: "~/x", want: filepath.Join(home, "x")},
		{name: "tilde user unchanged", in: "~user/x", want: "~user/x"},
		{name: "absolute unchanged", in: abs, want: abs},
		{name: "relative unchanged", in: "rel", want: "rel"},
		{name: "empty unchanged", in: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ExpandHome(tt.in))
		})
	}
}
