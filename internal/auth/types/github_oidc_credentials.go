package types

import "time"

// OIDCCredentials defines OIDC-specific credential fields.
type OIDCCredentials struct {
	Token    string `json:"token,omitempty"`
	Provider string `json:"provider,omitempty"`
	Audience string `json:"audience,omitempty"`
}

// IsExpired implements ICredentials for OIDCCredentials.
// If no expiration tracking exists, default to not expired.
func (c *OIDCCredentials) IsExpired() bool {
	return false
}

// GetExpiration implements ICredentials for OIDCCredentials.
func (c *OIDCCredentials) GetExpiration() (*time.Time, error) {
	return nil, nil
}

// BuildWhoamiInfo implements ICredentials for OIDCCredentials.
func (c *OIDCCredentials) BuildWhoamiInfo(info *WhoamiInfo) {
	// No additional fields to populate for generic OIDC creds
}
