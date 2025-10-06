package types

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
)

// OIDCCredentials defines OIDC-specific credential fields.
type OIDCCredentials struct {
	Token    string `json:"token,omitempty"`
	Provider string `json:"provider,omitempty"`
	Audience string `json:"audience,omitempty"`
}

// IsExpired implements ICredentials for OIDCCredentials.
// If no expiration tracking exists, default to not expired.
func (c *OIDCCredentials) IsExpired() bool {
	exp, err := c.GetExpiration()
	if err != nil || exp == nil {
		return false
	}
	// 5m skew to avoid edge expirations.
	return time.Now().After(exp.Add(-5 * time.Minute))
}

// GetExpiration implements ICredentials for OIDCCredentials.
func (c *OIDCCredentials) GetExpiration() (*time.Time, error) {
	// Expect c.Token to be a JWT. Decode payload and extract "exp" (seconds since epoch).
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
	t := time.Unix(claims.Exp, 0).UTC()
	return &t, nil
}

// BuildWhoamiInfo implements ICredentials for OIDCCredentials.
func (c *OIDCCredentials) BuildWhoamiInfo(info *WhoamiInfo) {
	if info == nil {
		return
	}
	// Typically, this is not used for OIDC credentials. As we just fetched the credentials from GitHub, they won't expire.
	if exp, _ := c.GetExpiration(); exp != nil {
		info.Expiration = exp
	}
}
