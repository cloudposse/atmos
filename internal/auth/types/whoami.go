package types

import "time"

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
