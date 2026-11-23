package credentials

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/creds"
)

var errStore = errors.New("store failure")

// useHelper swaps CurrentHelper for the duration of a test.
func useHelper(t *testing.T, h Helper) {
	t.Helper()
	orig := CurrentHelper
	CurrentHelper = h
	t.Cleanup(func() { CurrentHelper = orig })
}

func TestDefaultHelper(t *testing.T) {
	h := defaultHelper{}

	assert.NoError(t, h.Add(&Credentials{ServerURL: "https://idp.example.com"}))
	assert.NoError(t, h.Delete("https://idp.example.com"))
	assert.False(t, h.SupportsCredentialStorage())

	user, secret, err := h.Get("https://idp.example.com")
	assert.ErrorIs(t, err, ErrCredentialsNotFound)
	assert.Empty(t, user)
	assert.Empty(t, secret)
}

func TestIsErrCredentialsNotFound(t *testing.T) {
	assert.True(t, IsErrCredentialsNotFound(ErrCredentialsNotFound))
	assert.False(t, IsErrCredentialsNotFound(errStore))
	assert.False(t, IsErrCredentialsNotFound(nil))
}

func TestLookupCredentialsUsesDefaultHelper(t *testing.T) {
	useHelper(t, &defaultHelper{})

	err := LookupCredentials(&creds.LoginDetails{URL: "https://idp.example.com"}, "ADFS")
	assert.True(t, IsErrCredentialsNotFound(err))
}

func TestLookupCredentials(t *testing.T) {
	const (
		idpURL    = "https://idp.example.com"
		cookieKey = idpURL + "/sessionCookie"
		// The OneLogin token key is built with path.Join, which collapses the "//" in the scheme.
		oneLoginKey = "https:/idp.example.com/auth/oauth2/v2/token"
	)

	// Each case lists exactly the store lookups it expects, in order; the controller fails the test on any
	// other lookup and on a missing one.
	found := func(user, secret string) func(*MockHelper, string) *gomock.Call {
		return func(h *MockHelper, key string) *gomock.Call {
			return h.EXPECT().Get(key).Return(user, secret, nil).Times(1)
		}
	}
	failing := func(err error) func(*MockHelper, string) *gomock.Call {
		return func(h *MockHelper, key string) *gomock.Call {
			return h.EXPECT().Get(key).Return("", "", err).Times(1)
		}
	}
	type lookup struct {
		key    string
		answer func(*MockHelper, string) *gomock.Call
	}

	tests := []struct {
		name          string
		provider      string
		lookups       []lookup
		wantErr       error
		wantUsername  string
		wantPassword  string
		wantCookie    string
		wantClientID  string
		wantClientSec string
	}{
		{
			name:     "not found",
			provider: "ADFS",
			lookups:  []lookup{{idpURL, failing(ErrCredentialsNotFound)}},
			wantErr:  ErrCredentialsNotFound,
		},
		{
			name:     "store error",
			provider: "ADFS",
			lookups:  []lookup{{idpURL, failing(errStore)}},
			wantErr:  errStore,
		},
		{
			name:         "username and password",
			provider:     "ADFS",
			lookups:      []lookup{{idpURL, found("alice", "pw")}},
			wantUsername: "alice",
			wantPassword: "pw",
		},
		{
			name:         "okta session cookie",
			provider:     "Okta",
			lookups:      []lookup{{idpURL, found("alice", "pw")}, {cookieKey, found("alice", "sid-123")}},
			wantUsername: "alice",
			wantPassword: "pw",
			wantCookie:   "sid-123",
		},
		{
			name:         "okta without session cookie",
			provider:     "Okta",
			lookups:      []lookup{{idpURL, found("alice", "pw")}, {cookieKey, failing(ErrCredentialsNotFound)}},
			wantUsername: "alice",
			wantPassword: "pw",
		},
		{
			// The session cookie is never looked up for other providers.
			name:         "session cookie ignored for other providers",
			provider:     "ADFS",
			lookups:      []lookup{{idpURL, found("alice", "pw")}},
			wantUsername: "alice",
			wantPassword: "pw",
		},
		{
			name:          "onelogin client credentials",
			provider:      "OneLogin",
			lookups:       []lookup{{idpURL, found("alice", "pw")}, {oneLoginKey, found("client-id", "client-secret")}},
			wantUsername:  "alice",
			wantPassword:  "pw",
			wantClientID:  "client-id",
			wantClientSec: "client-secret",
		},
		{
			name:     "onelogin client credentials missing",
			provider: "OneLogin",
			lookups:  []lookup{{idpURL, found("alice", "pw")}, {oneLoginKey, failing(ErrCredentialsNotFound)}},
			wantErr:  ErrCredentialsNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewMockHelper(gomock.NewController(t))
			calls := make([]any, 0, len(tt.lookups))
			for _, l := range tt.lookups {
				calls = append(calls, l.answer(h, l.key))
			}
			gomock.InOrder(calls...)
			useHelper(t, h)
			ld := &creds.LoginDetails{URL: idpURL}

			err := LookupCredentials(ld, tt.provider)
			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantUsername, ld.Username)
			assert.Equal(t, tt.wantPassword, ld.Password)
			assert.Equal(t, tt.wantCookie, ld.OktaSessionCookie)
			assert.Equal(t, tt.wantClientID, ld.ClientID)
			assert.Equal(t, tt.wantClientSec, ld.ClientSecret)
		})
	}
}

func TestSaveCredentials(t *testing.T) {
	h := NewMockHelper(gomock.NewController(t))
	want := &Credentials{ServerURL: "https://idp.example.com", Username: "alice", Secret: "pw"}
	// The first save succeeds and the second reports the store failure; each is stored exactly once.
	gomock.InOrder(
		h.EXPECT().Add(want).Return(nil).Times(1),
		h.EXPECT().Add(want).Return(errStore).Times(1),
	)
	useHelper(t, h)

	require.NoError(t, SaveCredentials("https://idp.example.com", "alice", "pw"))
	assert.ErrorIs(t, SaveCredentials("https://idp.example.com", "alice", "pw"), errStore)
}

func TestSupportsStorage(t *testing.T) {
	for _, supported := range []bool{true, false} {
		h := NewMockHelper(gomock.NewController(t))
		h.EXPECT().SupportsCredentialStorage().Return(supported).Times(1)
		useHelper(t, h)
		assert.Equal(t, supported, SupportsStorage())
	}
}
