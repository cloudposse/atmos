package aws

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTryRefreshToken_NoRefreshToken(t *testing.T) {
	// Tokens written by atmos versions before refresh-token support have no
	// RefreshToken field. The refresh path must short-circuit so callers fall
	// through to the device-authorization flow instead of making a doomed API call.
	_, err := tryRefreshToken(context.Background(), nil, ssoTokenCache{
		AccessToken: "old-token",
		Region:      "us-east-1",
		StartURL:    "https://example.com",
		ExpiresAt:   time.Now().Add(-1 * time.Hour),
	})
	assert.ErrorIs(t, err, errRefreshNotSupported)
}

func TestTryRefreshToken_MissingClientCredentials(t *testing.T) {
	// A refresh token without client_id/client_secret is unusable. AWS SSO OIDC
	// requires both for the refresh exchange.
	_, err := tryRefreshToken(context.Background(), nil, ssoTokenCache{
		RefreshToken: "have-refresh",
		// ClientID/ClientSecret deliberately omitted.
	})
	assert.ErrorIs(t, err, errRefreshNotSupported)
}

func TestTryRefreshToken_ExpiredRegistration(t *testing.T) {
	// SSO OIDC client registrations expire (typically ~90 days). Once expired,
	// the refresh exchange will fail server-side; we detect this client-side to
	// fall through to re-registration via the device-auth path immediately.
	_, err := tryRefreshToken(context.Background(), nil, ssoTokenCache{
		RefreshToken:          "have-refresh",
		ClientID:              "client-id",
		ClientSecret:          "client-secret",
		RegistrationExpiresAt: time.Now().Add(-1 * time.Hour),
	})
	require := assert.New(t)
	require.Error(err)
	require.True(errors.Is(err, errRefreshNotSupported),
		"expired registration must surface as errRefreshNotSupported so callers fall through to device auth")
}

func TestTryRefreshToken_FreshRegistrationProceeds(t *testing.T) {
	// With all fields present and a non-expired registration, the function must
	// proceed to the actual API call (and fail because nil client). We're not
	// testing the API call itself here — that requires a mock OIDC client which
	// is deferred to integration tests. We're verifying we don't bail early.
	defer func() {
		// nil pointer deref from the client.CreateToken call is fine for this
		// gate test — the point is to confirm we got *past* the early returns.
		_ = recover()
	}()
	_, err := tryRefreshToken(context.Background(), nil, ssoTokenCache{
		RefreshToken:          "have-refresh",
		ClientID:              "client-id",
		ClientSecret:          "client-secret",
		RegistrationExpiresAt: time.Now().Add(30 * 24 * time.Hour),
	})
	// If we got here without a panic, we exited via the CreateToken error path.
	// Either error path is acceptable; we just must NOT see errRefreshNotSupported.
	if err != nil {
		assert.NotErrorIs(t, err, errRefreshNotSupported,
			"valid inputs should reach the API call, not return errRefreshNotSupported")
	}
}
