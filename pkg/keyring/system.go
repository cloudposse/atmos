package keyring

import (
	"errors"
	"fmt"

	zkeyring "github.com/zalando/go-keyring"

	"github.com/cloudposse/atmos/pkg/perf"
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
	defer perf.Track(nil, "keyring.newSystemKeyring")()

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
	defer perf.Track(nil, "keyring.systemKeyring.Get")()

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
	defer perf.Track(nil, "keyring.systemKeyring.Set")()

	return zkeyring.Set(key, s.account, value)
}

func (s *systemKeyring) Delete(key string) error {
	defer perf.Track(nil, "keyring.systemKeyring.Delete")()

	if err := zkeyring.Delete(key, s.account); err != nil && !errors.Is(err, zkeyring.ErrNotFound) {
		return err
	}
	return nil
}

func (s *systemKeyring) Has(key string) (bool, error) {
	defer perf.Track(nil, "keyring.systemKeyring.Has")()

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
	defer perf.Track(nil, "keyring.systemKeyring.List")()

	return nil, ErrListNotSupported
}

func (s *systemKeyring) Type() string {
	defer perf.Track(nil, "keyring.systemKeyring.Type")()

	return TypeSystem
}
