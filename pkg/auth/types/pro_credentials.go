package types

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
)

// ProCredentials holds an Atmos Pro session JWT (a `ws:gh:action` token) obtained by
// federating a GitHub Actions OIDC token through the Atmos Pro auth endpoint.
//
// It carries the BaseURL and WorkspaceID resolved by the atmos/pro provider so that
// downstream integrations (e.g., github/sts) can call Atmos Pro APIs without needing
// access to the global Atmos configuration.
type ProCredentials struct {
	Token       string `json:"token,omitempty"`
	BaseURL     string `json:"base_url,omitempty"`
	Endpoint    string `json:"endpoint,omitempty"`
	WorkspaceID string `json:"workspace_id,omitempty"`
	Provider    string `json:"provider,omitempty"`
}

// IsExpired implements ICredentials for ProCredentials.
// If no expiration can be derived from the JWT, default to not expired.
func (c *ProCredentials) IsExpired() bool {
	exp, err := c.GetExpiration()
	if err != nil || exp == nil {
		return false
	}
	// 5m skew to avoid edge expirations.
	return time.Now().After(exp.Add(-5 * time.Minute))
}

// GetExpiration implements ICredentials for ProCredentials by decoding the JWT `exp` claim.
func (c *ProCredentials) GetExpiration() (*time.Time, error) {
	parts := strings.Split(c.Token, ".")
	if len(parts) < 2 {
		return nil, nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errors.Join(errUtils.ErrAuthOidcDecodeFailed, err)
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, errors.Join(errUtils.ErrAuthOidcUnmarshalFailed, err)
	}
	if claims.Exp == 0 {
		return nil, nil
	}
	// Convert to local timezone for display to user.
	t := time.Unix(claims.Exp, 0).Local()
	return &t, nil
}

// BuildWhoamiInfo implements ICredentials for ProCredentials.
func (c *ProCredentials) BuildWhoamiInfo(info *WhoamiInfo) {
	if info == nil {
		return
	}
	if c.WorkspaceID != "" {
		info.Account = c.WorkspaceID
	}
	if exp, _ := c.GetExpiration(); exp != nil {
		info.Expiration = exp
	}
}

// Validate is not implemented for Atmos Pro session credentials.
func (c *ProCredentials) Validate(_ context.Context) (*ValidationInfo, error) {
	return nil, errUtils.ErrNotImplemented
}
