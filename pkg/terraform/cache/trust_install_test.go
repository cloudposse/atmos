package cache

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// TestMain gates the test binary so tests can use it as a cross-platform subprocess:
// with _ATMOS_TEST_EXIT_ONE the process exits 1 (a failing trust command), with
// _ATMOS_TEST_EXIT_ZERO it exits 0 (a succeeding one). Without either it runs normally.
func TestMain(m *testing.M) {
	if os.Getenv("_ATMOS_TEST_EXIT_ONE") == "1" {
		os.Exit(1)
	}
	if os.Getenv("_ATMOS_TEST_EXIT_ZERO") == "1" {
		os.Exit(0)
	}
	os.Exit(m.Run())
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

func TestRemoveTrust_NoopWhenTrustNotRequired(t *testing.T) {
	if required, _ := TrustInstructions(); required {
		t.Skip("platform performs a real (potentially prompting) OS trust-store removal")
	}
	assert.NoError(t, RemoveTrust(filepath.Join(t.TempDir(), "missing.pem")))
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
}

func TestLoginKeychainPath(t *testing.T) {
	path, err := loginKeychainPath()
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(filepath.ToSlash(path), "Library/Keychains/login.keychain-db"))
}
