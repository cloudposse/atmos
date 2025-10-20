package github

import (
	"fmt"
	"runtime"

	"github.com/zalando/go-keyring"
)

// osKeychainStore implements KeychainStore using OS-native keychains.
// Uses github.com/zalando/go-keyring which provides cross-platform support:
// - macOS: Keychain Access
// - Windows: Credential Manager
// - Linux: Secret Service (GNOME Keyring, KWallet).
type osKeychainStore struct{}

// newOSKeychainStore creates a new OS keychain store.
func newOSKeychainStore() KeychainStore {
	return &osKeychainStore{}
}

// Get retrieves a token from the OS keychain.
func (s *osKeychainStore) Get(service string, account string) (string, error) {
	token, err := keyring.Get(service, account)
	if err != nil {
		// go-keyring returns ErrNotFound on all platforms when item doesn't exist.
		if err == keyring.ErrNotFound {
			return "", fmt.Errorf("token not found in %s keychain for service=%s account=%s", runtime.GOOS, service, account)
		}
		return "", fmt.Errorf("failed to get token from %s keychain: %w", runtime.GOOS, err)
	}
	return token, nil
}

// Set stores a token in the OS keychain.
func (s *osKeychainStore) Set(service string, account string, token string) error {
	err := keyring.Set(service, account, token)
	if err != nil {
		return fmt.Errorf("failed to store token in %s keychain: %w", runtime.GOOS, err)
	}
	return nil
}

// Delete removes a token from the OS keychain.
func (s *osKeychainStore) Delete(service string, account string) error {
	err := keyring.Delete(service, account)
	if err != nil {
		// go-keyring returns ErrNotFound if item doesn't exist.
		if err == keyring.ErrNotFound {
			return fmt.Errorf("token not found in %s keychain for service=%s account=%s", runtime.GOOS, service, account)
		}
		return fmt.Errorf("failed to delete token from %s keychain: %w", runtime.GOOS, err)
	}
	return nil
}
