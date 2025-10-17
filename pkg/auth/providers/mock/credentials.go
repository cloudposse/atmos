package mock

import (
	"time"

	"github.com/cloudposse/atmos/pkg/auth/types"
)

// Credentials represents mock AWS-like credentials for testing.
type Credentials struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	Region          string
	Expiration      time.Time
}

// IsExpired checks if the credentials are expired.
func (c *Credentials) IsExpired() bool {
	return time.Now().After(c.Expiration)
}

// GetExpiration returns the expiration time of the credentials.
func (c *Credentials) GetExpiration() (*time.Time, error) {
	return &c.Expiration, nil
}

// BuildWhoamiInfo populates WhoamiInfo with mock credential information.
func (c *Credentials) BuildWhoamiInfo(info *types.WhoamiInfo) {
	info.Environment = map[string]string{
		"AWS_ACCESS_KEY_ID":     c.AccessKeyID,
		"AWS_SECRET_ACCESS_KEY": c.SecretAccessKey,
		"AWS_SESSION_TOKEN":     c.SessionToken,
		"AWS_REGION":            c.Region,
		"AWS_DEFAULT_REGION":    c.Region,
	}
	info.Expiration = &c.Expiration
}
