package process

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLookPathUsesProvidedEnvironment(t *testing.T) {
	t.Parallel()
	exe, err := os.Executable()
	require.NoError(t, err)
	env := []string{"PATH=" + filepath.Dir(exe), "PATHEXT=" + os.Getenv("PATHEXT")}

	found, err := LookPath(t.TempDir(), env, filepath.Base(exe))
	require.NoError(t, err)
	assert.Equal(t, filepath.Clean(exe), filepath.Clean(found))

	// A PATH without the directory does not fall back to the process environment.
	_, err = LookPath(t.TempDir(), []string{"PATH=" + t.TempDir()}, filepath.Base(exe))
	require.Error(t, err)

	_, err = LookPath(t.TempDir(), env, "atmos-lookpath-missing-tool")
	require.Error(t, err)
}
