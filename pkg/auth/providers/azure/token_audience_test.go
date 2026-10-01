package azure

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	azureCloud "github.com/cloudposse/atmos/pkg/auth/cloud/azure"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Regression tests for the field failure where azapi-based modules (all modern
// AVM modules) died mid-apply with "AzureCLICredential: ERROR: Can't find token
// from MSAL cache": the seeded Azure CLI cache held the management token only
// under the modern ARM scope (management.azure.com), while azidentity/az request
// the LEGACY ARM audience (management.core.windows.net) by default — and no
// refresh token was seeded, so az could neither serve nor mint it.

func seededProvider(t *testing.T) (*deviceCodeProvider, string) {
	t.Helper()
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	p, err := NewDeviceCodeProvider("test-provider", &schema.Provider{
		Kind: "azure/device-code",
		Spec: map[string]interface{}{"tenant_id": "tenant-123"},
	})
	require.NoError(t, err)
	p.SetRealm("test-realm")
	return p, tmpHome
}

func seededToken(t *testing.T) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"typ":"JWT","alg":"RS256"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"oid":"user-oid","upn":"user@example.com"}`))
	return header + "." + payload + ".sig"
}

func azCache(t *testing.T, tmpHome string) map[string]map[string]map[string]interface{} {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(tmpHome, ".azure", "msal_token_cache.json"))
	require.NoError(t, err)
	var cache map[string]map[string]map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &cache))
	return cache
}

func TestAzureCLICache_ManagementTokenCoversLegacyAudience(t *testing.T) {
	p, tmpHome := seededProvider(t)

	require.NoError(t, p.updateAzureCLICache(&tokenCacheUpdate{
		AccessToken:   seededToken(t),
		ExpiresAt:     time.Now().UTC().Add(1 * time.Hour),
		HomeAccountID: "home-oid.home-tenant",
	}))

	cache := azCache(t, tmpHome)
	var target string
	for _, entry := range cache["AccessToken"] {
		if tgt, _ := entry["target"].(string); strings.Contains(tgt, "management.azure.com") {
			target = tgt
		}
	}
	require.NotEmpty(t, target, "management token entry expected")
	assert.Contains(t, target, "https://management.core.windows.net/.default",
		"target must include the legacy ARM audience so az's default lookup (and azidentity/azapi) hit the cache")
	assert.Contains(t, target, "https://management.core.windows.net//.default",
		"target must include the double-slash legacy form az derives from the trailing-slash resource")
}

func TestAzureCLICache_RefreshTokenCopiedFromAtmosCache(t *testing.T) {
	p, tmpHome := seededProvider(t)

	// Seed the ATMOS realm MSAL cache with a refresh token for the account,
	// as MSAL persists after a device-code/interactive login.
	atmosCacheDir := filepath.Join(tmpHome, ".azure", "atmos", "test-realm")
	require.NoError(t, os.MkdirAll(atmosCacheDir, 0o700))
	atmosCache := map[string]interface{}{
		"RefreshToken": map[string]interface{}{
			"home-oid.home-tenant-login.microsoftonline.com-refreshtoken-04b07795-8ddb-461a-bbee-02f9e1bf7b46--": map[string]interface{}{
				"home_account_id": "home-oid.home-tenant",
				"environment":     "login.microsoftonline.com",
				"client_id":       "04b07795-8ddb-461a-bbee-02f9e1bf7b46",
				"credential_type": "RefreshToken",
				"secret":          "fake-refresh-token",
				"family_id":       "1",
			},
		},
	}
	data, err := json.Marshal(atmosCache)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(atmosCacheDir, "msal_token_cache.json"), data, 0o600))

	require.NoError(t, p.updateAzureCLICache(&tokenCacheUpdate{
		AccessToken:   seededToken(t),
		ExpiresAt:     time.Now().UTC().Add(1 * time.Hour),
		HomeAccountID: "home-oid.home-tenant",
	}))

	cache := azCache(t, tmpHome)
	require.NotEmpty(t, cache["RefreshToken"],
		"refresh token must be copied into the az cache so az can self-mint any audience and survive access-token expiry")
	for _, entry := range cache["RefreshToken"] {
		assert.Equal(t, "home-oid.home-tenant", entry["home_account_id"])
		assert.Equal(t, "fake-refresh-token", entry["secret"])
	}
	_ = azureCloud.GetCloudEnvironment("") // anchor import
}
