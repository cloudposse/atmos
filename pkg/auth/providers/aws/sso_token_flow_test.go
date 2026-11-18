package aws

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssooidc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time guards for the schema fields these tests rely on.
var _ = schema.Provider{Kind: "aws/iam-identity-center", StartURL: "", Region: "", Spec: nil}

// fakeOIDCTokenRequest is the subset of the SSO OIDC CreateToken request body the fake server inspects.
type fakeOIDCTokenRequest struct {
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
	GrantType    string `json:"grantType"`
	DeviceCode   string `json:"deviceCode"`
	RefreshToken string `json:"refreshToken"`
}

// fakeOIDCResponse is one scripted reply of the fake SSO OIDC /token endpoint.
type fakeOIDCResponse struct {
	status    int
	errorType string
	body      string
}

// fakeOIDCServer serves scripted /token replies and records every request it receives.
type fakeOIDCServer struct {
	*httptest.Server
	mu        sync.Mutex
	requests  []fakeOIDCTokenRequest
	responses []fakeOIDCResponse
	calls     atomic.Int32
}

func newFakeOIDCServer(t *testing.T, responses ...fakeOIDCResponse) *fakeOIDCServer {
	t.Helper()

	f := &fakeOIDCServer{responses: responses}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" {
			http.NotFound(w, r)
			return
		}
		var req fakeOIDCTokenRequest
		_ = json.NewDecoder(r.Body).Decode(&req)

		idx := int(f.calls.Add(1)) - 1
		f.mu.Lock()
		f.requests = append(f.requests, req)
		f.mu.Unlock()

		if idx >= len(f.responses) {
			idx = len(f.responses) - 1
		}
		resp := f.responses[idx]
		w.Header().Set("Content-Type", "application/json")
		if resp.errorType != "" {
			w.Header().Set("X-Amzn-ErrorType", resp.errorType)
		}
		w.WriteHeader(resp.status)
		_, _ = w.Write([]byte(resp.body))
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeOIDCServer) recorded() []fakeOIDCTokenRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]fakeOIDCTokenRequest(nil), f.requests...)
}

// newFakeOIDCClient builds a real SSO OIDC client that talks to the fake server without retries.
func newFakeOIDCClient(endpoint string) *ssooidc.Client {
	return ssooidc.New(ssooidc.Options{
		Region:       "us-east-1",
		BaseEndpoint: aws.String(endpoint),
		Credentials:  aws.AnonymousCredentials{},
		Retryer:      aws.NopRetryer{},
	})
}

var (
	pendingReply = fakeOIDCResponse{
		status:    http.StatusBadRequest,
		errorType: "AuthorizationPendingException",
		body:      `{"error":"authorization_pending"}`,
	}
	accessDeniedReply = fakeOIDCResponse{
		status:    http.StatusBadRequest,
		errorType: "AccessDeniedException",
		body:      `{"error":"access_denied"}`,
	}
)

func tokenReply(accessToken, refreshToken string, expiresIn int) fakeOIDCResponse {
	body, _ := json.Marshal(map[string]any{
		"accessToken":  accessToken,
		"refreshToken": refreshToken,
		"expiresIn":    expiresIn,
		"tokenType":    "Bearer",
	})
	return fakeOIDCResponse{status: http.StatusOK, body: string(body)}
}

func TestPollForAccessToken(t *testing.T) {
	tests := []struct {
		name          string
		responses     []fakeOIDCResponse
		expiresIn     int32
		wantCalls     int32
		wantAccess    string
		wantRefresh   string
		wantErrSubstr string
	}{
		{
			name:        "token issued on first poll",
			responses:   []fakeOIDCResponse{tokenReply("access-1", "refresh-1", 3600)},
			expiresIn:   600,
			wantCalls:   1,
			wantAccess:  "access-1",
			wantRefresh: "refresh-1",
		},
		{
			name:        "authorization pending is retried until the user approves",
			responses:   []fakeOIDCResponse{pendingReply, tokenReply("access-2", "", 3600)},
			expiresIn:   600,
			wantCalls:   2,
			wantAccess:  "access-2",
			wantRefresh: "",
		},
		{
			name:          "terminal OIDC error stops polling",
			responses:     []fakeOIDCResponse{accessDeniedReply, tokenReply("never", "", 3600)},
			expiresIn:     600,
			wantCalls:     1,
			wantErrSubstr: "Failed to create AWS SSO access token",
		},
		{
			name:          "device code expires while authorization is still pending",
			responses:     []fakeOIDCResponse{pendingReply},
			expiresIn:     1,
			wantCalls:     1,
			wantErrSubstr: "timed out",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Every case sleeps at least one polling interval; run them concurrently.
			t.Parallel()

			srv := newFakeOIDCServer(t, tt.responses...)
			p := &ssoProvider{name: "sso", startURL: "https://example.awsapps.com/start", region: "us-east-1"}

			before := time.Now()
			result, err := p.pollForAccessToken(context.Background(), newFakeOIDCClient(srv.URL),
				&ssooidc.RegisterClientOutput{ClientId: aws.String("client-id"), ClientSecret: aws.String("client-secret")},
				&ssooidc.StartDeviceAuthorizationOutput{DeviceCode: aws.String("device-code"), ExpiresIn: tt.expiresIn, Interval: 1},
			)

			assert.Equal(t, tt.wantCalls, srv.calls.Load(), "number of CreateToken calls")
			reqs := srv.recorded()
			require.NotEmpty(t, reqs)
			assert.Equal(t, "urn:ietf:params:oauth:grant-type:device_code", reqs[0].GrantType)
			assert.Equal(t, "client-id", reqs[0].ClientID)
			assert.Equal(t, "client-secret", reqs[0].ClientSecret)
			assert.Equal(t, "device-code", reqs[0].DeviceCode)

			if tt.wantErrSubstr != "" {
				require.Error(t, err)
				assert.ErrorIs(t, err, errUtils.ErrSSOTokenCreationFailed)
				assert.Contains(t, errUtils.Format(err, errUtils.DefaultFormatterConfig()), tt.wantErrSubstr)
				assert.Empty(t, result.AccessToken)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantAccess, result.AccessToken)
			assert.Equal(t, tt.wantRefresh, result.RefreshToken)
			// ExpiresAt is derived from the token's ExpiresIn, relative to the poll time.
			assert.WithinDuration(t, before.Add(time.Hour), result.ExpiresAt, 30*time.Second)
		})
	}
}

