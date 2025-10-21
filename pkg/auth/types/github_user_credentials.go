package types

import (
	"time"
)

// GitHubUserCredentials defines GitHub User access token credentials from Device Flow.
type GitHubUserCredentials struct {
	Token      string    `json:"token,omitempty"`
	Provider   string    `json:"provider,omitempty"`
	Expiration time.Time `json:"expiration,omitempty"`
}

// IsExpired implements ICredentials for GitHubUserCredentials.
func (c *GitHubUserCredentials) IsExpired() bool {
	if c.Expiration.IsZero() {
		return false
	}
	// 5m skew to avoid edge expirations.
	return time.Now().After(c.Expiration.Add(-5 * time.Minute))
}

// GetExpiration implements ICredentials for GitHubUserCredentials.
func (c *GitHubUserCredentials) GetExpiration() (*time.Time, error) {
	if c.Expiration.IsZero() {
		return nil, nil
	}
	return &c.Expiration, nil
}

// BuildWhoamiInfo implements ICredentials for GitHubUserCredentials.
func (c *GitHubUserCredentials) BuildWhoamiInfo(info *WhoamiInfo) {
	if info == nil {
		return
	}

	if exp, _ := c.GetExpiration(); exp != nil {
		info.Expiration = exp
	}

	// Add GitHub-specific information to whoami.
	if info.Environment == nil {
		info.Environment = make(map[string]string)
	}
	info.Environment["GITHUB_TOKEN"] = c.Token
	info.Environment["GH_TOKEN"] = c.Token
}
