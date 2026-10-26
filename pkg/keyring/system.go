package keyring

import (
	"errors"
	"fmt"

	zkeyring "github.com/zalando/go-keyring"
)

// systemKeyring stores values in the OS keychain via zalando go-keyring. Each entry is addressed
// by (service=key, account=ServiceName); the account namespaces all of Atmos's entries.
type systemKeyring struct {
	account string
}

// availabilityProbeKey is a key we never write; probing it distinguishes "keyring works, key
// absent" (ErrNotFound) from "keyring unavailable" (any other error).
const availabilityProbeKey = "atmos-keyring-availability-probe"

// newSystemKeyring constructs the system backend, probing availability up front so containers
// without a keyring service fail here (letting the caller decide on a fallback).
func newSystemKeyring(cfg Config) (*systemKeyring, error) {
	account := cfg.ServiceName
	if account == "" {
		account = defaultServiceName
	}

	if _, err := zkeyring.Get(availabilityProbeKey, account); err != nil && !errors.Is(err, zkeyring.ErrNotFound) {
		return nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}

	return &systemKeyring{account: account}, nil
}

func (s *systemKeyring) Get(key string) (string, error) {
	value, err := zkeyring.Get(key, s.account)
	if err != nil {
		if errors.Is(err, zkeyring.ErrNotFound) {
			return "", ErrNotFound
		}
		return "", err
	}
	return value, nil
}

func (s *systemKeyring) Set(key string, value string) error {
	return zkeyring.Set(key, s.account, value)
}

func (s *systemKeyring) Delete(key string) error {
	if err := zkeyring.Delete(key, s.account); err != nil && !errors.Is(err, zkeyring.ErrNotFound) {
		return err
	}
	return nil
}

func (s *systemKeyring) Has(key string) (bool, error) {
	_, err := s.Get(key)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// List is not supported: go-keyring cannot enumerate entries.
func (s *systemKeyring) List() ([]string, error) {
	return nil, ErrListNotSupported
}

func (s *systemKeyring) Type() string {
	return TypeSystem
}
