package tests

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPTYErrorWrapped(t *testing.T) {
	t.Parallel()
	eof := &os.PathError{Op: "read", Path: "pty", Err: syscall.EIO}
	require.NoError(t, ptyError(fmt.Errorf("read output: %w", eof)))
	failure := &os.PathError{Op: "read", Path: "pty", Err: syscall.EBADF}
	wrapped := fmt.Errorf("read output: %w", failure)
	assert.ErrorIs(t, ptyError(wrapped), syscall.EBADF)
	// Bare EIO is not necessarily a closed PTY.
	assert.ErrorIs(t, ptyError(syscall.EIO), syscall.EIO)
	require.NoError(t, ptyError(nil))
}

func TestLoadTestSuitesPreservesReadError(t *testing.T) {
	t.Parallel()
	_, err := loadTestSuites(filepath.Join(t.TempDir(), "missing"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestSetupTestConfigPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "atmos.yaml"), nil, 0o600))
	for _, args := range [][]string{nil, {"--chdir", "elsewhere"}, {"-C", "elsewhere"}, {"--chdir=elsewhere"}} {
		tc := TestCase{Args: args}
		setupTestConfigPath(&tc, dir)
		if len(args) == 0 {
			assert.Equal(t, dir, tc.Env["ATMOS_CLI_CONFIG_PATH"])
		} else {
			assert.NotContains(t, tc.Env, "ATMOS_CLI_CONFIG_PATH")
		}
	}
	tc := TestCase{}
	setupTestConfigPath(&tc, t.TempDir())
	assert.NotContains(t, tc.Env, "ATMOS_CLI_CONFIG_PATH")
}
