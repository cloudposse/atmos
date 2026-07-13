package aws

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssooidc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockTokenRefresher is a hand-rolled ssoTokenRefresher for exercising the
// refresh paths without a live AWS endpoint. createTokenFunc receives the input
// so assertions can verify the grant type and refresh token sent to the service.
type mockTokenRefresher struct {
	createTokenFunc func(ctx context.Context, in *ssooidc.CreateTokenInput) (*ssooidc.CreateTokenOutput, error)
	gotInput        *ssooidc.CreateTokenInput
}

func (m *mockTokenRefresher) CreateToken(ctx context.Context, in *ssooidc.CreateTokenInput, _ ...func(*ssooidc.Options)) (*ssooidc.CreateTokenOutput, error) {
	m.gotInput = in
	return m.createTokenFunc(ctx, in)
}

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
	require.Error(t, err)
	require.ErrorIs(t, err, errRefreshNotSupported,
		"expired registration must surface as errRefreshNotSupported so callers fall through to device auth")
}

func TestTryRefreshToken_SuccessWithRotatedRefreshToken(t *testing.T) {
	// The happy path: a valid refresh token is exchanged for a new access token,
	// and AWS SSO OIDC rotates the refresh token. The rotated token must be
	// persisted so the next refresh succeeds.
	mock := &mockTokenRefresher{
		createTokenFunc: func(_ context.Context, _ *ssooidc.CreateTokenInput) (*ssooidc.CreateTokenOutput, error) {
			return &ssooidc.CreateTokenOutput{
				AccessToken:  aws.String("new-access-token"),
				RefreshToken: aws.String("rotated-refresh-token"),
				ExpiresIn:    3600,
			}, nil
		},
	}

	cached := ssoTokenCache{
		RefreshToken:          "original-refresh-token",
		ClientID:              "client-id",
		ClientSecret:          "client-secret",
		Region:                "us-east-1",
		StartURL:              "https://example.com",
		RegistrationExpiresAt: time.Now().Add(30 * 24 * time.Hour),
	}

	got, err := tryRefreshToken(context.Background(), mock, cached)
	require.NoError(t, err)

	assert.Equal(t, "new-access-token", got.AccessToken)
	assert.Equal(t, "rotated-refresh-token", got.RefreshToken, "rotated refresh token must be persisted")
	// Portal identity and client registration carry over unchanged.
	assert.Equal(t, "us-east-1", got.Region)
	assert.Equal(t, "https://example.com", got.StartURL)
	assert.Equal(t, "client-id", got.ClientID)
	// Expiry is derived from ExpiresIn (~1h from now); allow slack for execution time.
	assert.WithinDuration(t, time.Now().Add(3600*time.Second), got.ExpiresAt, time.Minute)

	// The request must use the refresh-token grant with the cached refresh token.
	require.NotNil(t, mock.gotInput)
	assert.Equal(t, ssoRefreshGrantType, aws.ToString(mock.gotInput.GrantType))
	assert.Equal(t, "original-refresh-token", aws.ToString(mock.gotInput.RefreshToken))
}

func TestTryRefreshToken_SuccessWithoutRotationKeepsPriorToken(t *testing.T) {
	// AWS docs don't guarantee rotation on every call. When the service omits a
	// new refresh token, the prior one must be retained so refresh keeps working.
	mock := &mockTokenRefresher{
		createTokenFunc: func(_ context.Context, _ *ssooidc.CreateTokenInput) (*ssooidc.CreateTokenOutput, error) {
			return &ssooidc.CreateTokenOutput{
				AccessToken: aws.String("new-access-token"),
				// RefreshToken intentionally empty (no rotation).
				ExpiresIn: 3600,
			}, nil
		},
	}

	got, err := tryRefreshToken(context.Background(), mock, ssoTokenCache{
		RefreshToken:          "original-refresh-token",
		ClientID:              "client-id",
		ClientSecret:          "client-secret",
		RegistrationExpiresAt: time.Now().Add(30 * 24 * time.Hour),
	})
	require.NoError(t, err)
	assert.Equal(t, "original-refresh-token", got.RefreshToken,
		"when the service does not rotate, the prior refresh token must be retained")
}

func TestTryRefreshToken_APIErrorIsWrapped(t *testing.T) {
	// A server-side rejection (e.g., invalid_grant) must surface as an error so
	// the caller falls through to device auth. It must NOT masquerade as
	// errRefreshNotSupported, which is reserved for "can't even attempt refresh".
	sentinel := errors.New("invalid_grant")
	mock := &mockTokenRefresher{
		createTokenFunc: func(_ context.Context, _ *ssooidc.CreateTokenInput) (*ssooidc.CreateTokenOutput, error) {
			return nil, sentinel
		},
	}

	_, err := tryRefreshToken(context.Background(), mock, ssoTokenCache{
		RefreshToken:          "have-refresh",
		ClientID:              "client-id",
		ClientSecret:          "client-secret",
		RegistrationExpiresAt: time.Now().Add(30 * 24 * time.Hour),
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel, "underlying API error must be wrapped, not swallowed")
	assert.NotErrorIs(t, err, errRefreshNotSupported)
}
