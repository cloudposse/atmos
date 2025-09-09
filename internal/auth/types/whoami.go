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
	// Credentials holds raw credential material and must never be serialized.
	// Ensure secrets/tokens are not exposed via JSON or YAML outputs.
	Credentials ICredentials `json:"-" yaml:"-"`
	LastUpdated time.Time    `json:"last_updated"`
}
