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

	// CredentialsRef holds an opaque keystore handle for rehydrating credentials without exposing secrets.
	CredentialsRef string    `json:"credentials_ref,omitempty" yaml:"credentials_ref,omitempty"`
	LastUpdated    time.Time `json:"last_updated"`
}

// Rehydrate ensures that the Credentials field is populated by retrieving
// the underlying secret material from the provided credential store if
// Credentials is nil and a non-empty CredentialsRef is available.
// This avoids exposing secrets during serialization while allowing
// consumers to lazily fetch them when needed.
func (w *WhoamiInfo) Rehydrate(store CredentialStore) error {
	if w == nil {
		return nil
	}
	if w.Credentials != nil || w.CredentialsRef == "" {
		return nil
	}
	// Be tolerant of a nil store to avoid panics in consumers.
	if store == nil {
		return nil
	}
	creds, err := store.Retrieve(w.CredentialsRef)
	if err != nil {
		return err
	}
	w.Credentials = creds
	return nil
}
