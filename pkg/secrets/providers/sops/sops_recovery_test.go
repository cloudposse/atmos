package sops

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/secrets/providers"
)

type recipientRestorer interface {
	RestoreRecipients(string) (*providers.KeygenResult, error)
}

func restoreRecipients(t *testing.T, p *sopsProvider, base string) (*providers.KeygenResult, error) {
	t.Helper()
	restorer, ok := any(p).(recipientRestorer)
	require.True(t, ok, "SOPS must recover recipients without generating another private key")
	return restorer.RestoreRecipients(base)
}

func TestRestoreRecipientsMissingRule(t *testing.T) {
	for _, shared := range []bool{false, true} {
		name := "missing recipients file"
		if shared {
			name = "second vault sharing key"
		}
		t.Run(name, func(t *testing.T) {
			base := t.TempDir()
			t.Chdir(base)
			keys := filepath.Join(base, "keys.txt")
			t.Setenv("SOPS_AGE_KEY_FILE", keys)
			t.Setenv("SOPS_AGE_KEY", "")
			id, err := generateAgeIdentity()
			require.NoError(t, err)
			original := []byte("# preserve this comment\n" + id.Private + "\n")
			require.NoError(t, os.WriteFile(keys, original, 0o600))
			if shared {
				first := &sopsProvider{file: "first.enc.yaml"}
				require.NoError(t, first.writeRecipient(base, filepath.Join(base, ".sops.yaml"), id.Recipient))
			}
			p := &sopsProvider{name: "second", kind: "sops/age", file: "second.enc.yaml"}
			result, err := restoreRecipients(t, p, base)
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, id.Recipient, result.Public)
			after, err := os.ReadFile(keys)
			require.NoError(t, err)
			assert.Equal(t, original, after, "recovery must preserve private material byte-for-byte")
			coord := providers.Coordinate{Key: "TOKEN"}
			require.NoError(t, p.Set(coord, "recovered-value"))
			value, err := p.Get(coord)
			require.NoError(t, err)
			assert.Equal(t, "recovered-value", value)
			before, err := os.ReadFile(filepath.Join(base, ".sops.yaml"))
			require.NoError(t, err)
			result, err = restoreRecipients(t, p, base)
			require.NoError(t, err)
			assert.Nil(t, result)
			after, err = os.ReadFile(filepath.Join(base, ".sops.yaml"))
			require.NoError(t, err)
			assert.Equal(t, before, after, "existing recipients must not be rewritten")
		})
	}
}

func TestRestoreRecipientsPreservesConfiguredRules(t *testing.T) {
	for _, rule := range []string{
		"  - path_regex: second\\.enc\\.yaml$\n    age: age1existing\n",
		"  - path_regex: .*\\.enc\\.yaml$\n    age: age1existing\n",
		"  - key_groups:\n      - age: [age1existing]\n",
	} {
		t.Run(rule, func(t *testing.T) {
			base := t.TempDir()
			sink := filepath.Join(base, ".sops.yaml")
			original := []byte("# preserve recipients and comments\ncreation_rules:\n" + rule)
			require.NoError(t, os.WriteFile(sink, original, 0o644))
			p := &sopsProvider{kind: "sops/age", file: "second.enc.yaml", ageKey: "unreadable key is irrelevant"}
			result, err := restoreRecipients(t, p, base)
			require.NoError(t, err)
			assert.Nil(t, result)
			after, err := os.ReadFile(sink)
			require.NoError(t, err)
			assert.Equal(t, original, after)
		})
	}
}

func TestRestoreRecipientsRejectsAmbiguousAndInvalidKeys(t *testing.T) {
	first, err := generateAgeIdentity()
	require.NoError(t, err)
	second, err := generateAgeIdentity()
	require.NoError(t, err)
	for _, material := range []string{"", "not-a-key", first.Private + "\n" + second.Private} {
		t.Run(material[:min(len(material), 10)], func(t *testing.T) {
			base := t.TempDir()
			keys := filepath.Join(base, "keys.txt")
			require.NoError(t, os.WriteFile(keys, []byte(material), 0o600))
			p := &sopsProvider{kind: "sops/age", file: "second.enc.yaml", ageKeyFile: keys}
			result, err := restoreRecipients(t, p, base)
			require.ErrorIs(t, err, ErrSopsAgeKey)
			assert.Nil(t, result)
			assert.NotContains(t, err.Error(), first.Private)
			assert.NoFileExists(t, filepath.Join(base, ".sops.yaml"))
			after, err := os.ReadFile(keys)
			require.NoError(t, err)
			assert.Equal(t, material, string(after))
		})
	}
}

