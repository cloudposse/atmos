package exec

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helperEnvVar gates the test binary into "helper" mode. When set, the test
// binary acts as a predictable subprocess instead of running the test suite.
// This avoids depending on platform-specific external binaries (e.g. `go`,
// `cmd`, `sh`) per the cross-platform test contract.
const helperEnvVar = "_ATMOS_EXEC_HELPER"

// helperOutput is the fixed string the helper process writes to stdout.
const helperOutput = "atmos-exec-helper-ok"

// TestMain lets the test binary double as a cross-platform helper subprocess.
// When _ATMOS_EXEC_HELPER is set, it prints a known string and exits 0 so that
// CommandContext/LookPath can be exercised without external binaries.
func TestMain(m *testing.M) {
	if os.Getenv(helperEnvVar) != "" {
		// Running as the helper subprocess.
		os.Stdout.WriteString(helperOutput)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestDefaultExecutor(t *testing.T) {
	e := Default()
	require.NotNil(t, e)

	exePath, err := os.Executable()
	require.NoError(t, err)

	t.Run("LookPath finds a known binary", func(t *testing.T) {
		// Put the test binary's directory on PATH and look it up by name,
		// avoiding any dependency on an external binary being installed.
		dir := filepath.Dir(exePath)
		base := filepath.Base(exePath)
		t.Setenv("PATH", dir)

		path, err := e.LookPath(base)
		require.NoError(t, err)
		assert.NotEmpty(t, path)
	})

	t.Run("LookPath errors on a missing binary", func(t *testing.T) {
		_, err := e.LookPath("atmos-definitely-not-a-real-binary-xyz")
		assert.Error(t, err)
	})

	t.Run("CommandContext runs a trivial command", func(t *testing.T) {
		ctx := context.Background()
		// Use the test binary itself as a predictable, cross-platform subprocess.
		cmd := e.CommandContext(ctx, exePath)
		cmd.Env = append(os.Environ(), helperEnvVar+"=1")

		out, err := cmd.Output()
		require.NoError(t, err)
		assert.Equal(t, helperOutput, strings.TrimSpace(string(out)))
	})
}
