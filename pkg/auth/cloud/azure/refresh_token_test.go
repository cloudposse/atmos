package azure

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeAtmosRealmCache writes an MSAL cache file at the Atmos realm path under home.
func writeAtmosRealmCache(t *testing.T, home, realm string, content []byte) {
	t.Helper()
	dir := filepath.Join(home, ".azure", "atmos", realm)
	require.NoError(t, os.MkdirAll(dir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "msal_token_cache.json"), content, 0o600))
}

func rtEntry(homeAccountID string) map[string]interface{} {
	return map[string]interface{}{
		"home_account_id": homeAccountID,
		"credential_type": "RefreshToken",
		"secret":          "fake-refresh-token",
	}
}

func TestCopyAtmosRefreshTokensInto(t *testing.T) {
	const accountID = "oid-123.tenant-456"

	marshal := func(t *testing.T, v interface{}) []byte {
		t.Helper()
		data, err := json.Marshal(v)
		require.NoError(t, err)
		return data
	}

	tests := []struct {
		name          string
		realm         string
		homeAccountID string
		atmosCache    []byte // nil = no cache file written
		azCache       map[string]interface{}
		expectCopied  int
		expectDest    map[string]interface{} // nil = length check only
	}{
		{
			name:          "matching entry is copied",
			realm:         "test-realm",
			homeAccountID: accountID,
			atmosCache: marshal(t, map[string]interface{}{
				"RefreshToken": map[string]interface{}{"rt-key": rtEntry(accountID)},
			}),
			azCache:      map[string]interface{}{},
			expectCopied: 1,
			expectDest:   map[string]interface{}{"rt-key": rtEntry(accountID)},
		},
		{
			name:          "matching entry in a non-default realm is copied",
			realm:         "custom-realm",
			homeAccountID: accountID,
			atmosCache: marshal(t, map[string]interface{}{
				"RefreshToken": map[string]interface{}{"rt-key": rtEntry(accountID)},
			}),
			azCache:      map[string]interface{}{},
			expectCopied: 1,
			expectDest:   map[string]interface{}{"rt-key": rtEntry(accountID)},
		},
		{
			name:          "mismatched home account ID is skipped",
			realm:         "test-realm",
			homeAccountID: accountID,
			atmosCache: marshal(t, map[string]interface{}{
				"RefreshToken": map[string]interface{}{"rt-key": rtEntry("other-account")},
			}),
			azCache:      map[string]interface{}{},
			expectCopied: 0,
		},
		{
			name:          "empty realm is a no-op",
			realm:         "",
			homeAccountID: accountID,
			azCache:       map[string]interface{}{},
			expectCopied:  0,
		},
		{
			name:          "empty home account ID is a no-op",
			realm:         "test-realm",
			homeAccountID: "",
			atmosCache: marshal(t, map[string]interface{}{
				"RefreshToken": map[string]interface{}{"rt-key": rtEntry(accountID)},
			}),
			azCache:      map[string]interface{}{},
			expectCopied: 0,
		},
		{
			name:          "missing Atmos cache file is a no-op",
			realm:         "test-realm",
			homeAccountID: accountID,
			azCache:       map[string]interface{}{},
			expectCopied:  0,
		},
		{
			name:          "invalid JSON in Atmos cache is a no-op",
			realm:         "test-realm",
			homeAccountID: accountID,
			atmosCache:    []byte("{not json"),
			azCache:       map[string]interface{}{},
			expectCopied:  0,
		},
		{
			name:          "missing RefreshToken section is a no-op",
			realm:         "test-realm",
			homeAccountID: accountID,
			atmosCache:    marshal(t, map[string]interface{}{"AccessToken": map[string]interface{}{}}),
			azCache:       map[string]interface{}{},
			expectCopied:  0,
		},
		{
			name:          "non-object entries are skipped",
			realm:         "test-realm",
			homeAccountID: accountID,
			atmosCache: marshal(t, map[string]interface{}{
				"RefreshToken": map[string]interface{}{
					"bad-key":  "not an object",
					"good-key": rtEntry(accountID),
				},
			}),
			azCache:      map[string]interface{}{},
			expectCopied: 1,
		},
		{
			name:          "existing az RefreshToken section is preserved and appended to",
			realm:         "test-realm",
			homeAccountID: accountID,
			atmosCache: marshal(t, map[string]interface{}{
				"RefreshToken": map[string]interface{}{"rt-key": rtEntry(accountID)},
			}),
			azCache: map[string]interface{}{
				"RefreshToken": map[string]interface{}{"pre-existing": rtEntry("other-account")},
			},
			expectCopied: 1,
			expectDest: map[string]interface{}{
				"pre-existing": rtEntry("other-account"),
				"rt-key":       rtEntry(accountID),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			if tt.atmosCache != nil {
				writeAtmosRealmCache(t, home, tt.realm, tt.atmosCache)
			}

			preExisting := 0
			if dest, ok := tt.azCache["RefreshToken"].(map[string]interface{}); ok {
				preExisting = len(dest)
			}

			CopyAtmosRefreshTokensInto(tt.azCache, home, tt.realm, tt.homeAccountID)

			dest, _ := tt.azCache["RefreshToken"].(map[string]interface{})
			assert.Len(t, dest, preExisting+tt.expectCopied)
			if tt.expectDest != nil {
				assert.Equal(t, tt.expectDest, dest)
			}
			for _, raw := range dest {
				entry, ok := raw.(map[string]interface{})
				require.True(t, ok, "copied entries must be objects")
				assert.NotEmpty(t, entry["home_account_id"])
			}
		})
	}
}
