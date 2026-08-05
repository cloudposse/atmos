package aws

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssooidc"

	log "github.com/cloudposse/atmos/pkg/logger"
)

// ssoRefreshGrantType is the OAuth 2.0 grant type for refresh-token exchange.
// Defined by RFC 6749 §6 and supported by the AWS SSO OIDC service when the client
// was registered with the "refresh_token" grant type.
const ssoRefreshGrantType = "refresh_token"

// ssoTokenRefresher is the minimal slice of *ssooidc.Client that tryRefreshToken
// needs. Depending on this interface (rather than the concrete client) lets tests
// inject a mock and exercise the token-rotation, success, and API-error paths
// without a live AWS endpoint. The real *ssooidc.Client satisfies it.
type ssoTokenRefresher interface {
	CreateToken(ctx context.Context, params *ssooidc.CreateTokenInput, optFns ...func(*ssooidc.Options)) (*ssooidc.CreateTokenOutput, error)
}

// errRefreshNotSupported indicates the cached token predates refresh-token support
// (e.g., written by an older atmos version) and cannot be refreshed. Callers should
// fall through to the device-authorization flow.
var errRefreshNotSupported = errors.New("cached token has no refresh token; full device-auth required")

// tryRefreshToken attempts to exchange a cached refresh token for a new access
// token, avoiding the browser interaction entirely. Returns the new token bundle
// on success, or an error on any failure — callers should treat any error as a
// signal to fall back to device-authorization.
//
// This implements the silent-renewal path described in
// https://docs.aws.amazon.com/cli/latest/userguide/sso-configure-profile-token.html:
//
//	"When you sign in, the SSO token configuration uses the refresh token to obtain
//	 a new SSO token whenever the access token expires, until the maximum session
//	 duration has been reached."
func tryRefreshToken(ctx context.Context, client ssoTokenRefresher, cached ssoTokenCache) (ssoTokenCache, error) {
	if cached.RefreshToken == "" || cached.ClientID == "" || cached.ClientSecret == "" {
		return ssoTokenCache{}, errRefreshNotSupported
	}

	// If the client registration itself has expired, the SSO OIDC service will reject
	// the refresh call. Fail fast so the caller falls through to re-registration via
	// the device-auth path.
	if !cached.RegistrationExpiresAt.IsZero() && time.Now().After(cached.RegistrationExpiresAt) {
		return ssoTokenCache{}, fmt.Errorf("oidc client registration expired at %s: %w",
			cached.RegistrationExpiresAt.Format(time.RFC3339), errRefreshNotSupported)
	}

	log.Debug("Attempting SSO token refresh", "startUrl", cached.StartURL, "region", cached.Region)

	resp, err := client.CreateToken(ctx, &ssooidc.CreateTokenInput{
		ClientId:     aws.String(cached.ClientID),
		ClientSecret: aws.String(cached.ClientSecret),
		GrantType:    aws.String(ssoRefreshGrantType),
		RefreshToken: aws.String(cached.RefreshToken),
	})
	if err != nil {
		return ssoTokenCache{}, fmt.Errorf("refresh-token exchange failed: %w", err)
	}

	newToken := ssoTokenCache{
		AccessToken:           aws.ToString(resp.AccessToken),
		ExpiresAt:             time.Now().Add(time.Duration(resp.ExpiresIn) * time.Second),
		Region:                cached.Region,
		StartURL:              cached.StartURL,
		ClientID:              cached.ClientID,
		ClientSecret:          cached.ClientSecret,
		RegistrationExpiresAt: cached.RegistrationExpiresAt,
	}

	// AWS SSO OIDC returns a new (rotated) refresh token on each successful exchange.
	// Persist it so the next refresh can succeed; fall back to the prior one if the
	// service didn't rotate.
	if rotated := aws.ToString(resp.RefreshToken); rotated != "" {
		newToken.RefreshToken = rotated
	} else {
		newToken.RefreshToken = cached.RefreshToken
	}

	log.Debug("SSO token refresh successful", "expiresAt", newToken.ExpiresAt)
	return newToken, nil
}
