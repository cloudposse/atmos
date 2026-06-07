package cache

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/config/homedir"
	"github.com/cloudposse/atmos/pkg/perf"
)

// certCommonName is the subject CN used to locate the certificate for removal.
const certCommonName = "Atmos Terraform Registry Cache"

// TrustInstructions returns whether OS trust-store installation is required on this
// platform, plus a human note. On Linux/BSD Atmos trusts the cert via SSL_CERT_FILE
// automatically, so no trust-store change is needed.
func TrustInstructions() (required bool, note string) {
	defer perf.Track(nil, "cache.TrustInstructions")()

	switch runtime.GOOS {
	case "darwin":
		return true, "Installs the certificate into your login keychain (you may be prompted for your password)."
	case "windows":
		return true, "Installs the certificate into your user Root certificate store."
	default:
		return false, "Not required on this platform: Atmos trusts the cache certificate via SSL_CERT_FILE automatically."
	}
}

// InstallTrust adds the proxy certificate to the OS trust store so terraform/tofu
// trust the cache proxy. Used by `atmos terraform cache trust`. It is a no-op (nil)
// on platforms where Atmos handles trust via SSL_CERT_FILE.
func InstallTrust(certPath string) error {
	defer perf.Track(nil, "tfcache.InstallTrust")()

	if _, err := os.Stat(certPath); err != nil {
		return fmt.Errorf("%w: cache certificate not found at %q (run a terraform command with the cache enabled first): %w", errUtils.ErrInvalidConfig, certPath, err)
	}

	switch runtime.GOOS {
	case "darwin":
		keychain, err := loginKeychainPath()
		if err != nil {
			return err
		}
		return runTrustCommand("security", "add-trusted-cert", "-r", "trustRoot", "-k", keychain, certPath)
	case "windows":
		return runTrustCommand("certutil", "-addstore", "-user", "Root", certPath)
	default:
		return nil
	}
}

// RemoveTrust removes the proxy certificate from the OS trust store. Used by
// `atmos terraform cache untrust`.
func RemoveTrust(certPath string) error {
	defer perf.Track(nil, "tfcache.RemoveTrust")()

	switch runtime.GOOS {
	case "darwin":
		return runTrustCommand("security", "remove-trusted-cert", certPath)
	case "windows":
		return runTrustCommand("certutil", "-delstore", "-user", "Root", certCommonName)
	default:
		return nil
	}
}

// loginKeychainPath resolves the user's login keychain.
func loginKeychainPath() (string, error) {
	home, err := homedir.Dir()
	if err != nil {
		return "", fmt.Errorf("%w: resolving home directory: %w", errUtils.ErrInvalidConfig, err)
	}
	return filepath.Join(home, "Library", "Keychains", "login.keychain-db"), nil
}

// runTrustCommand runs an OS trust-store command, surfacing its output on failure.
func runTrustCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin = os.Stdin
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s failed: %w: %s", errUtils.ErrInvalidConfig, name, err, string(out))
	}
	return nil
}
