package cache

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

type recordedTrustCommand struct {
	name string
	args []string
}

func forceTrustPlatform(t *testing.T, goos string, fn func(string, ...string) error) *[]recordedTrustCommand {
	t.Helper()
	prevGOOS := trustRuntimeGOOS
	prevRun := runTrustCommandFunc
	commands := []recordedTrustCommand{}
	trustRuntimeGOOS = goos
	runTrustCommandFunc = func(name string, args ...string) error {
		commands = append(commands, recordedTrustCommand{name: name, args: append([]string(nil), args...)})
		if fn != nil {
			return fn(name, args...)
		}
		return nil
	}
	t.Cleanup(func() {
		trustRuntimeGOOS = prevGOOS
		runTrustCommandFunc = prevRun
	})
	return &commands
}

func forceWindowsTrustFuncs(t *testing.T, install func(string) error, remove func(string) error) {
	t.Helper()
	prevInstall := installWindowsTrustFunc
	prevRemove := removeWindowsTrustFunc
	installWindowsTrustFunc = install
	removeWindowsTrustFunc = remove
	t.Cleanup(func() {
		installWindowsTrustFunc = prevInstall
		removeWindowsTrustFunc = prevRemove
	})
}

// TestMain gates the test binary so tests can use it as a cross-platform subprocess:
// with _ATMOS_TEST_EXIT_ONE the process exits 1 (a failing trust command), with
// _ATMOS_TEST_EXIT_ZERO it exits 0 (a succeeding one), and with _ATMOS_TEST_HTTPS_PROBE_URL
// it probes that HTTPS URL with Go's default client (exit 0 on success, 1 on any error)
// so a parent test can observe whether SSL_CERT_FILE is honored on this platform.
// Without any gate it runs normally.
func TestMain(m *testing.M) {
	if os.Getenv("_ATMOS_TEST_BLOCK_TRUST_COMMAND") == "1" {
		time.Sleep(10 * time.Second)
		os.Exit(0)
	}
	if os.Getenv("_ATMOS_TEST_EXIT_ONE") == "1" {
		os.Exit(1)
	}
	if os.Getenv("_ATMOS_TEST_EXIT_ZERO") == "1" {
		os.Exit(0)
	}
	if url := os.Getenv("_ATMOS_TEST_HTTPS_PROBE_URL"); url != "" {
		os.Exit(runHTTPSProbe(url))
	}
	os.Exit(m.Run())
}

// runHTTPSProbe performs an HTTPS GET against url with Go's default client (which uses
// the platform verifier on macOS/Windows and honors SSL_CERT_FILE on Linux/BSD),
// returning 0 on success and 1 on any error. It runs in a re-exec'd subprocess so the
// SSL_CERT_FILE the parent sets is read fresh, before Go caches the system cert pool.
func runHTTPSProbe(url string) int {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url) //nolint:noctx // tiny loopback probe in a throwaway subprocess.
	if err != nil {
		return 1
	}
	_ = resp.Body.Close()
	return 0
}

func TestTrustInstructions(t *testing.T) {
	required, note := TrustInstructions()
	assert.NotEmpty(t, note)
	switch runtime.GOOS {
	case "darwin":
		assert.True(t, required)
		assert.Contains(t, note, "keychain")
	case "windows":
		assert.True(t, required)
		assert.Contains(t, note, "Root")
	default:
		assert.False(t, required)
		assert.Contains(t, note, "SSL_CERT_FILE")
	}
}

func TestInstallTrust_CertNotFound(t *testing.T) {
	// Missing cert is rejected before any OS trust-store call, on every platform.
	err := InstallTrust(filepath.Join(t.TempDir(), "missing.pem"))
	require.ErrorIs(t, err, errUtils.ErrInvalidConfig)
}

func TestInstallTrust_NoopWhenTrustNotRequired(t *testing.T) {
	if required, _ := TrustInstructions(); required {
		t.Skip("platform performs a real (potentially prompting) OS trust-store install")
	}
	cert := filepath.Join(t.TempDir(), "cert.pem")
	require.NoError(t, os.WriteFile(cert, []byte("placeholder"), tlsCertPerm))
	assert.NoError(t, InstallTrust(cert))
}

func TestInstallTrust_WindowsUsesCurrentUserRootStore(t *testing.T) {
	forceTrustPlatform(t, "windows", nil)
	cert := filepath.Join(t.TempDir(), "cert.pem")
	require.NoError(t, os.WriteFile(cert, []byte("placeholder"), tlsCertPerm))
	var gotPath string
	forceWindowsTrustFuncs(t, func(path string) error {
		gotPath = path
		return nil
	}, nil)

	require.NoError(t, InstallTrust(cert))
	assert.Equal(t, cert, gotPath)
}

func TestRemoveTrust_NoopWhenTrustNotRequired(t *testing.T) {
	if required, _ := TrustInstructions(); required {
		t.Skip("platform performs a real (potentially prompting) OS trust-store removal")
	}
	assert.NoError(t, RemoveTrust(filepath.Join(t.TempDir(), "missing.pem")))
}

func TestRemoveTrust_WindowsUsesCurrentUserRootStore(t *testing.T) {
	forceTrustPlatform(t, "windows", nil)
	cert := filepath.Join(t.TempDir(), "cert.pem")
	var gotCommonName string
	forceWindowsTrustFuncs(t, nil, func(commonName string) error {
		gotCommonName = commonName
		return nil
	})

	require.NoError(t, RemoveTrust(cert))
	assert.Equal(t, certCommonName, gotCommonName)
}

func TestRunTrustCommand(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)

	t.Run("success", func(t *testing.T) {
		t.Setenv("_ATMOS_TEST_EXIT_ZERO", "1")
		assert.NoError(t, runTrustCommand(exe))
	})

	t.Run("failure surfaces output", func(t *testing.T) {
		t.Setenv("_ATMOS_TEST_EXIT_ONE", "1")
		err := runTrustCommand(exe)
		require.ErrorIs(t, err, errUtils.ErrInvalidConfig)
	})

	t.Run("timeout surfaces actionable error", func(t *testing.T) {
		prevTimeout := trustCommandTimeout
		trustCommandTimeout = 50 * time.Millisecond
		t.Cleanup(func() {
			trustCommandTimeout = prevTimeout
		})

		t.Setenv("_ATMOS_TEST_BLOCK_TRUST_COMMAND", "1")
		err := runTrustCommand(exe)
		require.ErrorIs(t, err, errUtils.ErrInvalidConfig)
		assert.Contains(t, err.Error(), "timed out after")
	})
}

func TestLoginKeychainPath(t *testing.T) {
	path, err := loginKeychainPath()
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(filepath.ToSlash(path), "Library/Keychains/login.keychain-db"))
}
