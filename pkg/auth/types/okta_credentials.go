package types

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
)

// OktaCredentials defines Okta-specific credential fields.
type OktaCredentials struct {
	// OrgURL is the Okta organization URL (e.g., https://company.okta.com).
	OrgURL string `json:"org_url"`

	// AccessToken is the OAuth 2.0 access token for API access.
	AccessToken string `json:"access_token"`

	// IDToken is the OpenID Connect ID token containing user claims.
	// This is used for AWS OIDC federation via AssumeRoleWithWebIdentity.
	IDToken string `json:"id_token,omitempty"`

	// RefreshToken is used to obtain new access tokens (if offline_access scope was requested).
	RefreshToken string `json:"refresh_token,omitempty"`

	// ExpiresAt is when the access token expires.
	ExpiresAt time.Time `json:"expires_at"`

	// RefreshTokenExpiresAt is when the refresh token expires.
	RefreshTokenExpiresAt time.Time `json:"refresh_token_expires_at,omitempty"`

	// Scope contains the scopes granted by Okta (space-separated).
	Scope string `json:"scope,omitempty"`
}

// IsExpired returns true if the credentials are expired.
// This implements the ICredentials interface.
// Includes a 5-minute buffer before actual expiration.
func (c *OktaCredentials) IsExpired() bool {
	if c.ExpiresAt.IsZero() {
		return true
	}
	// Consider expired 5 minutes before actual expiration to avoid edge cases.
	return time.Now().Add(5 * time.Minute).After(c.ExpiresAt)
}

// GetExpiration implements ICredentials for OktaCredentials.
func (c *OktaCredentials) GetExpiration() (*time.Time, error) {
	if c.ExpiresAt.IsZero() {
		return nil, nil
	}
	// Convert to local timezone for display to user.
	localTime := c.ExpiresAt.Local()
	return &localTime, nil
}

// BuildWhoamiInfo implements ICredentials for OktaCredentials.
func (c *OktaCredentials) BuildWhoamiInfo(info *WhoamiInfo) {
	// Extract user info from ID token if available.
	if c.IDToken != "" {
		if claims, err := extractOktaIDTokenClaims(c.IDToken); err == nil {
			// Set principal from email or subject.
			if email, ok := claims["email"].(string); ok && email != "" {
				info.Principal = email
			} else if sub, ok := claims["sub"].(string); ok {
				info.Principal = sub
			}

			// Set account from Okta issuer (organization URL).
			if iss, ok := claims["iss"].(string); ok {
				info.Account = iss
			}
		}
	}

	// Fallback: use org URL as account if not set from claims.
	if info.Account == "" && c.OrgURL != "" {
		info.Account = c.OrgURL
	}

	if t, _ := c.GetExpiration(); t != nil {
		info.Expiration = t
	}
}

// Validate validates Okta credentials by parsing the ID token (if available)
// or checking token expiration.
// Note: Full API validation would require Okta SDK dependencies.
func (c *OktaCredentials) Validate(ctx context.Context) (*ValidationInfo, error) {
	// Check basic token presence.
	if c.AccessToken == "" {
		return nil, fmt.Errorf("%w: access token is empty", errUtils.ErrInvalidCredentials)
	}

	// Check if credentials are expired.
	if c.IsExpired() {
		return nil, fmt.Errorf("%w: Okta credentials are expired", errUtils.ErrOktaTokenExpired)
	}

	info := &ValidationInfo{}

	// Extract information from ID token if available.
	if c.IDToken != "" {
		claims, err := extractOktaIDTokenClaims(c.IDToken)
		if err != nil {
			return nil, fmt.Errorf("%w: failed to parse ID token: %w", errUtils.ErrInvalidCredentials, err)
		}

		// Set principal from email or subject.
		if email, ok := claims["email"].(string); ok && email != "" {
			info.Principal = email
		} else if sub, ok := claims["sub"].(string); ok {
			info.Principal = sub
		}

		// Set account from issuer (Okta org URL).
		if iss, ok := claims["iss"].(string); ok {
			info.Account = iss
		}

		// Get expiration from token if available.
		if exp, ok := claims["exp"].(float64); ok {
			expTime := time.Unix(int64(exp), 0)
			info.Expiration = &expTime
		}
	} else {
		// Use credential-level info as fallback.
		info.Account = c.OrgURL
		if !c.ExpiresAt.IsZero() {
			info.Expiration = &c.ExpiresAt
		}
	}

	return info, nil
}

// CanRefresh returns true if a refresh token is available and not expired.
func (c *OktaCredentials) CanRefresh() bool {
	if c.RefreshToken == "" {
		return false
	}
	// If no expiration set, assume refresh token is valid.
	if c.RefreshTokenExpiresAt.IsZero() {
		return true
	}
	// Include 1-minute buffer for refresh token expiration.
	return time.Now().Add(1 * time.Minute).Before(c.RefreshTokenExpiresAt)
}

// extractOktaIDTokenClaims decodes an Okta ID token (JWT) and returns the claims.
// This is a simplified JWT parser - does NOT validate the signature.
func extractOktaIDTokenClaims(token string) (map[string]interface{}, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWT format: expected 3 parts, got %d", len(parts))
	}

	// Decode payload (second part).
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("failed to decode JWT payload: %w", err)
	}

	var claims map[string]interface{}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("failed to parse JWT claims: %w", err)
	}

	return claims, nil
}
