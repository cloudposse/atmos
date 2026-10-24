// Package keyring provides a credential-agnostic key->string-value secret store backed by the
// OS keychain (zalando go-keyring), an encrypted file (99designs/keyring), or in-memory/noop
// backends. It carries no realm, credential-envelope, or expiry semantics — callers layer those
// on top. It is shared by the auth credential store (pkg/auth/credentials) and the keychain
// secrets store (pkg/store). See docs/prd/secrets-management.md.
package keyring

import (
	"fmt"
)

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_keyring.go -package=keyring

// Backend type identifiers returned by Keyring.Type().
const (
	TypeSystem = "system"
	TypeFile   = "file"
	TypeMemory = "memory"
	TypeNoop   = "noop"
)

const (
	// DefaultServiceName is the OS-keychain service / file ServiceName used when a caller does
	// not set Config.ServiceName.
	defaultServiceName = "atmos"
	// DefaultPasswordEnv is the environment variable consulted for the file backend password
	// when a caller does not set Config.PasswordEnv.
	defaultPasswordEnv = "ATMOS_KEYRING_PASSWORD"
)

// Keyring is a generic key->string-value secret store. Implementations persist raw string
// values; any structure (JSON, envelopes) is the caller's concern.
type Keyring interface {
	// Get returns the value for key, or ErrNotFound if absent.
	Get(key string) (string, error)
	// Set stores value under key, overwriting any existing value.
	Set(key string, value string) error
	// Delete removes key. It is idempotent: deleting an absent key returns nil.
	Delete(key string) error
	// Has reports whether key exists.
	Has(key string) (bool, error)
	// List returns all stored keys. Backends that cannot enumerate (e.g. the OS keychain)
	// return ErrListNotSupported.
	List() ([]string, error)
	// Type returns the backend identifier (one of TypeSystem/TypeFile/TypeMemory/TypeNoop).
	Type() string
}

// Config selects and configures a Keyring backend.
type Config struct {
	// Type is the backend: TypeSystem (default when empty), TypeFile, TypeMemory, or TypeNoop.
	Type string
	// ServiceName namespaces entries: the zalando account for the system backend and the
	// 99designs ServiceName for the file backend. Defaults to "atmos".
	ServiceName string
	// FileDir is the directory for the file backend. Empty uses the XDG data dir.
	FileDir string
	// PasswordEnv names the environment variable holding the file-backend password. Empty uses
	// ATMOS_KEYRING_PASSWORD.
	PasswordEnv string
}

// New constructs the backend named by cfg.Type. It does NOT fall back between backends: callers
// that want resilience (e.g. system->noop in containers) implement that policy themselves, so a
// store that needs durable writes can surface a hard error instead of silently using noop.
func New(cfg Config) (Keyring, error) {
	switch cfg.Type {
	case "", TypeSystem:
		return newSystemKeyring(cfg)
	case TypeFile:
		return newFileKeyring(cfg)
	case TypeMemory:
		return newMemoryKeyring(), nil
	case TypeNoop:
		return newNoopKeyring(), nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknownBackend, cfg.Type)
	}
}
