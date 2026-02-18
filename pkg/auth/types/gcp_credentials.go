package types

import (
	"context"
	"fmt"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
)

// GCPCredentials defines GCP-specific credential fields for OAuth2 access tokens.
type GCPCredentials struct {
	// AccessToken is the OAuth2 access token.
	AccessToken string `json:"access_token,omitempty"`
	// TokenExpiry is when the token expires.
	TokenExpiry time.Time `json:"token_expiry,omitempty"`
	// ProjectID is the GCP project ID.
	ProjectID string `json:"project_id,omitempty"`
	// ServiceAccountEmail is the service account being used (if impersonating).
	ServiceAccountEmail string `json:"service_account_email,omitempty"`
	// Scopes are the OAuth2 scopes.
	Scopes []string `json:"scopes,omitempty"`
}

// IsExpired returns true if the credentials are expired.
// This implements the ICredentials interface.
func (c *GCPCredentials) IsExpired() bool {
	if c.TokenExpiry.IsZero() {
		return false
	}
	return !time.Now().Before(c.TokenExpiry)
}

// GetExpiration implements ICredentials for GCPCredentials.
func (c *GCPCredentials) GetExpiration() (*time.Time, error) {
	if c.TokenExpiry.IsZero() {
		return nil, nil
	}
	localTime := c.TokenExpiry.Local()
	return &localTime, nil
}

// GetExpirationTime returns the token expiry time (zero value if not set).
func (c *GCPCredentials) GetExpirationTime() time.Time {
	return c.TokenExpiry
}

// BuildWhoamiInfo implements ICredentials for GCPCredentials.
func (c *GCPCredentials) BuildWhoamiInfo(info *WhoamiInfo) {
	if info == nil {
		return
	}
	if c.ProjectID != "" {
		info.Account = c.ProjectID
	}
	if c.ServiceAccountEmail != "" {
		info.Principal = c.ServiceAccountEmail
	}
	if t, _ := c.GetExpiration(); t != nil {
		info.Expiration = t
	}
}

// Validate validates GCP credentials. Not implemented in Stage 1.
// Returns ErrNotImplemented; later stages may add actual validation.
func (c *GCPCredentials) Validate(ctx context.Context) (*ValidationInfo, error) {
	return nil, fmt.Errorf("%w: GCP credential validation not yet implemented", errUtils.ErrNotImplemented)
}
