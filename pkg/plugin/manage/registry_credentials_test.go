package manage

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/keyring"
	"github.com/cloudposse/atmos/pkg/schema"
)

func credentialsFixture(t *testing.T, keyringType string, input string) (*RegistryCredentials, keyring.Keyring) {
	t.Helper()
	store, err := keyring.New(keyring.Config{Type: keyring.TypeMemory})
	require.NoError(t, err)
	return &RegistryCredentials{
		Auth:       &schema.AuthConfig{Keyring: schema.KeyringConfig{Type: keyringType}},
		Input:      strings.NewReader(input),
		StdinIsTTY: func() bool { return false },
		Open:       func(*schema.AuthConfig) (keyring.Keyring, error) { return store, nil },
	}, store
}

func TestStoreRefusesBackendsThatPersistNothing(t *testing.T) {
	for _, backend := range []string{"memory", "noop"} {
		t.Run(backend, func(t *testing.T) {
			credentials, store := credentialsFixture(t, backend, "secret\n")
			err := credentials.Store("registry.example.com", "robot")
			require.ErrorIs(t, err, errUtils.ErrPluginCredentialStore)
			hints := strings.Join(errUtils.AllHints(err), " ")
			assert.Contains(t, hints, "auth.keyring.type")
			assert.Contains(t, hints, "password_env")
			keys, listErr := store.List()
			require.NoError(t, listErr)
			assert.Empty(t, keys, "nothing may be written when persistence is impossible")
		})
	}
}

func TestStoreRefusesPasswordFromInteractiveTerminal(t *testing.T) {
	credentials, store := credentialsFixture(t, "file", "ignored")
	credentials.StdinIsTTY = func() bool { return true }
	err := credentials.Store("registry.example.com", "robot")
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	assert.Contains(t, strings.Join(errUtils.AllHints(err), " "), "Pipe the secret")
	keys, _ := store.List()
	assert.Empty(t, keys)
}

func TestStorePersistsCredentialsAndRemovePurgesThem(t *testing.T) {
	credentials, store := credentialsFixture(t, "file", "s3cret\r\n")
	require.NoError(t, credentials.Store("registry.example.com", "robot"))
	keys, err := store.List()
	require.NoError(t, err)
	require.Len(t, keys, 1)
	raw, err := store.Get(keys[0])
	require.NoError(t, err)
	assert.Contains(t, raw, "s3cret")
	assert.NotContains(t, raw, "\r")
	require.NoError(t, credentials.Remove("registry.example.com"))
	keys, _ = store.List()
	assert.Empty(t, keys)
	require.NoError(t, credentials.Remove("registry.example.com"), "removal is idempotent")
}

func TestStoreRejectsEmptyAndOversizedPasswords(t *testing.T) {
	credentials, _ := credentialsFixture(t, "file", "\n")
	require.ErrorIs(t, credentials.Store("registry.example.com", "robot"), errUtils.ErrPluginInvalidConfig)
	credentials, _ = credentialsFixture(t, "file", strings.Repeat("x", maxRegistryPassword+1))
	require.ErrorIs(t, credentials.Store("registry.example.com", "robot"), errUtils.ErrPluginPayloadTooLarge)
}

type failingKeyring struct{ keyring.Keyring }

func (failingKeyring) Set(string, string) error {
	return errors.New("exec: security: exit status 154")
}

func TestKeyringFailureIsNotLabelledAsRegistryFailure(t *testing.T) {
	credentials, _ := credentialsFixture(t, "system", "secret\n")
	memory, err := keyring.New(keyring.Config{Type: keyring.TypeMemory})
	require.NoError(t, err)
	credentials.Open = func(*schema.AuthConfig) (keyring.Keyring, error) { return failingKeyring{memory}, nil }
	err = credentials.Store("registry.example.com", "robot")
	require.ErrorIs(t, err, errUtils.ErrPluginCredentialStore)
	require.NotErrorIs(t, err, errUtils.ErrOCIRegistry)
	assert.NotContains(t, err.Error(), "OCI registry operation failed")
	assert.Contains(t, err.Error(), "system keyring")
	assert.Contains(t, err.Error(), "exit status 154", "the backend's own message stays visible")
	assert.Contains(t, strings.Join(errUtils.AllHints(err), " "), "auth.keyring.type")

	credentials.Input = strings.NewReader("secret\n")
	credentials.Open = func(*schema.AuthConfig) (keyring.Keyring, error) { return nil, keyring.ErrUnavailable }
	err = credentials.Store("registry.example.com", "robot")
	require.ErrorIs(t, err, errUtils.ErrPluginCredentialStore)
	require.ErrorIs(t, err, keyring.ErrUnavailable)
}
