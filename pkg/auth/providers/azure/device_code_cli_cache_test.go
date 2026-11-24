package azure

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	azureCloud "github.com/cloudposse/atmos/pkg/auth/cloud/azure"
	"github.com/cloudposse/atmos/pkg/filelock"
	"github.com/cloudposse/atmos/pkg/schema"
)

// The az-owned entry seeded into the shared cache; it must survive every Atmos update.
const azOwnedAccountKey = "az-owned-account"

// sandboxCLICacheHome points the home directory at a temp dir and returns the
// path of the shared Azure CLI MSAL cache inside it.
func sandboxCLICacheHome(t *testing.T) string {
	t.Helper()
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)
	t.Setenv("AZURE_CONFIG_DIR", tmpHome)
	azureDir := filepath.Join(tmpHome, ".azure")
	require.NoError(t, os.MkdirAll(azureDir, 0o700))
	return filepath.Join(azureDir, "msal_token_cache.json")
}

func newCLICacheTestProvider() *deviceCodeProvider {
	return &deviceCodeProvider{
		name:     "test-provider",
		tenantID: "tenant-123",
		cloudEnv: azureCloud.GetCloudEnvironment(""),
		config: &schema.Provider{
			Kind: "azure/device-code",
			Spec: map[string]interface{}{"tenant_id": "tenant-123"},
		},
	}
}

func newCLICacheTestUpdate() *tokenCacheUpdate {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"typ":"JWT","alg":"RS256"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"oid":"user-oid-123","upn":"user@example.com"}`))
	return &tokenCacheUpdate{
		AccessToken: header + "." + payload + ".testsignature",
		ExpiresAt:   time.Now().UTC().Add(time.Hour),
	}
}

func azOwnedCacheJSON(t *testing.T) []byte {
	t.Helper()
	data, err := json.Marshal(map[string]interface{}{
		"Account": map[string]interface{}{
			azOwnedAccountKey: map[string]interface{}{"username": "me@example.com"},
		},
		"RefreshToken": map[string]interface{}{
			"az-owned-refresh-token": map[string]interface{}{"secret": "keep-me"},
		},
	})
	require.NoError(t, err)
	return data
}

func readCLICache(t *testing.T, path string) map[string]map[string]interface{} {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var cache map[string]map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &cache))
	return cache
}

// TestUpdateAzureCLICache_PreservesBOMPrefixedCache verifies that a cache written
// with a UTF-8 BOM (as some Azure CLI builds do) is parsed and extended, not
// replaced by an empty cache that drops the user's own `az login` session.
func TestUpdateAzureCLICache_PreservesBOMPrefixedCache(t *testing.T) {
	cachePath := sandboxCLICacheHome(t)
	bom := []byte{0xEF, 0xBB, 0xBF}
	require.NoError(t, os.WriteFile(cachePath, append(bom, azOwnedCacheJSON(t)...), 0o600))

	require.NoError(t, newCLICacheTestProvider().updateAzureCLICache(newCLICacheTestUpdate()))

	cache := readCLICache(t, cachePath)
	assert.Contains(t, cache["Account"], azOwnedAccountKey, "az-owned account must survive the update")
	assert.Contains(t, cache["RefreshToken"], "az-owned-refresh-token", "az-owned refresh token must survive the update")
	assert.Len(t, cache["Account"], 2, "the Atmos account is added next to the az-owned one")
	assert.NotEmpty(t, cache[azureCloud.FieldAccessToken], "the Atmos access token is written")
}

// TestUpdateAzureCLICache_LeavesUnparseableCacheUntouched verifies that an
// unparseable cache (for example a partial write) is never reset to {} and
// written back: the file must be byte-for-byte unchanged.
func TestUpdateAzureCLICache_LeavesUnparseableCacheUntouched(t *testing.T) {
	cachePath := sandboxCLICacheHome(t)
	corrupt := []byte(`{"Account": {"az-owned-account": {"username": "me@exa`)
	require.NoError(t, os.WriteFile(cachePath, corrupt, 0o600))

	// Authentication must not fail because the az cache is damaged.
	require.NoError(t, newCLICacheTestProvider().updateAzureCLICache(newCLICacheTestUpdate()))

	data, err := os.ReadFile(cachePath)
	require.NoError(t, err)
	assert.Equal(t, string(corrupt), string(data), "an unparseable az cache must be left exactly as it was")
}

// TestUpdateAzureCLICache_WriteFailureIsNonFatal verifies a cache update that fails
// for a reason other than an unparseable file (here ".azure" is a regular file, so
// the directory cannot be created) still lets authentication succeed.
func TestUpdateAzureCLICache_WriteFailureIsNonFatal(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)
	t.Setenv("AZURE_CONFIG_DIR", tmpHome)
	azurePath := filepath.Join(tmpHome, ".azure")
	require.NoError(t, os.WriteFile(azurePath, []byte("not a directory"), 0o600))

	require.NoError(t, newCLICacheTestProvider().updateAzureCLICache(newCLICacheTestUpdate()))

	data, err := os.ReadFile(azurePath)
	require.NoError(t, err)
	assert.Equal(t, "not a directory", string(data))
}

// TestUpdateAzureCLICache_ReadsUnderMSALExtensionsLock verifies the read, update,
// and write all happen under the lock file msal-extensions (and therefore az)
// uses. While that lock is held, a concurrent az write lands in the file; the
// Atmos update must wait for the lock and then keep that write.
func TestUpdateAzureCLICache_ReadsUnderMSALExtensionsLock(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows FileLock is a no-op, so lock exclusion cannot be observed")
	}
	cachePath := sandboxCLICacheHome(t)
	require.NoError(t, os.WriteFile(cachePath, []byte(`{}`), 0o600))

	locked := make(chan struct{})
	release := make(chan struct{})
	holderDone := make(chan error, 1)
	go func() {
		holderDone <- filelock.New(cachePath+".lockfile").WithExclusive(context.Background(), func() error {
			close(locked)
			<-release
			// Simulate az persisting its own login while it holds the lock.
			return os.WriteFile(cachePath, azOwnedCacheJSON(t), 0o600)
		})
	}()
	<-locked

	updateDone := make(chan error, 1)
	go func() {
		updateDone <- newCLICacheTestProvider().updateAzureCLICache(newCLICacheTestUpdate())
	}()

	select {
	case err := <-updateDone:
		t.Fatalf("update finished (err=%v) while the msal-extensions lock was held", err)
	case <-time.After(200 * time.Millisecond):
	}

	close(release)
	require.NoError(t, <-holderDone)
	require.NoError(t, <-updateDone)

	cache := readCLICache(t, cachePath)
	assert.Contains(t, cache["Account"], azOwnedAccountKey, "the write az made while holding the lock must survive")
	assert.NotEmpty(t, cache[azureCloud.FieldAccessToken], "the Atmos access token is written")
}
