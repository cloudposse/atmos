package providers

import (
	"os"
	"path/filepath"
	"testing"

	"filippo.io/age"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// ageKeyFileFixture generates a fresh age identity, writes its private key to a temp file, and
// points SOPS_AGE_KEY_FILE at an ABSENT path so the only way to decrypt is via `spec.age_key_file`.
// It returns the identity, the key file path, and the (not-yet-created) encrypted file path.
func ageKeyFileFixture(t *testing.T) (*age.X25519Identity, string, string) {
	t.Helper()

	identity, err := age.GenerateX25519Identity()
	require.NoError(t, err)

	dir := t.TempDir()
	keyFile := filepath.Join(dir, "keys.txt")
	require.NoError(t, os.WriteFile(keyFile, []byte(identity.String()+"\n"), 0o600))

	// Ensure the environment cannot supply the key; only spec.age_key_file should work.
	t.Setenv("SOPS_AGE_KEY_FILE", filepath.Join(dir, "absent.txt"))
	t.Setenv("SOPS_AGE_KEY", "")

	return identity, keyFile, filepath.Join(dir, "dev.enc.yaml")
}

// ageProviderWith builds a SOPS provider with the given recipients/key-file spec.
func ageProviderWith(t *testing.T, file, recipients, ageKeyFile string) Provider {
	t.Helper()

	spec := map[string]any{"file": file}
	if recipients != "" {
		spec["age_recipients"] = recipients
	}
	if ageKeyFile != "" {
		spec["age_key_file"] = ageKeyFile
	}
	section := map[string]any{"dev-sops": map[string]any{"kind": "sops/age", "spec": spec}}
	p, err := newSopsProvider(&schema.AtmosConfiguration{}, "dev-sops", section)
	require.NoError(t, err)
	return p
}

// TestSopsProvider_AgeKeyFileDecrypts is the core proof: with SOPS_AGE_KEY_FILE pointing at an
// absent path, a provider configured with spec.age_key_file can decrypt (Get) and edit (Set) the
// file — i.e. the key is sourced from config, not the environment.
func TestSopsProvider_AgeKeyFileDecrypts(t *testing.T) {
	identity, keyFile, file := ageKeyFileFixture(t)
	datadog := Coordinate{Stack: "dev", Component: "api", Key: "DATADOG_API_KEY"}
	redis := Coordinate{Stack: "dev", Component: "api", Key: "REDIS_URL"}

	// Encrypt a fresh file using only recipients (no private key needed to encrypt).
	writer := ageProviderWith(t, file, identity.Recipient().String(), "")
	require.NoError(t, writer.Set(datadog, "dd-abc123secret"))

	// Reader has NO recipients and NO env key — only spec.age_key_file.
	reader := ageProviderWith(t, file, "", keyFile)

	got, err := reader.Get(datadog)
	require.NoError(t, err, "spec.age_key_file must allow decryption without SOPS_AGE_KEY_FILE")
	assert.Equal(t, "dd-abc123secret", got)

	ok, err := reader.Status(datadog)
	require.NoError(t, err)
	assert.True(t, ok)

	// The edit path (decrypt existing file, mutate, re-encrypt) must also use the configured key.
	require.NoError(t, reader.Set(redis, "redis://prod:6379"))
	gotRedis, err := reader.Get(redis)
	require.NoError(t, err)
	assert.Equal(t, "redis://prod:6379", gotRedis)
	gotDatadog, err := reader.Get(datadog)
	require.NoError(t, err)
	assert.Equal(t, "dd-abc123secret", gotDatadog)
}

// TestSopsProvider_AgeKeyFileWrongKeyFails proves a non-matching key file surfaces ErrSopsDecrypt.
func TestSopsProvider_AgeKeyFileWrongKeyFails(t *testing.T) {
	identity, _, file := ageKeyFileFixture(t)
	coord := Coordinate{Stack: "dev", Component: "api", Key: "DATADOG_API_KEY"}

	writer := ageProviderWith(t, file, identity.Recipient().String(), "")
	require.NoError(t, writer.Set(coord, "dd-abc123secret"))

	// A different identity's key cannot decrypt the file.
	wrong, err := age.GenerateX25519Identity()
	require.NoError(t, err)
	wrongKeyFile := filepath.Join(t.TempDir(), "wrong.txt")
	require.NoError(t, os.WriteFile(wrongKeyFile, []byte(wrong.String()+"\n"), 0o600))

	reader := ageProviderWith(t, file, "", wrongKeyFile)
	_, err = reader.Get(coord)
	require.ErrorIs(t, err, ErrSopsDecrypt)
}

// TestSopsProvider_AgeKeyFileMissingFile proves an unreadable key file surfaces ErrSopsAgeKeyFile.
func TestSopsProvider_AgeKeyFileMissingFile(t *testing.T) {
	identity, _, file := ageKeyFileFixture(t)
	coord := Coordinate{Stack: "dev", Component: "api", Key: "DATADOG_API_KEY"}

	writer := ageProviderWith(t, file, identity.Recipient().String(), "")
	require.NoError(t, writer.Set(coord, "dd-abc123secret"))

	reader := ageProviderWith(t, file, "", filepath.Join(t.TempDir(), "does-not-exist.txt"))
	_, err := reader.Get(coord)
	require.ErrorIs(t, err, ErrSopsAgeKeyFile)
}

// TestSopsProvider_FileNotFoundFriendlyError proves a missing encrypted file surfaces the
// actionable ErrSecretFileNotFound (not a cryptic decrypt error) so users know to initialize it.
func TestSopsProvider_FileNotFoundFriendlyError(t *testing.T) {
	_, keyFile, file := ageKeyFileFixture(t) // file is intentionally never created.
	reader := ageProviderWith(t, file, "", keyFile)

	_, err := reader.Get(Coordinate{Stack: "dev", Component: "api", Key: "DATADOG_API_KEY"})
	require.ErrorIs(t, err, ErrSecretFileNotFound)
}

// TestSopsProvider_AgeKeyFileEnvExpansion proves `$ENV` references in age_key_file are expanded.
func TestSopsProvider_AgeKeyFileEnvExpansion(t *testing.T) {
	identity, keyFile, file := ageKeyFileFixture(t)
	coord := Coordinate{Stack: "dev", Component: "api", Key: "DATADOG_API_KEY"}

	writer := ageProviderWith(t, file, identity.Recipient().String(), "")
	require.NoError(t, writer.Set(coord, "dd-abc123secret"))

	t.Setenv("ATMOS_TEST_AGE_KEY", keyFile)
	reader := ageProviderWith(t, file, "", "$ATMOS_TEST_AGE_KEY")
	got, err := reader.Get(coord)
	require.NoError(t, err)
	assert.Equal(t, "dd-abc123secret", got)
}
