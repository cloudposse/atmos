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
// Sensitive credentials are stored in info.Credentials (non-serializable).
// Only non-sensitive environment variables are placed in info.Environment.
func (c *Credentials) BuildWhoamiInfo(info *types.WhoamiInfo) {
	// Nil-check the incoming info pointer.
	if info == nil {
		return
	}

	// Store raw credentials in non-serializable field.
	info.Credentials = c

	// Only populate non-sensitive environment variables.
	// Credentials like AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, and AWS_SESSION_TOKEN
	// are NOT exposed in Environment to prevent serialization leaks.
	if info.Environment == nil {
		info.Environment = make(map[string]string)
	}
	info.Environment["AWS_REGION"] = c.Region
	info.Environment["AWS_DEFAULT_REGION"] = c.Region

	// Set expiration only if non-zero.
	if !c.Expiration.IsZero() {
		info.Expiration = &c.Expiration
	}

	// Set region at top level.
	info.Region = c.Region
}
