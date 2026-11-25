package aws

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	ssotypes "github.com/aws/aws-sdk-go-v2/service/sso/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time guard for the schema fields the permission-set tests rely on.
var _ = schema.Identity{Kind: "aws/permission-set", Principal: nil, Spec: nil}

// fakeSSOPortal serves the two AWS SSO portal operations a permission-set login uses.
type fakeSSOPortal struct {
	listCalls  atomic.Int32
	credsCalls atomic.Int32
	// credsStatus, when non-zero, makes GetRoleCredentials fail with UnauthorizedException.
	credsStatus int
}

func (f *fakeSSOPortal) handler(t *testing.T) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "sso-bearer-token", r.Header.Get("x-amz-sso_bearer_token"), "the SSO access token is the bearer token")
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/assignment/accounts":
			f.listCalls.Add(1)
			_, _ = w.Write([]byte(`{"accountList":[{"accountId":"111111111111","accountName":"staging"},{"accountId":"222222222222","accountName":"production"}]}`))
		case "/federation/credentials":
			f.credsCalls.Add(1)
			assert.Equal(t, "222222222222", r.URL.Query().Get("account_id"))
			assert.Equal(t, "AdminAccess", r.URL.Query().Get("role_name"))
			if f.credsStatus != 0 {
				w.Header().Set("X-Amzn-ErrorType", "UnauthorizedException")
				w.WriteHeader(f.credsStatus)
				_, _ = w.Write([]byte(`{"message":"session expired"}`))
				return
			}
			// Expiration is milliseconds since the Unix epoch: 2030-01-02T03:04:05Z.
			_, _ = w.Write([]byte(`{"roleCredentials":{"accessKeyId":"ASIAROLE","secretAccessKey":"role-secret","sessionToken":"role-session","expiration":1893553445000}}`))
		default:
			t.Errorf("unexpected SSO portal request: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

func newPortalPermissionSet(t *testing.T, endpoint string, account map[string]any) *permissionSetIdentity {
	t.Helper()
	identity, err := NewPermissionSetIdentity("prod-admin", &schema.Identity{
		Kind: "aws/permission-set",
		Principal: map[string]any{
			"name":    "AdminAccess",
			"account": account,
		},
		Spec: map[string]any{"endpoint_url": endpoint},
	})
	require.NoError(t, err)
	ps, ok := identity.(*permissionSetIdentity)
	require.True(t, ok)
	return ps
}

func TestPermissionSetIdentity_Authenticate_AgainstPortal(t *testing.T) {
	ssoBase := &types.AWSCredentials{AccessKeyID: "sso-bearer-token", Region: "us-east-1"}

	tests := []struct {
		name          string
		account       map[string]any
		wantListCalls int32
	}{
		{
			name:          "account id is used directly",
			account:       map[string]any{"id": "222222222222"},
			wantListCalls: 0,
		},
		{
			name:          "account name is resolved through the portal",
			account:       map[string]any{"name": "production"},
			wantListCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ATMOS_XDG_CACHE_HOME", t.TempDir())
			portal := &fakeSSOPortal{}
			srv := httptest.NewServer(portal.handler(t))
			t.Cleanup(srv.Close)

			ps := newPortalPermissionSet(t, srv.URL, tt.account)
			creds, err := ps.Authenticate(context.Background(), ssoBase)
			require.NoError(t, err)

			awsCreds, ok := creds.(*types.AWSCredentials)
			require.True(t, ok)
			assert.Equal(t, "ASIAROLE", awsCreds.AccessKeyID)
			assert.Equal(t, "role-secret", awsCreds.SecretAccessKey)
			assert.Equal(t, "role-session", awsCreds.SessionToken)
			assert.Equal(t, "us-east-1", awsCreds.Region)
			// The portal reports milliseconds; the credential must carry the matching wall-clock time.
			want := time.Date(2030, time.January, 2, 3, 4, 5, 0, time.UTC)
			got, err := time.Parse(time.RFC3339, awsCreds.Expiration)
			require.NoError(t, err)
			assert.True(t, want.Equal(got), "expiration %s, want %s", got, want)

			assert.Equal(t, tt.wantListCalls, portal.listCalls.Load())
			assert.Equal(t, int32(1), portal.credsCalls.Load())
		})
	}
}

func TestPermissionSetIdentity_Authenticate_ReusesResolvedAccountID(t *testing.T) {
	t.Setenv("ATMOS_XDG_CACHE_HOME", t.TempDir())
	portal := &fakeSSOPortal{}
	srv := httptest.NewServer(portal.handler(t))
	t.Cleanup(srv.Close)

	ssoBase := &types.AWSCredentials{AccessKeyID: "sso-bearer-token", Region: "us-east-1"}
	ps := newPortalPermissionSet(t, srv.URL, map[string]any{"name": "production"})

	for range 2 {
		_, err := ps.Authenticate(context.Background(), ssoBase)
		require.NoError(t, err)
	}

	// The account name is listed once per SSO session; later logins reuse the cached ID.
	assert.Equal(t, int32(1), portal.listCalls.Load())
	assert.Equal(t, int32(2), portal.credsCalls.Load())
}

func TestPermissionSetIdentity_Authenticate_GetRoleCredentialsError(t *testing.T) {
	t.Setenv("ATMOS_XDG_CACHE_HOME", t.TempDir())
	portal := &fakeSSOPortal{credsStatus: http.StatusUnauthorized}
	srv := httptest.NewServer(portal.handler(t))
	t.Cleanup(srv.Close)

	ps := newPortalPermissionSet(t, srv.URL, map[string]any{"id": "222222222222"})
	creds, err := ps.Authenticate(context.Background(), &types.AWSCredentials{AccessKeyID: "sso-bearer-token", Region: "us-east-1"})

	require.Error(t, err)
	assert.Nil(t, creds)
	assert.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
	var unauthorized *ssotypes.UnauthorizedException
	require.ErrorAs(t, err, &unauthorized, "the portal error must stay in the chain")
	assert.Equal(t, "session expired", unauthorized.ErrorMessage())
}
