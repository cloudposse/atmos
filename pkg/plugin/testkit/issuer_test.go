package testkit

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/keyring"
)

func TestIssuerRejectsInvalidProtocolRequests(t *testing.T) {
	t.Parallel()
	issuer, err := NewIssuer()
	require.NoError(t, err)
	t.Cleanup(issuer.Close)
	for _, tc := range []struct {
		name, path, body string
		status           int
	}{
		{"missing PKCE and nonce", "/authorize", "", http.StatusBadRequest},
		{"unknown route", "/unknown", "", http.StatusNotFound},
		{"malformed token form", "/token", "%invalid", http.StatusBadRequest},
		{"malformed revocation form", "/revoke", "%invalid", http.StatusBadRequest},
		{"unknown grant", "/token", "grant_type=unknown", http.StatusBadRequest},
		{"unknown authorization code", "/token", "grant_type=authorization_code&code=missing", http.StatusBadRequest},
		{"unknown refresh token", "/token", "grant_type=refresh_token&refresh_token=missing", http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			request := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			response := httptest.NewRecorder()
			issuer.serve(response, request)
			require.Equal(t, tc.status, response.Code)
			require.Empty(t, response.Header().Get("Location"))
			exchanges, refreshes, revocations := issuer.Counts()
			require.Zero(t, exchanges)
			require.Zero(t, refreshes)
			require.Zero(t, revocations)
		})
	}
}

func TestIssuerRejectsRefreshFromAnotherClientWithoutConsumingToken(t *testing.T) {
	t.Parallel()
	issuer, err := NewIssuer()
	require.NoError(t, err)
	t.Cleanup(issuer.Close)
	issuer.refresh["original"] = authorization{client: "owner", nonce: "nonce"}
	for _, client := range []string{"other", "owner"} {
		form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {"original"}, "client_id": {client}}
		request := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader(form.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		response := httptest.NewRecorder()
		issuer.serve(response, request)
		if client == "other" {
			require.Equal(t, http.StatusBadRequest, response.Code)
			require.Contains(t, issuer.refresh, "original")
			continue
		}
		require.Equal(t, http.StatusOK, response.Code)
		require.NotContains(t, issuer.refresh, "original")
		var tokens map[string]any
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &tokens))
		require.NotEmpty(t, tokens["access_token"])
		require.NotEmpty(t, tokens["refresh_token"])
		require.NotEmpty(t, tokens["id_token"])
	}
	_, refreshes, _ := issuer.Counts()
	require.Equal(t, 1, refreshes)
}

func TestIssuerReportsInvalidClaimMutationWithoutIssuingTokens(t *testing.T) {
	t.Parallel()
	issuer, err := NewIssuer(WithClaims(func(claims map[string]any) { claims["invalid"] = make(chan int) }))
	require.NoError(t, err)
	t.Cleanup(issuer.Close)
	response := httptest.NewRecorder()
	issuer.issueTokens(response, "client", "nonce")
	require.Equal(t, http.StatusInternalServerError, response.Code)
	require.Contains(t, response.Body.String(), "signing failed")
	require.NotContains(t, response.Body.String(), "access_token")
}

func TestExpireSessionRejectsMalformedStorageAndPropagatesReadErrors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, raw string }{
		{"base64", "not-base64"},
		{"envelope JSON", base64.StdEncoding.EncodeToString([]byte("not json"))},
		{"session JSON", sessionEnvelope(t, []byte("not json"))},
		{"missing token", sessionEnvelope(t, []byte(`{"other":"value"}`))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			storage := keyring.NewMockKeyring(gomock.NewController(t))
			storage.EXPECT().List().Return([]string{"atmos_plugin_fixture"}, nil)
			storage.EXPECT().Get("atmos_plugin_fixture").Return(tc.raw, nil)
			require.Error(t, ExpireOIDCSession(storage), "corruption must not be silently overwritten")
		})
	}
	failure := errors.New("storage unavailable")
	for _, operation := range []string{"list", "get"} {
		t.Run(operation, func(t *testing.T) {
			t.Parallel()
			storage := keyring.NewMockKeyring(gomock.NewController(t))
			if operation == "list" {
				storage.EXPECT().List().Return(nil, failure)
			} else {
				storage.EXPECT().List().Return([]string{"atmos_plugin_fixture"}, nil)
				storage.EXPECT().Get("atmos_plugin_fixture").Return("", failure)
			}
			require.ErrorIs(t, ExpireOIDCSession(storage), failure)
		})
	}
}

func TestExpireSessionPreservesUnrelatedStorage(t *testing.T) {
	t.Parallel()
	storage, err := keyring.New(keyring.Config{Type: keyring.TypeMemory})
	require.NoError(t, err)
	require.NoError(t, storage.Set("ordinary", "keep unchanged"))
	withoutSession := base64.StdEncoding.EncodeToString([]byte(`{"version":1,"values":{"another":"dmFsdWU="}}`))
	require.NoError(t, storage.Set("atmos_plugin_unrelated", withoutSession))
	require.ErrorIs(t, ExpireOIDCSession(storage), errUtils.ErrTestsFailed)
	ordinary, err := storage.Get("ordinary")
	require.NoError(t, err)
	require.Equal(t, "keep unchanged", ordinary)
	unrelated, err := storage.Get("atmos_plugin_unrelated")
	require.NoError(t, err)
	require.Equal(t, withoutSession, unrelated)
}

func sessionEnvelope(t *testing.T, session []byte) string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"version": 1, "values": map[string][]byte{"session/v1": session}})
	require.NoError(t, err)
	return base64.StdEncoding.EncodeToString(raw)
}
