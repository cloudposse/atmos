package aws

// Tests that the on-disk SSO token cache is safe to share between concurrent
// Atmos processes, such as parallel kubectl exec-plugin calls.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestSSOTokenCache_ReadersNeverSeeTruncatedFile rewrites the cache file while
// another goroutine reads it and verifies every read parses: a reader must see
// either the old or the new token, never a truncated or empty file.
func TestSSOTokenCache_ReadersNeverSeeTruncatedFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		// pkg/filesystem replaces a file on Windows by removing it and renaming
		// the new one into place, so an unlocked reader can briefly find no file.
		// Authenticate reads and writes the cache under its file lock instead.
		t.Skipf("file replacement is not atomic for unlocked readers on Windows")
	}

	p := newRefreshTestProvider(t, "https://unused.example.com")
	token := ssoTokenCache{
		AccessToken: "access", ExpiresAt: time.Now().Add(time.Hour),
		Region: p.region, StartURL: p.startURL, RefreshToken: "refresh",
	}
	require.NoError(t, p.saveFullCachedToken(token))

	var done atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer done.Store(true)
		for i := 0; i < 300; i++ {
			_ = p.saveFullCachedToken(token)
		}
	}()

	var reads, misses int
	for !done.Load() {
		reads++
		if _, ok := p.readCacheFile(); !ok {
			misses++
		}
	}
	wg.Wait()

	require.Positive(t, reads, "the reader must overlap the writer")
	assert.Zero(t, misses, "%d of %d reads saw a partially written cache file", misses, reads)
}

// rotatingRefreshServer is an SSO OIDC /token endpoint that, like AWS, rotates
// the refresh token: the first exchange of a refresh token succeeds (slowly)
// and any later exchange of the same token is rejected with invalid_grant.
type rotatingRefreshServer struct {
	*httptest.Server
	used  sync.Map
	calls atomic.Int32
}

func newRotatingRefreshServer(t *testing.T) *rotatingRefreshServer {
	t.Helper()

	s := &rotatingRefreshServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.calls.Add(1)
		var req fakeOIDCTokenRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		if _, reused := s.used.LoadOrStore(req.RefreshToken, true); reused {
			w.Header().Set("X-Amzn-ErrorType", "InvalidGrantException")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}
		// A slow exchange leaves room for a concurrent process to try the same token.
		time.Sleep(300 * time.Millisecond)
		reply := tokenReply("new-access", "rotated-refresh")
		w.WriteHeader(reply.status)
		_, _ = w.Write([]byte(reply.body))
	}))
	t.Cleanup(s.Close)
	return s
}

// TestSSOProviderAuthenticate_ConcurrentProcessesRefreshOnce simulates two
// Atmos processes (separate in-memory session stores, one on-disk cache) that
// find the same expired token at once. Only one may exchange the refresh token;
// the other must pick up the rotated token from disk instead of replaying the
// old refresh token, which AWS rejects.
func TestSSOProviderAuthenticate_ConcurrentProcessesRefreshOnce(t *testing.T) {
	if isInteractive() {
		t.Skipf("the session is interactive; a failed refresh would start the device-authorization flow")
	}

	srv := newRotatingRefreshServer(t)
	t.Setenv("ATMOS_XDG_CACHE_HOME", t.TempDir())
	t.Setenv("AWS_USE_FIPS_ENDPOINT", "false")
	t.Setenv("AWS_USE_DUALSTACK_ENDPOINT", "false")
	t.Setenv("AWS_ENDPOINT_URL_SSO_OIDC", srv.URL)

	newProcess := func() *ssoProvider {
		p, err := NewSSOProvider("sso", &schema.Provider{
			Kind:     "aws/iam-identity-center",
			StartURL: "https://example.awsapps.com/start",
			Region:   "us-east-1",
			Spec:     map[string]any{"endpoint_url": srv.URL},
		})
		require.NoError(t, err)
		p.sessionStore = newSessionTokenStore()
		return p
	}
	first, second := newProcess(), newProcess()
	require.NoError(t, first.saveFullCachedToken(ssoTokenCache{
		AccessToken:  "old-access",
		ExpiresAt:    time.Now().Add(-time.Minute),
		Region:       first.region,
		StartURL:     first.startURL,
		RefreshToken: "old-refresh",
		ClientID:     "cached-client",
		ClientSecret: "cached-secret",
	}))

	var wg sync.WaitGroup
	results := make([]authTypes.ICredentials, 2)
	errs := make([]error, 2)
	for i, p := range []*ssoProvider{first, second} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], errs[i] = p.Authenticate(context.Background())
		}()
	}
	wg.Wait()

	for i := range results {
		require.NoError(t, errs[i], "process %d", i)
		awsCreds, ok := results[i].(*authTypes.AWSCredentials)
		require.True(t, ok)
		assert.Equal(t, "new-access", awsCreds.AccessKeyID, "process %d", i)
	}
	assert.Equal(t, int32(1), srv.calls.Load(), "the refresh token must be exchanged exactly once")
}
