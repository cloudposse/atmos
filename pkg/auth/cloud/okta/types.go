package okta

import (
	"time"
)

// OktaTokens holds OAuth tokens returned by Okta.
// This structure is used for file-based caching of tokens.
type OktaTokens struct {
	// AccessToken is the OAuth 2.0 access token for API access.
	AccessToken string `json:"access_token"`

	// TokenType is typically "Bearer".
	TokenType string `json:"token_type"`

	// ExpiresIn is the token lifetime in seconds (from Okta response).
	ExpiresIn int `json:"expires_in"`

	// ExpiresAt is the calculated expiration time.
	ExpiresAt time.Time `json:"expires_at"`

	// RefreshToken is used to obtain new access tokens (if offline_access scope was requested).
	RefreshToken string `json:"refresh_token,omitempty"`

	// RefreshTokenExpiresAt is when the refresh token expires.
	RefreshTokenExpiresAt time.Time `json:"refresh_token_expires_at,omitempty"`

	// IDToken is the OpenID Connect ID token containing user claims.
	// This is used for AWS OIDC federation via AssumeRoleWithWebIdentity.
	IDToken string `json:"id_token,omitempty"`

	// Scope contains the scopes granted by Okta (space-separated).
	Scope string `json:"scope,omitempty"`
}

// IsExpired returns true if the access token is expired.
// Includes a 5-minute buffer before actual expiration.
func (t *OktaTokens) IsExpired() bool {
	if t.ExpiresAt.IsZero() {
		return true
	}
	// Consider expired 5 minutes before actual expiration to avoid edge cases.
	return time.Now().Add(5 * time.Minute).After(t.ExpiresAt)
}

// CanRefresh returns true if a refresh token is available and not expired.
func (t *OktaTokens) CanRefresh() bool {
	if t.RefreshToken == "" {
		return false
	}
	// If no expiration set, assume refresh token is valid.
	if t.RefreshTokenExpiresAt.IsZero() {
		return true
	}
	// Include 1-minute buffer for refresh token expiration.
	return time.Now().Add(1 * time.Minute).Before(t.RefreshTokenExpiresAt)
}

// DeviceAuthorizationResponse holds the response from Okta's device authorization endpoint.
type DeviceAuthorizationResponse struct {
	// DeviceCode is the code used to poll for tokens.
	DeviceCode string `json:"device_code"`

	// UserCode is the code the user enters in the browser.
	UserCode string `json:"user_code"`

	// VerificationURI is the URL the user visits to authenticate.
	VerificationURI string `json:"verification_uri"`

	// VerificationURIComplete is the URL with user_code pre-filled (if supported).
	VerificationURIComplete string `json:"verification_uri_complete,omitempty"`

	// ExpiresIn is how long the device code is valid (seconds).
	ExpiresIn int `json:"expires_in"`

	// Interval is the minimum polling interval in seconds.
	Interval int `json:"interval"`
}

// TokenResponse holds the response from Okta's token endpoint.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// TokenErrorResponse holds error responses from Okta's token endpoint.
type TokenErrorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description,omitempty"`
}

// ToOktaTokens converts a TokenResponse to OktaTokens with calculated expiration.
func (r *TokenResponse) ToOktaTokens() *OktaTokens {
	expiresAt := time.Now().Add(time.Duration(r.ExpiresIn) * time.Second)

	tokens := &OktaTokens{
		AccessToken:  r.AccessToken,
		TokenType:    r.TokenType,
		ExpiresIn:    r.ExpiresIn,
		ExpiresAt:    expiresAt,
		RefreshToken: r.RefreshToken,
		IDToken:      r.IDToken,
		Scope:        r.Scope,
	}

	// Refresh tokens typically have longer lifetimes.
	// Okta default is 7 days, but we set a conservative estimate.
	if r.RefreshToken != "" {
		tokens.RefreshTokenExpiresAt = time.Now().Add(7 * 24 * time.Hour)
	}

	return tokens
}
