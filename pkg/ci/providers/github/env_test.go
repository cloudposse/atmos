package github

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	ghactions "github.com/cloudposse/atmos/pkg/github/actions"
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
		assert.Equal(t, ghactions.FormatValue("A", "1")+ghactions.FormatValue("B", "x\ny"), string(got))
		assert.Contains(t, string(got), "A=1\n")
		assert.Contains(t, string(got), "B<<ATMOS_EOF_B\nx\ny\nATMOS_EOF_B\n")
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
