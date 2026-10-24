package atmospro

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// withStubbedMintExchange replaces the package-level mint/exchange hooks for a test.
func withStubbedMintExchange(t *testing.T, oidcToken string, mintErr error, sessionJWT string, exchangeErr error) *struct {
	gotAudience               string
	gotBaseURL, gotEndpoint   string
	gotOIDCToken, gotWorkspce string
} {
	t.Helper()
	calls := &struct {
		gotAudience               string
		gotBaseURL, gotEndpoint   string
		gotOIDCToken, gotWorkspce string
	}{}

	oldMint, oldExchange := mintOIDCToken, exchangeOIDCToken
	mintOIDCToken = func(_ context.Context, audience string) (string, error) {
		calls.gotAudience = audience
		return oidcToken, mintErr
	}
	exchangeOIDCToken = func(baseURL, endpoint, oidc, workspaceID string) (string, error) {
		calls.gotBaseURL = baseURL
		calls.gotEndpoint = endpoint
		calls.gotOIDCToken = oidc
		calls.gotWorkspce = workspaceID
		return sessionJWT, exchangeErr
	}
	t.Cleanup(func() { mintOIDCToken, exchangeOIDCToken = oldMint, oldExchange })
	return calls
}

func newProvider(t *testing.T, spec map[string]interface{}) types.Provider {
	t.Helper()
	p, err := NewProvider("atmos-pro", &schema.Provider{Kind: Kind, Spec: spec})
	require.NoError(t, err)
	return p
}

func TestProvider_Authenticate_Success(t *testing.T) {
	calls := withStubbedMintExchange(t, "oidc-tok", nil, "session-jwt", nil)

	p := newProvider(t, map[string]interface{}{
		"base_url":     "https://pro.example.com",
		"endpoint":     "api/v1",
		"workspace_id": "ws-123",
		"audience":     "custom-aud",
	})

	creds, err := p.Authenticate(context.Background())
	require.NoError(t, err)

	pc, ok := creds.(*types.ProCredentials)
	require.True(t, ok)
	assert.Equal(t, "session-jwt", pc.Token)
	assert.Equal(t, "https://pro.example.com", pc.BaseURL)
	assert.Equal(t, "api/v1", pc.Endpoint)
	assert.Equal(t, "ws-123", pc.WorkspaceID)
	assert.Equal(t, "atmos-pro", pc.Provider)

	// Verify the OIDC token is exchanged exactly as minted with the configured audience.
	assert.Equal(t, "custom-aud", calls.gotAudience)
	assert.Equal(t, "oidc-tok", calls.gotOIDCToken)
	assert.Equal(t, "https://pro.example.com", calls.gotBaseURL)
	assert.Equal(t, "ws-123", calls.gotWorkspce)
}

func TestProvider_Authenticate_DefaultAudience(t *testing.T) {
	calls := withStubbedMintExchange(t, "oidc-tok", nil, "session-jwt", nil)

	p := newProvider(t, map[string]interface{}{"workspace_id": "ws-1"})
	_, err := p.Authenticate(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "atmos-pro.com", calls.gotAudience, "default audience must be atmos-pro.com")
}

func TestProvider_Authenticate_MissingWorkspaceID(t *testing.T) {
	withStubbedMintExchange(t, "oidc-tok", nil, "session-jwt", nil)
	// Ensure env doesn't supply a workspace id.
	t.Setenv("ATMOS_PRO_WORKSPACE_ID", "")

	p := newProvider(t, map[string]interface{}{})
	_, err := p.Authenticate(context.Background())
	require.ErrorIs(t, err, errUtils.ErrProWorkspaceIDMissing)
}

func TestProvider_Authenticate_MintError(t *testing.T) {
	withStubbedMintExchange(t, "", errors.New("not in actions"), "", nil)
	p := newProvider(t, map[string]interface{}{"workspace_id": "ws-1"})
	_, err := p.Authenticate(context.Background())
	require.ErrorIs(t, err, errUtils.ErrProAuthFailed)
}

func TestProvider_Authenticate_ExchangeError(t *testing.T) {
	withStubbedMintExchange(t, "oidc-tok", nil, "", errors.New("exchange failed"))
	p := newProvider(t, map[string]interface{}{"workspace_id": "ws-1"})
	_, err := p.Authenticate(context.Background())
	require.ErrorIs(t, err, errUtils.ErrProAuthFailed)
}

func TestProvider_KindAndLogout(t *testing.T) {
	p := newProvider(t, map[string]interface{}{})
	assert.Equal(t, "atmos/pro", p.Kind())
	assert.Equal(t, "atmos-pro", p.Name())
	require.ErrorIs(t, p.Logout(context.Background()), errUtils.ErrLogoutNotSupported)
}

// TestProProvider_IsNotAmbient is a regression guard, not a coverage filler.
//
// The atmos/pro provider mints a SINGLE-USE OIDC token that cannot be re-exchanged, so
// the auth manager's cached-credential reuse is load-bearing for it (see the
// EnsureIdentityEnvironment contract in pkg/auth/types/interfaces.go). Opting this
// provider into types.AmbientProvider would suppress that caching and break
// authentication outright.
//
// It is superficially similar to the providers that ARE ambient — short-lived token, no
// credential files — so this test pins the distinction that matters: what makes a
// provider ambient is that it can re-resolve, not that its token is short-lived.
func TestProProvider_IsNotAmbient(t *testing.T) {
	p := &proProvider{name: "atmos-pro"}

	assert.False(t, types.ProviderIsAmbient(p),
		"atmos/pro mints single-use tokens and MUST stay cacheable")

	_, isAmbient := any(p).(types.AmbientProvider)
	assert.False(t, isAmbient,
		"atmos/pro must not implement types.AmbientProvider at all")
}
