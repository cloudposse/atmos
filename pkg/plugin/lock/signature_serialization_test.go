package lock

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestSignedLockMetadataRoundTripsAndRejectsIncompleteVerdicts(t *testing.T) {
	for _, tc := range []struct {
		name         string
		verification Verification
		valid        bool
	}{
		{"verified signer", Verification{Signature: "verified", Identity: "builder@example.com", Issuer: "https://issuer.example.com"}, true},
		{"unsigned", Verification{Signature: "unverified"}, true},
		{"missing identity", Verification{Signature: "verified", Issuer: "https://issuer.example.com"}, false},
		{"missing issuer", Verification{Signature: "verified", Identity: "builder@example.com"}, false},
		{"unsigned with identity", Verification{Signature: "unverified", Identity: "builder@example.com"}, false},
		{"unsigned with issuer", Verification{Signature: "unverified", Issuer: "https://issuer.example.com"}, false},
		{"unknown verdict", Verification{Signature: "trusted", Identity: "builder@example.com", Issuer: "https://issuer.example.com"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			directory := t.TempDir()
			file := New()
			entry := validEntry()
			entry.Verification = tc.verification
			file.Spec.Packages["corporate"] = entry
			require.NoError(t, Save(directory, file))
			loaded, err := Load(directory)
			if !tc.valid {
				require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
				require.Nil(t, loaded)
				return
			}
			require.NoError(t, err)
			require.Equal(t, entry, loaded.Spec.Packages["corporate"])
			raw, err := os.ReadFile(Path(directory))
			require.NoError(t, err)
			require.Contains(t, string(raw), "signature: "+tc.verification.Signature)
			assertVerificationSerialization(t, tc.verification)
		})
	}
}

func assertVerificationSerialization(t *testing.T, verification Verification) {
	t.Helper()
	jsonRaw, err := json.Marshal(verification)
	require.NoError(t, err)
	yamlRaw, err := yaml.Marshal(verification)
	require.NoError(t, err)
	for _, raw := range [][]byte{jsonRaw, yamlRaw} {
		var fields map[string]string
		require.NoError(t, yaml.Unmarshal(raw, &fields))
		expected := map[string]string{"signature": verification.Signature}
		if verification.Signature == "verified" {
			expected["identity"] = verification.Identity
			expected["issuer"] = verification.Issuer
		}
		require.Equal(t, expected, fields)
	}
}
