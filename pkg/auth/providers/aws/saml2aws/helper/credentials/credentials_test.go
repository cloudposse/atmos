package credentials

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/creds"
)

var errStore = errors.New("store failure")

type storedSecret struct {
	username string
	secret   string
}

// fakeHelper is an in-memory credentials store.
type fakeHelper struct {
	entries  map[string]storedSecret
	getErr   map[string]error
	added    []*Credentials
	addErr   error
	supports bool
}

func (f *fakeHelper) Add(c *Credentials) error {
	f.added = append(f.added, c)
	return f.addErr
}

func (f *fakeHelper) Delete(string) error { return nil }

func (f *fakeHelper) Get(serverURL string) (string, string, error) {
	if err, ok := f.getErr[serverURL]; ok {
		return "", "", err
	}
	entry, ok := f.entries[serverURL]
	if !ok {
		return "", "", ErrCredentialsNotFound
	}
	return entry.username, entry.secret, nil
}

func (f *fakeHelper) SupportsCredentialStorage() bool { return f.supports }

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
	const idpURL = "https://idp.example.com"

	tests := []struct {
		name          string
		provider      string
		helper        *fakeHelper
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
			helper:   &fakeHelper{},
			wantErr:  ErrCredentialsNotFound,
		},
		{
			name:     "store error",
			provider: "ADFS",
			helper:   &fakeHelper{getErr: map[string]error{idpURL: errStore}},
			wantErr:  errStore,
		},
		{
			name:         "username and password",
			provider:     "ADFS",
			helper:       &fakeHelper{entries: map[string]storedSecret{idpURL: {"alice", "pw"}}},
			wantUsername: "alice",
			wantPassword: "pw",
		},
		{
			name:     "okta session cookie",
			provider: "Okta",
			helper: &fakeHelper{entries: map[string]storedSecret{
				idpURL:                    {"alice", "pw"},
				idpURL + "/sessionCookie": {"alice", "sid-123"},
			}},
			wantUsername: "alice",
			wantPassword: "pw",
			wantCookie:   "sid-123",
		},
		{
			name:         "okta without session cookie",
			provider:     "Okta",
			helper:       &fakeHelper{entries: map[string]storedSecret{idpURL: {"alice", "pw"}}},
			wantUsername: "alice",
			wantPassword: "pw",
		},
		{
			name:     "session cookie ignored for other providers",
			provider: "ADFS",
			helper: &fakeHelper{entries: map[string]storedSecret{
				idpURL:                    {"alice", "pw"},
				idpURL + "/sessionCookie": {"alice", "sid-123"},
			}},
			wantUsername: "alice",
			wantPassword: "pw",
		},
		{
			// The OneLogin token key is built with path.Join, which collapses the "//" in the scheme.
			name:     "onelogin client credentials",
			provider: "OneLogin",
			helper: &fakeHelper{entries: map[string]storedSecret{
				idpURL: {"alice", "pw"},
				"https:/idp.example.com/auth/oauth2/v2/token": {"client-id", "client-secret"},
			}},
			wantUsername:  "alice",
			wantPassword:  "pw",
			wantClientID:  "client-id",
			wantClientSec: "client-secret",
		},
		{
			name:     "onelogin client credentials missing",
			provider: "OneLogin",
			helper:   &fakeHelper{entries: map[string]storedSecret{idpURL: {"alice", "pw"}}},
			wantErr:  ErrCredentialsNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useHelper(t, tt.helper)
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
	h := &fakeHelper{}
	useHelper(t, h)

	require.NoError(t, SaveCredentials("https://idp.example.com", "alice", "pw"))
	require.Len(t, h.added, 1)
	assert.Equal(t, &Credentials{ServerURL: "https://idp.example.com", Username: "alice", Secret: "pw"}, h.added[0])

	h.addErr = errStore
	assert.ErrorIs(t, SaveCredentials("https://idp.example.com", "alice", "pw"), errStore)
}

func TestSupportsStorage(t *testing.T) {
	useHelper(t, &fakeHelper{supports: true})
	assert.True(t, SupportsStorage())

	useHelper(t, &fakeHelper{supports: false})
	assert.False(t, SupportsStorage())
}
