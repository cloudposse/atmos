package azure

import (
	"encoding/base64"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	httpClient "github.com/cloudposse/atmos/pkg/http"
)

// base64URLEncode base64url-encodes (no padding) a string, matching JWT segment encoding.
func base64URLEncode(s string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(s))
}

// testJWTWithExp builds a minimally valid JWT string (header.payload.signature)
// whose payload contains only the given "exp" claim, for exercising
// refreshTokenExpiry without a real signing key.
func testJWTWithExp(exp int64) string {
	header := base64URLEncode(`{"alg":"none"}`)
	payload := base64URLEncode(`{"exp":` + strconv.FormatInt(exp, 10) + `}`)
	return header + "." + payload + ".sig"
}

func TestGetAuthorizationToken_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := httpClient.NewMockClient(ctrl)

	exp := time.Now().Add(1 * time.Hour).Unix()
	refreshToken := testJWTWithExp(exp)

	client.EXPECT().Do(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, "https://myregistry.azurecr.io/oauth2/exchange", req.URL.String())
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		bodyStr := string(body)
		assert.Contains(t, bodyStr, "grant_type=access_token")
		assert.Contains(t, bodyStr, "service=myregistry.azurecr.io")
		assert.Contains(t, bodyStr, "tenant=tenant-123")

		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"refresh_token":"` + refreshToken + `"}`)),
		}, nil
	})

	origClient := acrHTTPClient
	acrHTTPClient = client
	t.Cleanup(func() { acrHTTPClient = origClient })

	creds := &types.AzureCredentials{AccessToken: "aad-token", TenantID: "tenant-123"}
	result, err := GetAuthorizationToken(t.Context(), creds, "myregistry.azurecr.io")
	require.NoError(t, err)
	assert.Equal(t, acrLoginUsername, result.Username)
	assert.Equal(t, refreshToken, result.Password)
	assert.Equal(t, "myregistry.azurecr.io", result.Registry)
	assert.WithinDuration(t, time.Unix(exp, 0), result.ExpiresAt, time.Second)
}

func TestGetAuthorizationToken_WrongCredentialType(t *testing.T) {
	_, err := GetAuthorizationToken(t.Context(), &types.AWSCredentials{}, "myregistry.azurecr.io")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrACRAuthFailed)
}

func TestGetAuthorizationToken_NonOKStatus(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := httpClient.NewMockClient(ctrl)
	client.EXPECT().Do(gomock.Any()).Return(&http.Response{
		StatusCode: http.StatusUnauthorized,
		Body:       io.NopCloser(strings.NewReader(`{"error":"invalid"}`)),
	}, nil)

	origClient := acrHTTPClient
	acrHTTPClient = client
	t.Cleanup(func() { acrHTTPClient = origClient })

	creds := &types.AzureCredentials{AccessToken: "aad-token", TenantID: "tenant-123"}
	_, err := GetAuthorizationToken(t.Context(), creds, "myregistry.azurecr.io")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrACRAuthFailed)
}

func TestGetAuthorizationToken_EmptyRefreshToken(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := httpClient.NewMockClient(ctrl)
	client.EXPECT().Do(gomock.Any()).Return(&http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"refresh_token":""}`)),
	}, nil)

	origClient := acrHTTPClient
	acrHTTPClient = client
	t.Cleanup(func() { acrHTTPClient = origClient })

	creds := &types.AzureCredentials{AccessToken: "aad-token", TenantID: "tenant-123"}
	_, err := GetAuthorizationToken(t.Context(), creds, "myregistry.azurecr.io")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrACRAuthFailed)
}

func TestBuildRegistryURL(t *testing.T) {
	assert.Equal(t, "myregistry.azurecr.io", BuildRegistryURL("myregistry"))
}

func TestParseRegistryURL_Success(t *testing.T) {
	name, err := ParseRegistryURL("myregistry.azurecr.io")
	require.NoError(t, err)
	assert.Equal(t, "myregistry", name)
}

func TestParseRegistryURL_WithHTTPSPrefix(t *testing.T) {
	name, err := ParseRegistryURL("https://myregistry.azurecr.io")
	require.NoError(t, err)
	assert.Equal(t, "myregistry", name)
}

func TestParseRegistryURL_Invalid(t *testing.T) {
	_, err := ParseRegistryURL("not-a-registry.example.com")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrACRInvalidRegistry)
}

func TestIsACRRegistry(t *testing.T) {
	assert.True(t, IsACRRegistry("myregistry.azurecr.io"))
	assert.True(t, IsACRRegistry("https://myregistry.azurecr.io"))
	assert.False(t, IsACRRegistry("myregistry.example.com"))
}

func TestRefreshTokenExpiry_InvalidToken(t *testing.T) {
	assert.True(t, refreshTokenExpiry("not-a-jwt").IsZero())
}

func TestRefreshTokenExpiry_NoExpClaim(t *testing.T) {
	token := base64URLEncode(`{"alg":"none"}`) + "." + base64URLEncode(`{}`) + ".sig"
	assert.True(t, refreshTokenExpiry(token).IsZero())
}
