package types

import "time"

// AWSCredentials defines AWS-specific credential fields.
type AWSCredentials struct {
	AccessKeyID     string `json:"access_key_id,omitempty"`
	SecretAccessKey string `json:"secret_access_key,omitempty"`
	SessionToken    string `json:"session_token,omitempty"`
	Region          string `json:"region,omitempty"`
	Expiration      string `json:"expiration,omitempty"`
	MfaArn          string `json:"mfa_arn,omitempty"`
}

// IsExpired returns true if the credentials are expired.
// This implements the ICredentials interface.
func (c *AWSCredentials) IsExpired() bool {
	if c.Expiration == "" {
		return false
	}
	expTime, err := time.Parse(time.RFC3339, c.Expiration)
	if err != nil {
		return true
	}
	return time.Now().After(expTime)
}

// GetExpiration implements ICredentials for AWSCredentials.
func (c *AWSCredentials) GetExpiration() (*time.Time, error) {
	if c.Expiration == "" {
		return nil, nil
	}
	expTime, err := time.Parse(time.RFC3339, c.Expiration)
	if err != nil {
		return nil, err
	}
	return &expTime, nil
}

// BuildWhoamiInfo implements ICredentials for AWSCredentials.
func (c *AWSCredentials) BuildWhoamiInfo(info *WhoamiInfo) {
	info.Region = c.Region
}

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

// WhoamiInfo represents the current effective authentication principal.
type WhoamiInfo struct {
	Provider    string            `json:"provider"`
	Identity    string            `json:"identity"`
	Principal   string            `json:"principal"`
	Account     string            `json:"account,omitempty"`
	Region      string            `json:"region,omitempty"`
	Expiration  *time.Time        `json:"expiration,omitempty"`
	Environment map[string]string `json:"environment,omitempty"`
	Credentials ICredentials      `json:"credentials,omitempty"`
	LastUpdated time.Time         `json:"last_updated"`
}
