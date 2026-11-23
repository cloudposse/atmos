package acr

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	// fakeACRHelperModeEnv turns the test binary into a fake docker-credential-*
	// helper. "unsupported" mimics a helper whose store and erase are not
	// implemented (it gets registry credentials itself); "fail" is a helper whose
	// store genuinely fails.
	fakeACRHelperModeEnv     = "_ATMOS_TEST_FAKE_ACR_CRED_HELPER_MODE"
	fakeACRHelperUnsupported = "unsupported"
	fakeACRHelperFail        = "fail"
)

// runFakeACRCredentialHelper speaks the docker-credential-helpers protocol. A
// helper that does not implement store prints "not implemented" on stdout and
// exits 1, as docker-credential-helpers' Serve does.
func runFakeACRCredentialHelper(mode string, args []string) int {
	if len(args) != 1 {
		return 2
	}
	_, _ = io.Copy(io.Discard, os.Stdin)
	switch {
	case mode == fakeACRHelperUnsupported && (args[0] == "store" || args[0] == "erase"):
		_, _ = io.WriteString(os.Stdout, "not implemented\n")
		return 1
	case mode == fakeACRHelperUnsupported && args[0] == "get":
		_, _ = io.WriteString(os.Stdout, "credentials not found in native keychain\n")
		return 1
	case mode == fakeACRHelperFail:
		_, _ = io.WriteString(os.Stdout, "keychain is locked\n")
		return 1
	}
	return 0
}

// installFakeACRCredentialHelper puts a copy of the test binary on PATH as
// docker-credential-<name> in the given mode. PATH is replaced, not prepended,
// so no real credential helper can run.
func installFakeACRCredentialHelper(t *testing.T, name, mode string) {
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
	t.Setenv(fakeACRHelperModeEnv, mode)
}