func TestRestoreRecipientsKeySources(t *testing.T) {
	id, err := generateAgeIdentity()
	require.NoError(t, err)
	t.Run("inline duplicate identity", func(t *testing.T) {
		p := &sopsProvider{kind: "sops/age", file: "second.enc.yaml", ageKey: id.Private + "\n" + id.Private}
		result, err := restoreRecipients(t, p, t.TempDir())
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, id.Recipient, result.Public)
	})
	t.Run("store identity", func(t *testing.T) {
		cfg := newKeychainBackedConfig(t)
		p := &sopsProvider{name: "vault", kind: "sops/age", file: "second.enc.yaml", ageKeyStore: "keychain", stores: cfg.Stores}
		_, err := p.writeKeyToStore(id.Private)
		require.NoError(t, err)
		result, err := restoreRecipients(t, p, t.TempDir())
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, id.Recipient, result.Public)
		material, err := p.existingKeyMaterial()
		require.NoError(t, err)
		assert.Equal(t, id.Private, material)
	})
	for _, p := range []*sopsProvider{
		{kind: "sops/aws-kms"},
		{kind: "sops/age", ageRecipients: id.Recipient},
	} {
		result, err := restoreRecipients(t, p, t.TempDir())
		require.NoError(t, err)
		assert.Nil(t, result)
	}
}

func TestRestoreRecipientsInvalidConfiguration(t *testing.T) {
	for _, content := range []string{"creation_rules: [unterminated", "creation_rules: wrong-type", "creation_rules:\n  - path_regex: '['\n"} {
		t.Run(content, func(t *testing.T) {
			base := t.TempDir()
			sink := filepath.Join(base, ".sops.yaml")
			require.NoError(t, os.WriteFile(sink, []byte(content), 0o644))
			p := &sopsProvider{kind: "sops/age", file: "secret.enc.yaml"}
			result, err := restoreRecipients(t, p, base)
			require.ErrorIs(t, err, ErrSopsRecipients)
			assert.Nil(t, result)
			after, err := os.ReadFile(sink)
			require.NoError(t, err)
			assert.Equal(t, content, string(after))
		})
	}
	t.Run("recipient file is a directory", func(t *testing.T) {
		base := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(base, ".sops.yaml"), 0o700))
		p := &sopsProvider{kind: "sops/age", file: "secret.enc.yaml"}
		_, err := restoreRecipients(t, p, base)
		require.ErrorIs(t, err, ErrSopsRecipients)
	})
	t.Run("missing explicit private key", func(t *testing.T) {
		base := t.TempDir()
		p := &sopsProvider{kind: "sops/age", file: "secret.enc.yaml", ageKeyFile: filepath.Join(base, "missing.txt")}
		_, err := restoreRecipients(t, p, base)
		require.ErrorIs(t, err, ErrSopsAgeKeyFile)
		assert.NoFileExists(t, filepath.Join(base, ".sops.yaml"))
	})
	t.Run("missing private key store", func(t *testing.T) {
		p := &sopsProvider{kind: "sops/age", file: "secret.enc.yaml", ageKeyStore: "missing"}
		_, err := restoreRecipients(t, p, t.TempDir())
		require.ErrorIs(t, err, ErrSopsAgeKey)
	})
	t.Run("store has no identity", func(t *testing.T) {
		cfg := newKeychainBackedConfig(t)
		p := &sopsProvider{name: "vault", kind: "sops/age", file: "secret.enc.yaml", ageKeyStore: "keychain", stores: cfg.Stores}
		_, err := restoreRecipients(t, p, t.TempDir())
		require.ErrorIs(t, err, ErrSopsAgeKey)
		_, err = p.writeKeyToStore("")
		require.NoError(t, err)
		_, err = restoreRecipients(t, p, t.TempDir())
		require.ErrorIs(t, err, ErrSopsAgeKey)
	})
}
