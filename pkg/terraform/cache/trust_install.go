package cache

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/config/homedir"
	"github.com/cloudposse/atmos/pkg/perf"
)

// certCommonName is the subject CN used to locate the certificate for removal.
const certCommonName = "Atmos Terraform Registry Cache"

const (
	macosSecurityCommand    = "security"
	macosSystemKeychainPath = "/Library/Keychains/System.keychain"
)

type windowsTrustStoreScope string

const (
	windowsTrustStoreCurrentUser  windowsTrustStoreScope = "CurrentUser"
	windowsTrustStoreLocalMachine windowsTrustStoreScope = "LocalMachine"
)

var (
	trustRuntimeGOOS        = runtime.GOOS
	runTrustCommandFunc     = runTrustCommand
	installWindowsTrustFunc = installWindowsTrust
	removeWindowsTrustFunc  = removeWindowsTrust
	trustCommandTimeout     = 30 * time.Second
	isGitHubActionsFunc     = isGitHubActions
)

// TrustInstructions returns whether OS trust-store installation is required on this
// platform, plus a human note. On Linux/BSD Atmos trusts the cert via SSL_CERT_FILE
// automatically, so no trust-store change is needed.
func TrustInstructions() (required bool, note string) {
	defer perf.Track(nil, "cache.TrustInstructions")()

	switch trustRuntimeGOOS {
	case "darwin":
		if macOSUsesSystemTrustStore() {
			return true, "Installs the certificate into the macOS System keychain."
		}
		return true, "Installs the certificate into your login keychain (you may be prompted for your password)."
	case "windows":
		return true, fmt.Sprintf("Installs the certificate into the Windows %s Root certificate store.", windowsTrustStoreScopeForInstall())
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

	switch trustRuntimeGOOS {
	case "darwin":
		return installMacOSTrust(certPath)
	case "windows":
		if windowsUsesCertutilTrustCommand() {
			return runTrustCommandFunc("certutil", "-addstore", "-enterprise", "-f", "Root", certPath)
		}
		return runTrustOperation("Windows trust store install", func() error {
			return installWindowsTrustFunc(certPath)
		})
	default:
		return nil
	}
}

// RemoveTrust removes the proxy certificate from the OS trust store. Used by
// `atmos terraform cache untrust`.
func RemoveTrust(certPath string) error {
	defer perf.Track(nil, "tfcache.RemoveTrust")()

	switch trustRuntimeGOOS {
	case "darwin":
		return removeMacOSTrust(certPath)
	case "windows":
		if windowsUsesCertutilTrustCommand() {
			return runTrustCommandFunc("certutil", "-delstore", "-enterprise", "Root", certCommonName)
		}
		return runTrustOperation("Windows trust store removal", func() error {
			return removeWindowsTrustFunc(certPath)
		})
	default:
		return nil
	}
}

func installMacOSTrust(certPath string) error {
	if macOSUsesSystemTrustStore() {
		err := runTrustCommandFunc("sudo", macosSecurityCommand, "add-trusted-cert", "-d", "-r", "trustRoot", "-p", "ssl", "-k", macosSystemKeychainPath, certPath)
		if err == nil {
			return nil
		}
		if fallbackErr := installMacOSLoginKeychainTrust(certPath); fallbackErr != nil {
			return fmt.Errorf("%w; fallback to login keychain failed: %w", err, fallbackErr)
		}
		return nil
	}
	return installMacOSLoginKeychainTrust(certPath)
}

func installMacOSLoginKeychainTrust(certPath string) error {
	keychain, err := loginKeychainPath()
	if err != nil {
		return err
	}
	return runTrustCommandFunc(macosSecurityCommand, "add-trusted-cert", "-r", "trustRoot", "-k", keychain, certPath)
}

func removeMacOSTrust(certPath string) error {
	if macOSUsesSystemTrustStore() {
		err := runTrustCommandFunc("sudo", macosSecurityCommand, "delete-certificate", "-c", certCommonName, macosSystemKeychainPath)
		if err == nil {
			return nil
		}
		if fallbackErr := runTrustCommandFunc(macosSecurityCommand, "remove-trusted-cert", certPath); fallbackErr != nil {
			return fmt.Errorf("%w; fallback to login keychain failed: %w", err, fallbackErr)
		}
		return nil
	}
	return runTrustCommandFunc(macosSecurityCommand, "remove-trusted-cert", certPath)
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
	ctx, cancel := context.WithTimeout(context.Background(), trustCommandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = os.Stdin
	out, err := cmd.CombinedOutput()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("%w: %s timed out after %s: %w: %s", errUtils.ErrInvalidConfig, name, trustCommandTimeout, err, string(out))
		}
		return fmt.Errorf("%w: %s failed: %w: %s", errUtils.ErrInvalidConfig, name, err, string(out))
	}
	return nil
}

func runTrustOperation(name string, fn func() error) error {
	done := make(chan error, 1)
	go func() {
		done <- fn()
	}()

	timer := time.NewTimer(trustCommandTimeout)
	defer timer.Stop()

	select {
	case err := <-done:
		return err
	case <-timer.C:
		return fmt.Errorf("%w: %s timed out after %s", errUtils.ErrInvalidConfig, name, trustCommandTimeout)
	}
}

func windowsTrustStoreScopeForInstall() windowsTrustStoreScope {
	if isGitHubActionsFunc() {
		return windowsTrustStoreLocalMachine
	}
	return windowsTrustStoreCurrentUser
}

func windowsUsesCertutilTrustCommand() bool {
	return isGitHubActionsFunc()
}

func macOSUsesSystemTrustStore() bool {
	return isGitHubActionsFunc()
}

func isGitHubActions() bool {
	return os.Getenv("GITHUB_ACTIONS") == "true" ||
		os.Getenv("ACTIONS_ORCHESTRATION_ID") != "" ||
		os.Getenv("ACTIONS_RUNNER_RETURN_JOB_RESULT_FOR_HOSTED") != ""
}
