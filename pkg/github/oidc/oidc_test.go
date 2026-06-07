package oidc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeJWT builds a JWT-shaped string (header.payload.signature) whose payload encodes claims.
// Only the payload segment is read by decodeClaims; the header/signature are placeholders.
func makeJWT(t *testing.T, claims map[string]any) string {
	t.Helper()
	payload, err := json.Marshal(claims)
	require.NoError(t, err)
	return "header." + base64.RawURLEncoding.EncodeToString(payload) + ".signature"
}

// clearRunnerEnv neutralizes any ambient GitHub Actions variables so tests are deterministic even
// when the suite itself runs inside GitHub Actions.
func clearRunnerEnv(t *testing.T) {
	t.Helper()
	t.Setenv("GITHUB_ACTIONS", "")
	t.Setenv("ACTIONS_ID_TOKEN", "")
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "")
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", "")
}

func TestDecodeClaims(t *testing.T) {
	jwt := makeJWT(t, map[string]any{
		"repository":  "cloudposse/example",
		"environment": "prod",
		"ref":         "refs/heads/main",
		"sub":         "repo:cloudposse/example:environment:prod",
	})
	claims, err := decodeClaims(jwt)
	require.NoError(t, err)
	assert.Equal(t, "cloudposse/example", claims.Repository)
	assert.Equal(t, "prod", claims.Environment)
	assert.Equal(t, "refs/heads/main", claims.Ref)
	assert.Equal(t, "repo:cloudposse/example:environment:prod", claims.Subject)
}

func TestDecodeClaims_Errors(t *testing.T) {
	t.Run("malformed (not three segments)", func(t *testing.T) {
		_, err := decodeClaims("a.b")
		require.ErrorIs(t, err, ErrTokenDecode)
	})
	t.Run("invalid base64 payload", func(t *testing.T) {
		_, err := decodeClaims("header.!!!notbase64!!!.sig")
		require.ErrorIs(t, err, ErrTokenDecode)
	})
}

func TestValidateRequestURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{name: "valid https", url: "https://token.actions.githubusercontent.com/abc"},
		{name: "http rejected", url: "http://token.actions.githubusercontent.com/abc", wantErr: true},
		{name: "empty host rejected", url: "https:///path", wantErr: true},
		{name: "garbage rejected", url: "://not a url", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRequestURL(tt.url)
			if tt.wantErr {
				require.ErrorIs(t, err, ErrInvalidRequestURL)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestRequestClaims_Unavailable(t *testing.T) {
	t.Run("not a GitHub Actions runner", func(t *testing.T) {
		clearRunnerEnv(t)
		claims, available, err := RequestClaims(context.Background())
		require.NoError(t, err)
		assert.False(t, available)
		assert.Nil(t, claims)
	})

	t.Run("runner without id-token permission", func(t *testing.T) {
		clearRunnerEnv(t)
		t.Setenv("GITHUB_ACTIONS", "true")
		// No ACTIONS_ID_TOKEN and no request token/url → cannot mint a token.
		claims, available, err := RequestClaims(context.Background())
		require.NoError(t, err)
		assert.False(t, available)
		assert.Nil(t, claims)
	})
}

func TestRequestClaims_FromInjectedToken(t *testing.T) {
	clearRunnerEnv(t)
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("ACTIONS_ID_TOKEN", makeJWT(t, map[string]any{
		"repository":  "cloudposse/example",
		"environment": "staging",
	}))

	claims, available, err := RequestClaims(context.Background())
	require.NoError(t, err)
	require.True(t, available)
	require.NotNil(t, claims)
	assert.Equal(t, "cloudposse/example", claims.Repository)
	assert.Equal(t, "staging", claims.Environment)
}

func TestRequestClaims_FromInjectedToken_Malformed(t *testing.T) {
	clearRunnerEnv(t)
	t.Setenv("GITHUB_ACTIONS", "true")
	// A non-three-segment token is available but cannot be decoded.
	t.Setenv("ACTIONS_ID_TOKEN", "not.a.valid.jwt")

	claims, available, err := RequestClaims(context.Background())
	require.ErrorIs(t, err, ErrTokenDecode)
	// The token was obtainable, so available is true even though decoding failed.
	assert.True(t, available)
	assert.Nil(t, claims)
}

func TestFetchToken(t *testing.T) {
	t.Run("success returns jwt and sends expected headers", func(t *testing.T) {
		jwt := makeJWT(t, map[string]any{"repository": "o/r"})
		var gotAuth, gotAccept string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			gotAccept = r.Header.Get("Accept")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"value":"` + jwt + `"}`))
		}))
		defer srv.Close()

		got, err := fetchToken(context.Background(), srv.URL, "secret-token")
		require.NoError(t, err)
		assert.Equal(t, jwt, got)
		// The request must authenticate with a bearer token and accept JSON.
		assert.Equal(t, "bearer secret-token", gotAuth)
		assert.Equal(t, "application/json", gotAccept)
	})

	t.Run("empty token in response wraps ErrTokenRequest", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"value":""}`))
		}))
		defer srv.Close()

		_, err := fetchToken(context.Background(), srv.URL, "tok")
		require.ErrorIs(t, err, ErrTokenRequest)
	})

	t.Run("non-200 status wraps ErrTokenRequest", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		_, err := fetchToken(context.Background(), srv.URL, "tok")
		require.ErrorIs(t, err, ErrTokenRequest)
	})

	t.Run("malformed JSON body wraps ErrTokenRequest", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{not json`))
		}))
		defer srv.Close()

		_, err := fetchToken(context.Background(), srv.URL, "tok")
		require.ErrorIs(t, err, ErrTokenRequest)
	})
}

func TestRequestToken(t *testing.T) {
	t.Run("not a GitHub Actions runner", func(t *testing.T) {
		clearRunnerEnv(t)
		// GITHUB_ACTIONS unset (empty) → not a runner.
		jwt, available, err := requestToken(context.Background())
		require.NoError(t, err)
		assert.False(t, available)
		assert.Empty(t, jwt)
	})

	t.Run("GITHUB_ACTIONS=false", func(t *testing.T) {
		clearRunnerEnv(t)
		t.Setenv("GITHUB_ACTIONS", "false")
		jwt, available, err := requestToken(context.Background())
		require.NoError(t, err)
		assert.False(t, available)
		assert.Empty(t, jwt)
	})

	t.Run("injected token is trimmed", func(t *testing.T) {
		clearRunnerEnv(t)
		t.Setenv("GITHUB_ACTIONS", "true")
		// Whitespace padding must be trimmed off the returned token.
		t.Setenv("ACTIONS_ID_TOKEN", "  padded.jwt.value  ")
		jwt, available, err := requestToken(context.Background())
		require.NoError(t, err)
		assert.True(t, available)
		assert.Equal(t, "padded.jwt.value", jwt)
	})

	t.Run("runner without request token/url", func(t *testing.T) {
		clearRunnerEnv(t)
		t.Setenv("GITHUB_ACTIONS", "true")
		// No ACTIONS_ID_TOKEN and an empty request token → cannot mint.
		t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", "https://token.actions.githubusercontent.com/abc")
		jwt, available, err := requestToken(context.Background())
		require.NoError(t, err)
		assert.False(t, available)
		assert.Empty(t, jwt)
	})

	t.Run("invalid request URL returns ErrInvalidRequestURL", func(t *testing.T) {
		clearRunnerEnv(t)
		t.Setenv("GITHUB_ACTIONS", "true")
		t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "tok")
		// Non-https URL fails SSRF validation; available is true because the job granted id-token.
		t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", "http://x")
		jwt, available, err := requestToken(context.Background())
		require.ErrorIs(t, err, ErrInvalidRequestURL)
		assert.True(t, available)
		assert.Empty(t, jwt)
	})

	t.Run("mints token from valid https request token/url", func(t *testing.T) {
		jwt := makeJWT(t, map[string]any{"repository": "o/r", "environment": "prod"})
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"value":"` + jwt + `"}`))
		}))
		defer srv.Close()

		// fetchToken builds its own client with no Transport, so it uses http.DefaultTransport.
		// Swap in the TLS server's trusting transport so the https minting path can reach it,
		// then restore the original transport when the test finishes.
		orig := http.DefaultTransport
		http.DefaultTransport = srv.Client().Transport
		t.Cleanup(func() { http.DefaultTransport = orig })

		clearRunnerEnv(t)
		t.Setenv("GITHUB_ACTIONS", "true")
		t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "tok")
		// The httptest TLS server serves https, so validateRequestURL accepts it.
		t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", srv.URL)

		got, available, err := requestToken(context.Background())
		require.NoError(t, err)
		assert.True(t, available)
		assert.Equal(t, jwt, got)
	})
}

func TestRequestClaims_EndToEnd(t *testing.T) {
	clearRunnerEnv(t)
	t.Setenv("GITHUB_ACTIONS", "true")
	// A valid 3-part JWT whose payload decodes to the expected claims.
	t.Setenv("ACTIONS_ID_TOKEN", makeJWT(t, map[string]any{
		"repository":  "o/r",
		"environment": "prod",
	}))

	claims, available, err := RequestClaims(context.Background())
	require.NoError(t, err)
	require.True(t, available)
	require.NotNil(t, claims)
	assert.Equal(t, "o/r", claims.Repository)
	assert.Equal(t, "prod", claims.Environment)
}
