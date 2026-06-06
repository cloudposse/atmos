package keyring

import "github.com/cloudposse/atmos/pkg/perf"

// noopKeyring is a backend for environments without a usable keyring (e.g. containers with no
// dbus). Writes are dropped and reads report nothing stored; callers fall back to other sources.
type noopKeyring struct{}

func newNoopKeyring() *noopKeyring {
	return &noopKeyring{}
}

func (s *noopKeyring) Get(key string) (string, error) {
	defer perf.Track(nil, "keyring.noopKeyring.Get")()

	return "", ErrNotFound
}

func (s *noopKeyring) Set(key string, value string) error {
	defer perf.Track(nil, "keyring.noopKeyring.Set")()

	return nil
}

func (s *noopKeyring) Delete(key string) error {
	defer perf.Track(nil, "keyring.noopKeyring.Delete")()

	return nil
}

func (s *noopKeyring) Has(key string) (bool, error) {
	defer perf.Track(nil, "keyring.noopKeyring.Has")()

	return false, nil
}

func (s *noopKeyring) List() ([]string, error) {
	defer perf.Track(nil, "keyring.noopKeyring.List")()

	return []string{}, nil
}

func (s *noopKeyring) Type() string {
	defer perf.Track(nil, "keyring.noopKeyring.Type")()

	return TypeNoop
}
