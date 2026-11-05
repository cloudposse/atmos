package github

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestProvider_WriteEnv(t *testing.T) {
	t.Run("appends single-line and multiline values", func(t *testing.T) {
		envFile := filepath.Join(t.TempDir(), "env")
		t.Setenv("GITHUB_ENV", envFile)

		p := NewProvider()
		require.NoError(t, p.WriteEnv("A", "1"))
		require.NoError(t, p.WriteEnv("B", "x\ny"))

		got, err := os.ReadFile(envFile)
		require.NoError(t, err)
		assert.Equal(t, "A=1\nB<<EOF\nx\ny\nEOF\n", string(got))
	})

	t.Run("unset GITHUB_ENV fails without creating a file", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("GITHUB_ENV", "")

		err := NewProvider().WriteEnv("A", "1")
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrCIEnvWriteFailed)

		entries, readErr := os.ReadDir(dir)
		require.NoError(t, readErr)
		assert.Empty(t, entries)
	})

	t.Run("unwritable path wraps the sentinel", func(t *testing.T) {
		t.Setenv("GITHUB_ENV", filepath.Join(t.TempDir(), "missing-dir", "env"))

		err := NewProvider().WriteEnv("A", "1")
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrCIEnvWriteFailed)
	})
}

func TestProvider_AddPath(t *testing.T) {
	t.Run("appends directories one per line", func(t *testing.T) {
		pathFile := filepath.Join(t.TempDir(), "path")
		t.Setenv("GITHUB_PATH", pathFile)
		dirA := filepath.Join(t.TempDir(), "bin")
		dirB := filepath.Join(t.TempDir(), "tools")

		p := NewProvider()
		require.NoError(t, p.AddPath(dirA))
		require.NoError(t, p.AddPath(dirB))

		got, err := os.ReadFile(pathFile)
		require.NoError(t, err)
		assert.Equal(t, dirA+"\n"+dirB+"\n", string(got))
	})

	t.Run("unset GITHUB_PATH fails", func(t *testing.T) {
		t.Setenv("GITHUB_PATH", "")

		err := NewProvider().AddPath(filepath.Join(t.TempDir(), "bin"))
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrCIEnvWriteFailed)
	})
}

func TestProvider_AddPathRejectsNewlines(t *testing.T) {
	for _, dir := range []string{"safe\nother", "safe\rother", "safe\r\nother"} {
		t.Run(dir, func(t *testing.T) {
			pathFile := filepath.Join(t.TempDir(), "path")
			t.Setenv("GITHUB_PATH", pathFile)
			require.NoError(t, os.WriteFile(pathFile, []byte("existing\n"), 0o600))
			require.ErrorIs(t, NewProvider().AddPath(dir), errUtils.ErrCIEnvWriteFailed)
			got, err := os.ReadFile(pathFile)
			require.NoError(t, err)
			assert.Equal(t, "existing\n", string(got), "rejected paths must not change the runner file")
		})
	}
}

func TestProvider_WriteEnvRejectsInvalidNames(t *testing.T) {
	for _, name := range []string{"", "A\nINJECTED", "A\rINJECTED", "A\r\nINJECTED", "A=B", "A<<EOF", "A\x00B", "A B", "1A", "A-B"} {
		t.Run(name, func(t *testing.T) {
			for _, value := range []string{"value", "multi\nline"} {
				envFile := filepath.Join(t.TempDir(), "env")
				t.Setenv("GITHUB_ENV", envFile)
				require.NoError(t, os.WriteFile(envFile, []byte("EXISTING=value\n"), 0o600))
				assert.ErrorIs(t, NewProvider().WriteEnv(name, value), errUtils.ErrCIEnvWriteFailed)
				got, err := os.ReadFile(envFile)
				require.NoError(t, err)
				assert.Equal(t, "EXISTING=value\n", string(got), "invalid names must not change the runner file")
			}
		})
	}
	t.Run("does not create a file", func(t *testing.T) {
		envFile := filepath.Join(t.TempDir(), "env")
		t.Setenv("GITHUB_ENV", envFile)
		require.ErrorIs(t, NewProvider().WriteEnv("A=B", "value"), errUtils.ErrCIEnvWriteFailed)
		_, err := os.Stat(envFile)
		require.ErrorIs(t, err, os.ErrNotExist)
	})
}

func TestProvider_WriteEnvAcceptsPortableNames(t *testing.T) {
	envFile := filepath.Join(t.TempDir(), "env")
	t.Setenv("GITHUB_ENV", envFile)
	for _, name := range []string{"_", "_lower123", "UPPER_CASE", "mixedCase42"} {
		require.NoError(t, NewProvider().WriteEnv(name, "value"))
	}
	got, err := os.ReadFile(envFile)
	require.NoError(t, err)
	assert.Equal(t, "_=value\n_lower123=value\nUPPER_CASE=value\nmixedCase42=value\n", string(got))
}
