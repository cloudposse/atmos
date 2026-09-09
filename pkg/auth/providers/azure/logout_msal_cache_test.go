package azure

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// realmMSALCachePath returns the on-disk path of the realm-scoped MSAL token cache
// that createMSALClient() seeds and Authenticate() reads for silent token reuse.
func realmMSALCachePath(home, realm string) string {
	return filepath.Join(home, ".azure", "atmos", realm, "msal_token_cache.json")
}

// sharedAzureCLICachePath returns the path of the Azure CLI's own MSAL cache, which is
// co-owned with a user's `az login` session and must survive an Atmos logout.
func sharedAzureCLICachePath(home string) string {
	return filepath.Join(home, ".azure", "msal_token_cache.json")
}

func writeCacheFile(t *testing.T, path string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(`{"Account":{},"RefreshToken":{}}`), 0o600))
}

// TestDeviceCodeProvider_Logout_RemovesRealmMSALCache reproduces the bug where
// `atmos auth logout` leaves the realm-scoped MSAL token cache
// (~/.azure/atmos/{realm}/msal_token_cache.json) on disk. Because
// Authenticate() tries a silent MSAL acquisition from that cache first, the next
// `atmos auth login` reuses the previous session's account and refresh token —
// a stale token that predates any role/PIM change — producing persistent 403s.
//
// Logout must remove the realm MSAL cache (forcing a fresh interactive login) while
// leaving the shared Azure CLI cache (~/.azure/msal_token_cache.json) untouched, so a
// user's separate `az login` session is not destroyed.
func TestDeviceCodeProvider_Logout_RemovesRealmMSALCache(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	const realm = "test-realm"

	provider, err := NewDeviceCodeProvider("test-provider", &schema.Provider{
		Kind: "azure/device-code",
		Spec: map[string]interface{}{"tenant_id": "tenant-123"},
	})
	require.NoError(t, err)
	provider.SetRealm(realm)

	realmCache := realmMSALCachePath(tmpHome, realm)
	sharedCache := sharedAzureCLICachePath(tmpHome)

	// Simulate the post-login on-disk state: MSAL persisted the realm cache, and the
	// login wrote back into the shared Azure CLI cache.
	writeCacheFile(t, realmCache)
	writeCacheFile(t, sharedCache)

	require.NoError(t, provider.Logout(context.Background()))

	// The realm MSAL cache must be gone so the next login is fresh.
	_, err = os.Stat(realmCache)
	require.True(t, os.IsNotExist(err), "realm MSAL cache should be removed by logout, got err=%v", err)

	// The shared Azure CLI cache must survive — it is co-owned with `az login`.
	_, err = os.Stat(sharedCache)
	require.NoError(t, err, "shared Azure CLI cache must NOT be removed by Atmos logout")
}

// TestInteractiveProvider_Logout_RemovesRealmMSALCache verifies the interactive
// provider (which embeds deviceCodeProvider) inherits the realm-cache cleanup, since
// it is the provider that most commonly hits the stale-token bug.
func TestInteractiveProvider_Logout_RemovesRealmMSALCache(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	const realm = "cw"

	provider, err := NewInteractiveProvider("azure-interactive", &schema.Provider{
		Kind: "azure/interactive",
		Spec: map[string]interface{}{"tenant_id": "tenant-123"},
	})
	require.NoError(t, err)
	provider.SetRealm(realm)

	realmCache := realmMSALCachePath(tmpHome, realm)
	writeCacheFile(t, realmCache)

	require.NoError(t, provider.Logout(context.Background()))

	_, err = os.Stat(realmCache)
	require.True(t, os.IsNotExist(err), "interactive provider logout should remove the realm MSAL cache, got err=%v", err)
}

// TestDeviceCodeProvider_Logout_DeleteCachedTokenError verifies that when the device-code
// token cache removal fails (here, an unresolvable XDG cache dir), Logout surfaces that error
// and short-circuits before touching the MSAL cache.
func TestDeviceCodeProvider_Logout_DeleteCachedTokenError(t *testing.T) {
	// A zero-value mock returns an error from GetXDGCacheDir (xdgCacheDir unset), so
	// deleteCachedToken -> getTokenCachePath fails.
	provider := &deviceCodeProvider{
		name:         "test-provider",
		realm:        "test-realm",
		cacheStorage: &mockCacheStorage{},
	}

	err := provider.Logout(context.Background())
	require.Error(t, err, "Logout should surface a device-code token cache removal failure")
}

// TestDeviceCodeProvider_Logout_MSALCacheMissingIsNotAnError verifies the steady-state
// logout (no cache present) is a clean no-op rather than a failure.
func TestDeviceCodeProvider_Logout_MSALCacheMissingIsNotAnError(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	provider, err := NewDeviceCodeProvider("test-provider", &schema.Provider{
		Kind: "azure/device-code",
		Spec: map[string]interface{}{"tenant_id": "tenant-123"},
	})
	require.NoError(t, err)
	provider.SetRealm("test-realm")

	require.NoError(t, provider.Logout(context.Background()))
}
