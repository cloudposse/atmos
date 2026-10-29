package credtest_test

import (
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	zkeyring "github.com/zalando/go-keyring"

	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/credentials/credtest"
	"github.com/cloudposse/atmos/pkg/auth/types"
)

// The package default comes from TestMain, exactly as other test packages consume it.
func TestMain(m *testing.M) {
	if os.Getenv("_ATMOS_CREDTEST_UNKNOWN_BACKEND") == "1" {
		os.Exit(m.Run())
	}
	os.Exit(credtest.IsolateKeyringMain(m))
}

const (
	guardAlias = "credtest-guard-identity"
	guardRealm = "credtest-guard-realm"
)

func guardCreds() types.ICredentials {
	return &types.AWSCredentials{AccessKeyID: "AKIAGUARD", SecretAccessKey: "secret", Region: "us-east-1"}
}

// TestIsolateKeyringMain_DefaultsToMemory proves that, with no per-test setup, the credential
// store never selects the system keychain and stores nothing outside the store instance.
func TestIsolateKeyringMain_DefaultsToMemory(t *testing.T) {
	first := credentials.NewCredentialStoreWithConfig(nil)
	assert.Equal(t, types.CredentialStoreTypeMemory, first.Type())
	assert.NotEqual(t, types.CredentialStoreTypeSystemKeyring, first.Type())

	require.NoError(t, first.Store(guardAlias, guardCreds(), guardRealm))
	got, err := first.Retrieve(guardAlias, guardRealm)
	require.NoError(t, err)
	assert.NotNil(t, got)

	// A second instance has its own memory keyring, so nothing is shared.
	second := credentials.NewCredentialStoreWithConfig(nil)
	_, err = second.Retrieve(guardAlias, guardRealm)
	assert.Error(t, err)
}

// TestIsolateKeyring_SelectsFileBackend proves IsolateKeyring picks the file backend, not the
// system keychain, and that credentials persist across store instances within the test.
func TestIsolateKeyring_SelectsFileBackend(t *testing.T) {
	credtest.IsolateKeyring(t)

	first := credentials.NewCredentialStoreWithConfig(nil)
	require.Equal(t, types.CredentialStoreTypeFile, first.Type())
	assert.NotEqual(t, types.CredentialStoreTypeSystemKeyring, first.Type())

	require.NoError(t, first.Store(guardAlias, guardCreds(), guardRealm))

	second := credentials.NewCredentialStoreWithConfig(nil)
	got, err := second.Retrieve(guardAlias, guardRealm)
	require.NoError(t, err, "the file backend must persist across store instances in one test")
	assert.NotNil(t, got)
}

// TestIsolateKeyring_DoesNotLeakAcrossTests runs two ordered subtests that each isolate the
// keyring: the credential written by the first must not be visible to the second.
func TestIsolateKeyring_DoesNotLeakAcrossTests(t *testing.T) {
	t.Run("writer", func(t *testing.T) {
		credtest.IsolateKeyring(t)
		store := credentials.NewCredentialStoreWithConfig(nil)
		require.Equal(t, types.CredentialStoreTypeFile, store.Type())
		require.NoError(t, store.Store(guardAlias, guardCreds(), guardRealm))
		_, err := store.Retrieve(guardAlias, guardRealm)
		require.NoError(t, err)
	})

	t.Run("reader", func(t *testing.T) {
		credtest.IsolateKeyring(t)
		store := credentials.NewCredentialStoreWithConfig(nil)
		require.Equal(t, types.CredentialStoreTypeFile, store.Type())
		_, err := store.Retrieve(guardAlias, guardRealm)
		assert.Error(t, err, "credentials from another test must not leak into this one")
	})
}

// TestIsolateKeyring_RestoresEnvironment proves the helper leaves the process environment as it
// found it once the test ends.
func TestIsolateKeyring_RestoresEnvironment(t *testing.T) {
	before := os.Getenv(credtest.EnvKeyringType)

	t.Run("inner", func(t *testing.T) {
		credtest.IsolateKeyring(t)
		assert.Equal(t, credtest.TypeFile, os.Getenv(credtest.EnvKeyringType))
		assert.NotEmpty(t, os.Getenv(credtest.EnvKeyringFilePath))
	})

	assert.Equal(t, before, os.Getenv(credtest.EnvKeyringType))
}

// TestEnsureIsolated covers both branches: it isolates when the environment would reach the system
// keychain and leaves an explicit backend choice untouched.
func TestEnsureIsolated(t *testing.T) {
	tests := []struct {
		name     string
		initial  string
		wantType string
	}{
		{name: "empty resolves to system so it is isolated", initial: "", wantType: credtest.TypeFile},
		{name: "explicit system is isolated", initial: "system", wantType: credtest.TypeFile},
		{name: "memory is kept", initial: "memory", wantType: "memory"},
		{name: "file is kept", initial: "file", wantType: "file"},
		{name: "invalid choice is kept", initial: "unknown", wantType: "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(credtest.EnvKeyringType, tt.initial)
			credtest.EnsureIsolated(t)
			assert.Equal(t, tt.wantType, os.Getenv(credtest.EnvKeyringType))
		})
	}
}

// Run without IsolateKeyringMain so prior tests cannot hide a missing keychain guard.
func TestEnsureIsolatedUnknownBackendWithoutTestMain(t *testing.T) {
	if os.Getenv("_ATMOS_CREDTEST_UNKNOWN_BACKEND") != "1" {
		executable, err := os.Executable()
		require.NoError(t, err)
		command := exec.CommandContext(t.Context(), executable, "-test.run=^TestEnsureIsolatedUnknownBackendWithoutTestMain$")
		command.Env = append(os.Environ(), "_ATMOS_CREDTEST_UNKNOWN_BACKEND=1")
		output, err := command.CombinedOutput()
		require.NoError(t, err, "%s", output)
		return
	}
	// Install a failing mock first: even the unfixed regression test cannot access the host keychain.
	zkeyring.MockInitWithError(assert.AnError)
	t.Setenv(credtest.EnvKeyringType, "unknown")
	credtest.EnsureIsolated(t)
	require.Equal(t, "unknown", os.Getenv(credtest.EnvKeyringType))
	require.NoError(t, zkeyring.Set("credtest", "guard", "isolated"), "EnsureIsolated must install its working system mock")
	store := credentials.NewCredentialStoreWithConfig(nil)
	require.Equal(t, types.CredentialStoreTypeSystemKeyring, store.Type())
	require.NoError(t, store.Store(guardAlias, guardCreds(), guardRealm))
	got, err := store.Retrieve(guardAlias, guardRealm)
	require.NoError(t, err)
	require.Equal(t, guardCreds(), got)
}
