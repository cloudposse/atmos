package types

import (
	"context"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
)

// PluginCredentials persists a versioned credential envelope without exposing
// its private payload through whoami. Refresh tokens belong in private session
// storage and must never enter this downstream envelope.
type PluginCredentials struct {
	Type          string         `json:"type"`
	ExpiresAt     time.Time      `json:"expires_at"`
	Data          map[string]any `json:"data"`
	SensitiveKeys []string       `json:"sensitive_keys,omitempty"`
}

// IsExpired treats a missing expiration as unusable, rather than infinite validity.
func (c *PluginCredentials) IsExpired() bool {
	return c.ExpiresAt.IsZero() || !time.Now().Before(c.ExpiresAt)
}

// GetExpiration returns the validated envelope expiration.
func (c *PluginCredentials) GetExpiration() (*time.Time, error) {
	if c.ExpiresAt.IsZero() {
		return nil, errUtils.ErrInvalidAuthConfig
	}
	value := c.ExpiresAt
	return &value, nil
}

// BuildWhoamiInfo exposes only identity metadata, never arbitrary plugin data.
func (c *PluginCredentials) BuildWhoamiInfo(info *WhoamiInfo) {
	if info == nil {
		return
	}
	if subject, ok := c.Data["subject"].(string); ok {
		info.Principal = subject
	}
	info.Expiration, _ = c.GetExpiration()
}

// Validate requires provider-specific validation rather than accepting opaque data.
func (*PluginCredentials) Validate(context.Context) (*ValidationInfo, error) {
	return nil, errUtils.ErrNotImplemented
}

// String prevents accidental formatting of credential material in diagnostics.
func (c *PluginCredentials) String() string { return "[plugin credentials redacted]" }
