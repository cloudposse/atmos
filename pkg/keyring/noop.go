package keyring

// noopKeyring is a backend for environments without a usable keyring (e.g. containers with no
// dbus). Writes are dropped and reads report nothing stored; callers fall back to other sources.
type noopKeyring struct{}

func newNoopKeyring() *noopKeyring {
	return &noopKeyring{}
}

func (s *noopKeyring) Get(key string) (string, error) {
	return "", ErrNotFound
}

func (s *noopKeyring) Set(key string, value string) error {
	return nil
}

func (s *noopKeyring) Delete(key string) error {
	return nil
}

func (s *noopKeyring) Has(key string) (bool, error) {
	return false, nil
}

func (s *noopKeyring) List() ([]string, error) {
	return []string{}, nil
}

func (s *noopKeyring) Type() string {
	return TypeNoop
}