// newRefreshTestProvider builds an SSO provider whose OIDC endpoint is the fake
// server and whose token cache lives in a temporary XDG cache directory.
func newRefreshTestProvider(t *testing.T, endpoint string) *ssoProvider {
	t.Helper()
	t.Setenv("ATMOS_XDG_CACHE_HOME", t.TempDir())

	p, err := NewSSOProvider("sso", &schema.Provider{
		Kind:     "aws/iam-identity-center",
		StartURL: "https://example.awsapps.com/start",
		Region:   "us-east-1",
		Spec:     map[string]any{"endpoint_url": endpoint},
	})
	require.NoError(t, err)
	// Isolate from the process-wide in-memory session store.
	p.sessionStore = newSessionTokenStore()
	return p
}

func TestSSOProviderAuthenticate_RefreshesExpiredTokenWithoutBrowser(t *testing.T) {
	srv := newFakeOIDCServer(t, tokenReply("new-access", "rotated-refresh", 3600))
	p := newRefreshTestProvider(t, srv.URL)

	registrationExpiry := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Second)
	require.NoError(t, p.saveFullCachedToken(ssoTokenCache{
		AccessToken:           "old-access",
		ExpiresAt:             time.Now().Add(-time.Minute),
		Region:                p.region,
		StartURL:              p.startURL,
		RefreshToken:          "old-refresh",
		ClientID:              "cached-client",
		ClientSecret:          "cached-secret",
		RegistrationExpiresAt: registrationExpiry,
	}))

	creds, err := p.Authenticate(context.Background())
	require.NoError(t, err)

	awsCreds, ok := creds.(*authTypes.AWSCredentials)
	require.True(t, ok)
	assert.Equal(t, "new-access", awsCreds.AccessKeyID)
	assert.Equal(t, "us-east-1", awsCreds.Region)
	expiresAt, err := time.Parse(time.RFC3339, awsCreds.Expiration)
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now().Add(time.Hour), expiresAt, time.Minute)

	// The refresh used the cached registration and refresh token.
	reqs := srv.recorded()
	require.Len(t, reqs, 1)
	assert.Equal(t, ssoRefreshGrantType, reqs[0].GrantType)
	assert.Equal(t, "old-refresh", reqs[0].RefreshToken)
	assert.Equal(t, "cached-client", reqs[0].ClientID)
	assert.Equal(t, "cached-secret", reqs[0].ClientSecret)

	// The rotated refresh token and registration survive on disk for the next refresh.
	onDisk, ok := p.loadFullCachedToken()
	require.True(t, ok, "refreshed token must be persisted")
	assert.Equal(t, "new-access", onDisk.AccessToken)
	assert.Equal(t, "rotated-refresh", onDisk.RefreshToken)
	assert.Equal(t, "cached-client", onDisk.ClientID)
	assert.True(t, registrationExpiry.Equal(onDisk.RegistrationExpiresAt))

	// A second call is served from memory without contacting the OIDC service.
	_, err = p.Authenticate(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int32(1), srv.calls.Load())
}

func TestSSOProviderAuthenticate_FailedRefreshInNonInteractiveSessionFails(t *testing.T) {
	if isTTY() {
		t.Skipf("stderr is a terminal; the device-authorization fallback would start an interactive flow")
	}

	srv := newFakeOIDCServer(t, fakeOIDCResponse{
		status:    http.StatusBadRequest,
		errorType: "InvalidGrantException",
		body:      `{"error":"invalid_grant"}`,
	})
	p := newRefreshTestProvider(t, srv.URL)
	require.NoError(t, p.saveFullCachedToken(ssoTokenCache{
		AccessToken:  "old-access",
		ExpiresAt:    time.Now().Add(-time.Minute),
		Region:       p.region,
		StartURL:     p.startURL,
		RefreshToken: "revoked-refresh",
		ClientID:     "cached-client",
		ClientSecret: "cached-secret",
	}))

	creds, err := p.Authenticate(context.Background())
	require.Error(t, err)
	assert.Nil(t, creds)
	assert.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
	assert.Equal(t, int32(1), srv.calls.Load(), "refresh is attempted exactly once before giving up")
}

func TestSSOProviderSaveFullCachedToken_RestrictsFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skipf("POSIX file modes are not enforced on Windows")
	}

	p := newRefreshTestProvider(t, "https://unused.example.com")
	require.NoError(t, p.saveFullCachedToken(ssoTokenCache{
		AccessToken:  "secret-access",
		ExpiresAt:    time.Now().Add(time.Hour),
		Region:       p.region,
		StartURL:     p.startURL,
		RefreshToken: "secret-refresh",
	}))

	path, err := p.getTokenCachePath()
	require.NoError(t, err)
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(ssoTokenCacheFilePerms), info.Mode().Perm(), "token cache holds bearer and refresh tokens")

	dirInfo, err := os.Stat(filepath.Dir(path))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(ssoTokenCacheDirPerms), dirInfo.Mode().Perm())
}
