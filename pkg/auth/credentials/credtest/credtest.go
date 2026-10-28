// Package credtest isolates tests from the operating-system keychain.
//
// The credential store defaults to the "system" keyring (macOS Keychain, Windows Credential
// Manager, Secret Service). Any test that builds an auth manager or credential store without
// choosing a backend would otherwise read from, write to, or merely probe the developer's real
// keychain, which makes macOS show keychain prompts and can leak state between runs.
//
// This package selects a non-prompting backend purely through the environment variables the
// production resolver already honors (ATMOS_KEYRING_TYPE, ATMOS_KEYRING_PASSWORD,
// ATMOS_KEYRING_FILE_PATH), so production defaults are unchanged. It deliberately imports no
// Atmos packages so that any test package, including pkg/auth/credentials itself, can use it.
//
// Typical usage:
//
//	func TestMain(m *testing.M) { os.Exit(credtest.IsolateKeyringMain(m)) }
//
//	func TestNeedsPersistence(t *testing.T) { credtest.IsolateKeyring(t) }
package credtest

import (
	"os"
	"sync"
	"testing"

	zkeyring "github.com/zalando/go-keyring"
)

const (
	// EnvKeyringType selects the credential-store backend.
	EnvKeyringType = "ATMOS_KEYRING_TYPE"
	// EnvKeyringPassword is the default password variable of the file backend.
	EnvKeyringPassword = "ATMOS_KEYRING_PASSWORD"
	// EnvKeyringPasswordEnv optionally renames the password variable of the file backend.
	EnvKeyringPasswordEnv = "ATMOS_KEYRING_PASSWORD_ENV"
	// EnvKeyringFilePath is the directory of the file backend.
	EnvKeyringFilePath = "ATMOS_KEYRING_FILE_PATH"

	// TypeFile is the encrypted file backend, which persists across credential-store instances.
	TypeFile = "file"
	// TypeSystem is the OS keychain backend that tests must never reach for real.
	TypeSystem = "system"
	// TypeMemory is the in-memory backend, which is private to each credential-store instance.
	TypeMemory = "memory"

	// Password is the throwaway password of the file backend (meets the 8 character minimum).
	Password = "atmos-test-keyring-password"
)

var mockSystemOnce sync.Once

// MockSystemKeyring replaces the OS keychain provider (zalando go-keyring) with an in-process
// mock for the rest of the test binary. It is defense in depth: tests that deliberately select the
// "system" backend, or reach it through a fallback path, exercise the mock and never prompt for
// the real keychain. It is safe to call repeatedly and from parallel tests.
func MockSystemKeyring() {
	mockSystemOnce.Do(zkeyring.MockInit)
}

// IsolateKeyring points the credential store at a fresh encrypted file keyring in a per-test
// temporary directory. Credentials survive across credential-store instances within the test
// (like the real keychain) but never reach the system keychain or another test. The previous
// environment is restored when the test ends.
func IsolateKeyring(tb testing.TB) {
	tb.Helper()

	MockSystemKeyring()
	tb.Setenv(EnvKeyringType, TypeFile)
	tb.Setenv(EnvKeyringPassword, Password)
	// Clear any renamed password variable and point at this test's own directory.
	tb.Setenv(EnvKeyringPasswordEnv, "")
	tb.Setenv(EnvKeyringFilePath, tb.TempDir())
}

// EnsureIsolated isolates the keyring like IsolateKeyring, but only when the environment would
// still resolve to the system keychain (ATMOS_KEYRING_TYPE unset, empty, or "system"). A test or
// TestMain that already chose a backend, including a deliberately invalid one, is left alone, so
// calling this from a shared helper never overrides a keyring the test set up itself.
func EnsureIsolated(tb testing.TB) {
	tb.Helper()

	// Unknown backends fall back to the system keyring without changing their configured name.
	MockSystemKeyring()

	if keyringType := os.Getenv(EnvKeyringType); keyringType != "" && keyringType != TypeSystem { //nolint:forbidigo // Test isolation must inspect the process environment without importing application configuration.
		return
	}
	IsolateKeyring(tb)
}

// IsolateKeyringMain makes every test in the calling package default to the in-memory keyring,
// also mocks the OS keychain provider (see MockSystemKeyring), then runs the package's tests and returns the exit code. Each credential-store instance gets
// its own empty memory keyring, so nothing is shared between tests. Tests that need credentials
// to persist across store instances call IsolateKeyring, and tests that choose a backend
// themselves still override this default through their own environment.
//
// Call it from the package's TestMain: os.Exit(credtest.IsolateKeyringMain(m)).
func IsolateKeyringMain(m *testing.M) int {
	MockSystemKeyring()

	previous, had := os.LookupEnv(EnvKeyringType)
	_ = os.Setenv(EnvKeyringType, TypeMemory)
	defer func() {
		if had {
			_ = os.Setenv(EnvKeyringType, previous)
		} else {
			_ = os.Unsetenv(EnvKeyringType)
		}
	}()

	return m.Run()
}
