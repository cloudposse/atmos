package aws

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sso"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

const cachedAccountResponse = `{"accountList":[{"accountId":"123456789012","accountName":"production"},{"accountId":"210987654321","accountName":"development"}]}`

func accountCacheTestClient(t *testing.T, body string, status int, calls *atomic.Int32) *sso.Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, err := w.Write([]byte(body))
		assert.NoError(t, err)
	}))
	t.Cleanup(server.Close)
	return sso.New(sso.Options{
		Region:       "us-east-1",
		BaseEndpoint: aws.String(server.URL),
		Credentials:  aws.AnonymousCredentials{},
		HTTPClient:   server.Client(),
		Retryer:      aws.NopRetryer{},
	})
}

func TestPermissionSetAccountCache_PersistsAcrossIdentityInstances(t *testing.T) {
	t.Setenv("ATMOS_XDG_CACHE_HOME", t.TempDir())
	var calls atomic.Int32
	client := accountCacheTestClient(t, cachedAccountResponse, http.StatusOK, &calls)
	first := &permissionSetIdentity{name: "first", realm: "test"}
	id, err := first.resolveAccountID(context.Background(), client, "production", "", "sso-access-token")
	require.NoError(t, err)
	assert.Equal(t, "123456789012", id)

	// A fresh instance and SDK client share only the on-disk cache, as separate commands would.
	second := &permissionSetIdentity{name: "second", realm: "test"}
	id, err = second.resolveAccountID(context.Background(), sso.New(client.Options()), "production", "", "sso-access-token")
	require.NoError(t, err)
	assert.Equal(t, "123456789012", id)
	assert.EqualValues(t, 1, calls.Load())

	path := first.accountCachePath(client, "production")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "sso-access-token")
	if runtime.GOOS != "windows" {
		info, statErr := os.Stat(path)
		require.NoError(t, statErr)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
		info, statErr = os.Stat(filepath.Dir(path))
		require.NoError(t, statErr)
		assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	}
}

func TestPermissionSetAccountCache_Isolation(t *testing.T) {
	for _, scope := range []string{"session", "realm", "region", "endpoint", "account"} {
		t.Run(scope, func(t *testing.T) {
			t.Setenv("ATMOS_XDG_CACHE_HOME", t.TempDir())
			var calls atomic.Int32
			client := accountCacheTestClient(t, cachedAccountResponse, http.StatusOK, &calls)
			identity := &permissionSetIdentity{realm: "original"}
			_, err := identity.resolveAccountID(context.Background(), client, "production", "", "original-token")
			require.NoError(t, err)

			second := &permissionSetIdentity{realm: "original"}
			accountName, token, wantID := "production", "original-token", "123456789012"
			switch scope {
			case "session":
				token = "new-session-token"
			case "realm":
				second.realm = "another-realm"
			case "region":
				options := client.Options()
				options.Region = "us-west-2"
				client = sso.New(options)
			case "endpoint":
				client = accountCacheTestClient(t, cachedAccountResponse, http.StatusOK, &calls)
			case "account":
				accountName, wantID = "development", "210987654321"
			}
			id, err := second.resolveAccountID(context.Background(), client, accountName, "", token)
			require.NoError(t, err)
			assert.Equal(t, wantID, id)
			assert.EqualValues(t, 2, calls.Load(), "a different scope must query SSO")
		})
	}
}

func TestPermissionSetAccountCache_InvalidEntriesAreRefreshed(t *testing.T) {
	for _, state := range []string{"expired", "corrupt", "empty ID"} {
		t.Run(state, func(t *testing.T) {
			t.Setenv("ATMOS_XDG_CACHE_HOME", t.TempDir())
			var calls atomic.Int32
			client := accountCacheTestClient(t, cachedAccountResponse, http.StatusOK, &calls)
			identity := &permissionSetIdentity{realm: "test"}
			path := identity.accountCachePath(client, "production")
			savePermissionSetAccountID(path, "token", "999999999999")
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			var entry permissionSetAccountCache
			require.NoError(t, json.Unmarshal(data, &entry))
			switch state {
			case "expired":
				entry.ExpiresAt = time.Now().Add(-time.Minute)
			case "empty ID":
				entry.AccountID = ""
			}
			data, err = json.Marshal(entry)
			require.NoError(t, err)
			if state == "corrupt" {
				data = []byte("invalid JSON")
			}
			require.NoError(t, os.WriteFile(path, data, 0o600))

			for range 2 {
				id, resolveErr := identity.resolveAccountID(context.Background(), client, "production", "", "token")
				require.NoError(t, resolveErr)
				assert.Equal(t, "123456789012", id)
			}
			assert.EqualValues(t, 1, calls.Load(), "refresh once and reuse the repaired cache")
		})
	}
}

func TestPermissionSetAccountCache_UnavailableStorageIsNonfatal(t *testing.T) {
	for _, failure := range []string{"directory", "entry"} {
		t.Run(failure, func(t *testing.T) {
			cacheRoot := t.TempDir()
			t.Setenv("ATMOS_XDG_CACHE_HOME", cacheRoot)
			var calls atomic.Int32
			client := accountCacheTestClient(t, cachedAccountResponse, http.StatusOK, &calls)
			identity := &permissionSetIdentity{realm: "test"}
			if failure == "directory" {
				blocker := filepath.Join(cacheRoot, "blocker")
				require.NoError(t, os.WriteFile(blocker, nil, 0o600))
				t.Setenv("ATMOS_XDG_CACHE_HOME", blocker)
			} else {
				// A directory in place of the entry makes both read and atomic write fail.
				path := identity.accountCachePath(client, "production")
				require.NoError(t, os.Mkdir(path, 0o700))
			}
			id, err := identity.resolveAccountID(context.Background(), client, "production", "", "token")
			require.NoError(t, err)
			assert.Equal(t, "123456789012", id)
			assert.EqualValues(t, 1, calls.Load())
		})
	}
}

func TestPermissionSetAccountCache_DoesNotCacheFailures(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusUnauthorized} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Setenv("ATMOS_XDG_CACHE_HOME", t.TempDir())
			var calls atomic.Int32
			client := accountCacheTestClient(t, `{"accountList":[]}`, status, &calls)
			identity := &permissionSetIdentity{realm: "test"}
			for range 2 {
				id, err := identity.resolveAccountID(context.Background(), client, "production", "", "token")
				require.ErrorIs(t, err, errUtils.ErrAwsAuth)
				assert.Empty(t, id)
			}
			assert.EqualValues(t, 2, calls.Load())
		})
	}
}
