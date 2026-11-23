package aws

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeHelperModeEnv turns the test binary into a fake docker-credential-*
// helper: "not-implemented" replies like docker-credential-ecr-login (which
// cannot store), "fail" is a helper whose store genuinely fails.
const fakeHelperModeEnv = "_ATMOS_TEST_FAKE_DOCKER_CRED_HELPER_REPLY"

func TestMain(m *testing.M) {
	// When re-executed as a fake credential helper, speak the helper protocol
	// instead of running the tests, so no real helper ever runs.
	if mode := os.Getenv(fakeHelperModeEnv); mode != "" {
		_, _ = io.Copy(io.Discard, os.Stdin)
		if mode == "fail" {
			_, _ = io.WriteString(os.Stdout, "keychain is locked\n")
		} else {
			_, _ = io.WriteString(os.Stdout, "not implemented\n")
		}
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// installFakeCredentialHelper puts the test binary on PATH (replacing it) as
// docker-credential-<name> and writes config.json under a fresh DOCKER_CONFIG.
// It returns the config.json path.
func installFakeCredentialHelper(t *testing.T, name, mode, configJSON string) string {
	t.Helper()

	exe, err := os.Executable()
	require.NoError(t, err)
	binDir := t.TempDir()
	dst := filepath.Join(binDir, "docker-credential-"+name)
	if runtime.GOOS == "windows" {
		dst += ".exe"
	}
	if err := os.Link(exe, dst); err != nil {
		data, readErr := os.ReadFile(exe)
		require.NoError(t, readErr)
		require.NoError(t, os.WriteFile(dst, data, 0o700))
	}
	t.Setenv("PATH", binDir)
	t.Setenv(fakeHelperModeEnv, mode)

	dockerDir := t.TempDir()
	t.Setenv("DOCKER_CONFIG", dockerDir)
	path := filepath.Join(dockerDir, "config.json")
	require.NoError(t, os.WriteFile(path, []byte(configJSON), 0o600))
	return path
}
