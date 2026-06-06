package oidc

import (
	"context"
	"encoding/base64"
	"encoding/json"
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
