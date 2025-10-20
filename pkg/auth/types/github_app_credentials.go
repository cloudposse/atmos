package types

import (
	"time"
)

// GitHubAppCredentials represents GitHub App installation token credentials.
type GitHubAppCredentials struct {
	Token          string    `json:"token,omitempty"`
	AppID          string    `json:"app_id,omitempty"`
	InstallationID string    `json:"installation_id,omitempty"`
	Provider       string    `json:"provider,omitempty"`
	Expiration     time.Time `json:"expiration,omitempty"`
}

// IsExpired checks if the GitHub App token has expired.
// Uses 5 minute skew for safety.
func (c *GitHubAppCredentials) IsExpired() bool {
	if c.Expiration.IsZero() {
		return true
	}
	return time.Now().After(c.Expiration.Add(-5 * time.Minute))
}

// GetExpiration returns the expiration time.
func (c *GitHubAppCredentials) GetExpiration() (*time.Time, error) {
	return &c.Expiration, nil
}

// BuildWhoamiInfo populates WhoamiInfo with GitHub App credentials.
func (c *GitHubAppCredentials) BuildWhoamiInfo(info *WhoamiInfo) {
	if info.Environment == nil {
		info.Environment = make(map[string]string)
	}
	info.Environment["GITHUB_TOKEN"] = c.Token
	info.Environment["GH_TOKEN"] = c.Token
	info.Environment["GITHUB_APP_ID"] = c.AppID
	info.Environment["GITHUB_INSTALLATION_ID"] = c.InstallationID
}
