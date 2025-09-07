package types

import "time"

// Credentials defines credential storage configuration.
type Credentials struct {
	AWS  *AWSCredentials  `yaml:"aws,omitempty" json:"aws,omitempty" mapstructure:"aws"`
	OIDC *OIDCCredentials `yaml:"oidc,omitempty" json:"oidc,omitempty" mapstructure:"oidc"`
}

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

// OIDCCredentials defines OIDC-specific credential fields.
type OIDCCredentials struct {
	Token    string `json:"token,omitempty"`
	Provider string `json:"provider,omitempty"`
	Audience string `json:"audience,omitempty"`
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
	Credentials *Credentials      `json:"credentials,omitempty"`
	LastUpdated time.Time         `json:"last_updated"`
}
