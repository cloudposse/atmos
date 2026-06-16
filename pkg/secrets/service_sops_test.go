package secrets

import (
	"os"
	"path/filepath"
	"testing"

	"filippo.io/age"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
)

// newSopsServiceConfig builds a config + component section wiring a SOPS (age) provider declared
// in the stack/component `secrets.providers` block, plus a secrets.vars declaration backed by it.
// It generates a fresh age key on a temp dir (no `sops` binary, no committed fixtures) and returns
// the config, the section, and the resolved encrypted file path. When pinRecipients is false the
// provider omits spec.age_recipients/age_key so GenerateKey has a writable sink.
func newSopsServiceConfig(t *testing.T, pinRecipients bool) (*schema.AtmosConfiguration, map[string]any, string) {
	t.Helper()

	identity, err := age.GenerateX25519Identity()
	require.NoError(t, err)

	dir := t.TempDir()
	keyFile := filepath.Join(dir, "keys.txt")
	require.NoError(t, os.WriteFile(keyFile, []byte(identity.String()+"\n"), 0o600))
	t.Setenv("SOPS_AGE_KEY_FILE", keyFile)

	file := filepath.Join(dir, "dev.enc.yaml")
	spec := map[string]any{"file": file}
	if pinRecipients {
		spec["age_recipients"] = identity.Recipient().String()
	}

	section := map[string]any{
		"secrets": map[string]any{
			"providers": map[string]any{
				"dev-sops": map[string]any{
					"kind": "sops/age",
					"spec": spec,
				},
			},
			"vars": map[string]any{
				"DATADOG_API_KEY": map[string]any{"sops": "dev-sops", "required": true},
				"REDIS_URL":       map[string]any{"sops": "dev-sops"},
			},
		},
	}

	cfg := &schema.AtmosConfiguration{BasePath: dir}
	return cfg, section, file
}

// TestService_IsDeclared covers both branches of IsDeclared.
func TestService_IsDeclared(t *testing.T) {
	cfg, section, _ := newSopsServiceConfig(t, true)
	svc := NewService(cfg, "dev", "api", section)

	assert.True(t, svc.IsDeclared("DATADOG_API_KEY"))
	assert.False(t, svc.IsDeclared("NOT_DECLARED"))
}

// TestService_DeleteAll deletes every declared secret and returns the count processed.
func TestService_DeleteAll(t *testing.T) {
	cfg, section, _ := newSopsServiceConfig(t, true)
	svc := NewService(cfg, "dev", "api", section)

	// Seed one of the two declared secrets; Delete is idempotent on the unset one.
	require.NoError(t, svc.Set("DATADOG_API_KEY", "dd-secret"))

	n, err := svc.DeleteAll()
	require.NoError(t, err)
	assert.Equal(t, 2, n, "both declared secrets are processed")

	// Both keys are now absent from the backend.
	_, err = svc.Get("DATADOG_API_KEY", ResolveOptions{})
	require.ErrorIs(t, err, ErrSecretMissing)
}

// TestService_DeleteAll_StopsOnError proves DeleteAll aborts (returns 0) on the first error. A
// SOPS file that does not decrypt with the available key makes Delete fail.
func TestService_DeleteAll_StopsOnError(t *testing.T) {
	cfg, section, file := newSopsServiceConfig(t, true)
	svc := NewService(cfg, "dev", "api", section)

	// Create a real encrypted file, then swap the age key so Delete's decrypt step fails.
	require.NoError(t, svc.Set("DATADOG_API_KEY", "dd-secret"))
	require.FileExists(t, file)
	t.Setenv("SOPS_AGE_KEY_FILE", filepath.Join(t.TempDir(), "absent.txt"))

	n, err := svc.DeleteAll()
	require.Error(t, err)
	assert.Zero(t, n)
}

// TestService_Reset overwrites the SOPS file with a clean document and dedups by file.
func TestService_Reset(t *testing.T) {
	cfg, section, _ := newSopsServiceConfig(t, true)
	svc := NewService(cfg, "dev", "api", section)

	require.NoError(t, svc.Set("DATADOG_API_KEY", "dd-secret"))
	require.NoError(t, svc.Set("REDIS_URL", "redis://h:6379"))

	didReset, err := svc.Reset()
	require.NoError(t, err)
	assert.True(t, didReset)

	// Both keys are gone after the whole-file reset.
	_, err = svc.Get("DATADOG_API_KEY", ResolveOptions{})
	require.ErrorIs(t, err, ErrSecretMissing)
	_, err = svc.Get("REDIS_URL", ResolveOptions{})
	require.ErrorIs(t, err, ErrSecretMissing)
}

// TestService_VaultsMissingKeys returns SOPS vaults with no key material yet. With recipients
// pinned and no age key present in the default sink, the vault reports a missing key.
func TestService_VaultsMissingKeys(t *testing.T) {
	cfg, section, _ := newSopsServiceConfig(t, false)
	// Point the default age keys file at an empty location so HasKey() reports false.
	t.Setenv("SOPS_AGE_KEY_FILE", filepath.Join(t.TempDir(), "absent.txt"))
	svc := NewService(cfg, "dev", "api", section)

	missing, err := svc.VaultsMissingKeys()
	require.NoError(t, err)
	require.Len(t, missing, 1, "two declarations share one vault => de-duplicated")
	assert.Equal(t, "dev-sops", missing[0].Name)
	assert.Equal(t, string(BackendSops), missing[0].Track)
}

// TestService_VaultsMissingKeys_HasKey returns no vaults when the age key is resolvable.
func TestService_VaultsMissingKeys_HasKey(t *testing.T) {
	cfg, section, _ := newSopsServiceConfig(t, false)
	// SOPS_AGE_KEY_FILE (set by the helper) points at a real key => HasKey() is true.
	svc := NewService(cfg, "dev", "api", section)

	missing, err := svc.VaultsMissingKeys()
	require.NoError(t, err)
	assert.Empty(t, missing)
}

// TestService_GenerateKeyForVault generates an age key pair for a SOPS vault and records it.
func TestService_GenerateKeyForVault(t *testing.T) {
	cfg, section, _ := newSopsServiceConfig(t, false)
	// Direct the private sink at a writable temp path and the recipient sink at basePath.
	keysFile := filepath.Join(t.TempDir(), "generated-keys.txt")
	t.Setenv("SOPS_AGE_KEY_FILE", keysFile)
	svc := NewService(cfg, "dev", "api", section)

	res, err := svc.GenerateKeyForVault(GenerableVault{Track: string(BackendSops), Name: "dev-sops"})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "dev-sops", res.Vault)
	assert.NotEmpty(t, res.Public, "an age recipient is produced")
	require.Len(t, res.Outputs, 2)
	assert.Equal(t, "private identity", res.Outputs[0].Label)
	assert.Equal(t, "public recipient", res.Outputs[1].Label)
	require.FileExists(t, keysFile)
}

// TestService_GenerateKeyForVault_Unsupported proves a store-backed vault (whose provider does
// not implement KeyGenerator) returns ErrKeygenUnsupported.
func TestService_GenerateKeyForVault_Unsupported(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// A store-backed provider exists but does not implement KeyGenerator.
	storeCfg, storeSection := serviceTestConfig(store.NewMockStore(ctrl))
	svc := NewService(storeCfg, "prod", "api", storeSection)

	_, err := svc.GenerateKeyForVault(GenerableVault{Track: string(BackendStore), Name: "app-secrets"})
	require.ErrorIs(t, err, ErrKeygenUnsupported)
}
