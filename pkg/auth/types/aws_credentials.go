package types

import (
	"fmt"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
)

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
		return nil, fmt.Errorf("%w: failed parsing AWS credential expiration: %v", errUtils.ErrInvalidAuthConfig, err)
	}
	return &expTime, nil
}

// BuildWhoamiInfo implements ICredentials for AWSCredentials.
func (c *AWSCredentials) BuildWhoamiInfo(info *WhoamiInfo) {
	info.Region = c.Region
	if t, _ := c.GetExpiration(); t != nil {
		info.Expiration = t
	}
}
