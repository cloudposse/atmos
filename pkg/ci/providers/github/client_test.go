package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

const defaultAPIBaseURL = "https://api.github.com/"

// clearAPIURLEnv blanks every variable NewClient consults so tests are hermetic.
func clearAPIURLEnv(t *testing.T) {
	t.Helper()
	t.Setenv("ATMOS_CI_GITHUB_API_URL", "")
	t.Setenv("GITHUB_API_URL", "")
	t.Setenv("ATMOS_CI_GITHUB_TOKEN", "")
	t.Setenv("ATMOS_PRO_GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "test-token")
}

func TestNewClient_APIURL(t *testing.T) {
	tests := []struct {
		name        string
		ciURL       string
		githubURL   string
		wantBaseURL string
		wantErr     bool
	}{
		{name: "default when unset", wantBaseURL: defaultAPIBaseURL},
		{name: "GITHUB_API_URL gets trailing slash", githubURL: "http://127.0.0.1:8080", wantBaseURL: "http://127.0.0.1:8080/"},
		{name: "GITHUB_API_URL keeps existing trailing slash", githubURL: "http://127.0.0.1:8080/", wantBaseURL: "http://127.0.0.1:8080/"},
		{name: "GHES path preserved", githubURL: "https://ghes.example.com/api/v3", wantBaseURL: "https://ghes.example.com/api/v3/"},
		{name: "ATMOS_CI_GITHUB_API_URL wins", ciURL: "http://override.example.com/api", githubURL: "https://ghes.example.com/api/v3", wantBaseURL: "http://override.example.com/api/"},
		{name: "unparsable URL errors", githubURL: "http://[::1", wantErr: true},
		{name: "relative URL errors", githubURL: "not-a-url", wantErr: true},
		{name: "unsupported scheme errors", githubURL: "ftp://example.com", wantErr: true},
		{name: "invalid override errors even when GITHUB_API_URL is valid", ciURL: "://bad", githubURL: "https://ghes.example.com/api/v3", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearAPIURLEnv(t)
			t.Setenv("ATMOS_CI_GITHUB_API_URL", tt.ciURL)
			t.Setenv("GITHUB_API_URL", tt.githubURL)

			client, err := NewClient()
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, errUtils.ErrInvalidURL)
				assert.Nil(t, client)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, client)
			assert.Equal(t, tt.wantBaseURL, client.GitHub().BaseURL.String())
			if tt.wantBaseURL != defaultAPIBaseURL {
				assert.Equal(t, tt.wantBaseURL, client.GitHub().UploadURL.String())
			}
		})
	}
}

func TestNewClient_TokenPrecedenceUnchanged(t *testing.T) {
	clearAPIURLEnv(t)
	t.Setenv("GITHUB_TOKEN", "")

	_, err := NewClient()
	require.ErrorIs(t, err, errUtils.ErrGitHubTokenNotFound)

	t.Setenv("GH_TOKEN", "gh")
	client, err := NewClient()
	require.NoError(t, err)
	assert.NotNil(t, client)
}

// TestNewClient_HTTPSOnlyToken checks the wire request, including redirect hops.
func TestNewClient_HTTPSOnlyToken(t *testing.T) {
	for _, tc := range []struct {
		name             string
		secure, redirect bool
	}{
		{"HTTP override", false, false},
		{"HTTPS override", true, false},
		{"HTTPS redirects to HTTP", true, true},
		{"HTTP redirects to HTTPS", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clearAPIURLEnv(t)
			httpAuth := make(chan string, 1)
			httpsAuth := make(chan string, 1)
			var plainURL, secureURL string
			handler := func(auth chan string, secure bool) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					auth <- r.Header.Get("Authorization")
					if tc.redirect && secure == tc.secure {
						target := secureURL
						if secure {
							target = plainURL
						}
						http.Redirect(w, r, target+"/user", http.StatusFound)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"login":"test"}`))
				}
			}
			plain := httptest.NewServer(handler(httpAuth, false))
			defer plain.Close()
			secure := httptest.NewTLSServer(handler(httpsAuth, true))
			defer secure.Close()
			plainURL, secureURL = plain.URL, secure.URL
			base := plainURL
			if tc.secure {
				base = secureURL
			}
			t.Setenv("ATMOS_CI_GITHUB_API_URL", base)
			// Trust only the local test server's certificate without changing authentication.
			oldTransport := http.DefaultTransport
			http.DefaultTransport = secure.Client().Transport
			t.Cleanup(func() { http.DefaultTransport = oldTransport })
			client, err := NewClient()
			require.NoError(t, err)
			user, _, err := client.GitHub().Users.Get(context.Background(), "")
			require.NoError(t, err)
			assert.Equal(t, "test", user.GetLogin())
			if !tc.secure || tc.redirect {
				assert.Empty(t, <-httpAuth, "HTTP must never receive credentials")
			}
			if tc.secure || tc.redirect {
				assert.Equal(t, "Bearer test-token", <-httpsAuth, "HTTPS keeps authentication")
			}
		})
	}
}

func TestNewClientWithToken_HTTPStripsAuthorization(t *testing.T) {
	received := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	client := NewClientWithToken("test-token")
	base, err := url.Parse(server.URL + "/")
	require.NoError(t, err)
	client.GitHub().BaseURL = base
	req, err := client.GitHub().NewRequest(http.MethodGet, "user", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer copied-on-redirect")
	_, err = client.GitHub().Do(context.Background(), req, nil)
	require.NoError(t, err)
	assert.Empty(t, <-received)
	assert.Equal(t, "Bearer copied-on-redirect", req.Header.Get("Authorization"), "transport must not mutate caller headers")
}
