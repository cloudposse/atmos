package authstore

import (
	"encoding/json"
	"time"

	"github.com/zalando/go-keyring"
)

// Constants
const service = "atmos-auth"

// AuthMethod represents different login methods supported
type AuthMethod string

const (
	MethodSSO  AuthMethod = "sso"
	MethodSAML AuthMethod = "saml"
	MethodUser AuthMethod = "user"
)

// AuthCredential holds the authentication token and metadata
type AuthCredential struct {
	Method      AuthMethod `json:"method"`
	Token       string     `json:"token"`
	ExpiresAt   time.Time  `json:"expires_at"`
	LastUpdated time.Time  `json:"last_updated"`
}

// AuthStore interface for storing/retrieving credentials
type AuthStore interface {
	Get(alias string) (*AuthCredential, error)
	Set(alias string, cred *AuthCredential) error
	Delete(alias string) error
	IsValid(cred *AuthCredential) bool
}

// KeyringAuthStore implements AuthStore using OS keyring
type KeyringAuthStore struct{}

func NewKeyringAuthStore() *KeyringAuthStore {
	return &KeyringAuthStore{}
}

// Get retrieves the credential from the keyring
func (k *KeyringAuthStore) Get(alias string) (*AuthCredential, error) {
	secret, err := keyring.Get(service, alias)
	if err != nil {
		return nil, err
	}
	var cred AuthCredential
	if err := json.Unmarshal([]byte(secret), &cred); err != nil {
		return nil, err
	}
	return &cred, nil
}

// Set stores the credential in the keyring
func (k *KeyringAuthStore) Set(alias string, cred *AuthCredential) error {
	cred.LastUpdated = time.Now()
	bytes, err := json.Marshal(cred)
	if err != nil {
		return err
	}
	return keyring.Set(service, alias, string(bytes))
}

// Delete removes the credential from the keyring
func (k *KeyringAuthStore) Delete(alias string) error {
	return keyring.Delete(service, alias)
}

// IsValid checks if a credential is still valid (e.g. not expired)
func (k *KeyringAuthStore) IsValid(cred *AuthCredential) bool {
	return cred.ExpiresAt.After(time.Now())
}
