package store

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/cloudposse/atmos/pkg/keyring"
)

const (
	// Namespaces keychain entries (OS-keychain service / file ServiceName).
	defaultKeychainService = "atmos-secrets"
	// Prepended to every composed key.
	defaultKeychainPrefix = "atmos"
	// Splits a stack name into key segments.
	defaultKeychainStackDelimiter = "-"
	// Joins the composed key segments.
	keychainKeyDelimiter = "/"
)

// KeychainStoreOptions configures a keychain secret store backed by pkg/keyring. Unlike the
// read-only cloud stores, a keychain store is writable, making it a good local-development
// backend for `atmos secret set/get/delete` (and a place to keep bootstrap credentials like a
// 1Password token or a SOPS age key).
type KeychainStoreOptions struct {
	// Backend selects the keyring backend: "system" (OS keychain, default), "file" (encrypted
	// file), or "memory" (testing).
	Backend string `mapstructure:"backend"`
	// Service namespaces the entries. Defaults to "atmos-secrets".
	Service string `mapstructure:"service"`
	// FileDir is the directory for the file backend (defaults to the XDG data dir).
	FileDir string `mapstructure:"file_dir"`
	// PasswordEnv names the environment variable holding the file-backend password (defaults to
	// ATMOS_KEYRING_PASSWORD).
	PasswordEnv string `mapstructure:"password_env"`
	// Prefix is prepended to every composed key. Defaults to "atmos".
	Prefix string `mapstructure:"prefix"`
	// StackDelimiter splits the stack into key segments. Defaults to "-". A pointer distinguishes
	// "unset" (use default) from an explicit empty string.
	StackDelimiter *string `mapstructure:"stack_delimiter"`
}

// KeychainStore implements a writable Store over an OS keychain or encrypted file via pkg/keyring.
type KeychainStore struct {
	kr             keyring.Keyring
	prefix         string
	stackDelimiter string
}

// Ensure KeychainStore implements the expected interfaces.
var (
	_ Store          = (*KeychainStore)(nil)
	_ DeletableStore = (*KeychainStore)(nil)
	_ StatusStore    = (*KeychainStore)(nil)
)

// NewKeychainStore initializes a keychain store. Constructing the system backend probes keyring
// availability, so an unusable keychain (e.g. a headless container) fails here rather than
// silently dropping writes.
func NewKeychainStore(options *KeychainStoreOptions) (Store, error) {
	backend := options.Backend
	if backend == "" {
		backend = keyring.TypeSystem
	}
	service := options.Service
	if service == "" {
		service = defaultKeychainService
	}

	kr, err := keyring.New(keyring.Config{
		Type:        backend,
		ServiceName: service,
		FileDir:     options.FileDir,
		PasswordEnv: options.PasswordEnv,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrKeychainInit, err)
	}

	prefix := options.Prefix
	if prefix == "" {
		prefix = defaultKeychainPrefix
	}
	stackDelimiter := defaultKeychainStackDelimiter
	if options.StackDelimiter != nil {
		stackDelimiter = *options.StackDelimiter
	}

	return &KeychainStore{kr: kr, prefix: prefix, stackDelimiter: stackDelimiter}, nil
}

// composeKey builds the stable keychain key for a stack/component/key triple. An empty stack
// and/or component is permitted: scoped secret coordinates (stack/global scope) omit those
// path segments.
func (s *KeychainStore) composeKey(stack, component, key string) (string, error) {
	if key == "" {
		return "", ErrEmptyKey
	}
	return getKey(s.prefix, s.stackDelimiter, stack, component, key, keychainKeyDelimiter)
}

// Set stores a value for the stack/component/key triple. The value is JSON-encoded so any type
// round-trips through the string-valued keyring.
func (s *KeychainStore) Set(stack string, component string, key string, value any) error {
	if value == nil {
		return ErrNilValue
	}
	composed, err := s.composeKey(stack, component, key)
	if err != nil {
		return err
	}
	encoded, err := keychainEncodeValue(value)
	if err != nil {
		return err
	}
	if err := s.kr.Set(composed, encoded); err != nil {
		return fmt.Errorf(errWrapFormatWithID, ErrKeychainWrite, composed, err)
	}
	return nil
}

// Get retrieves the value for the stack/component/key triple.
func (s *KeychainStore) Get(stack string, component string, key string) (any, error) {
	composed, err := s.composeKey(stack, component, key)
	if err != nil {
		return nil, err
	}
	return s.get(composed)
}

// GetKey retrieves a value directly by its composed key, without stack/component context.
func (s *KeychainStore) GetKey(key string) (any, error) {
	if key == "" {
		return nil, ErrEmptyKey
	}
	return s.get(key)
}

func (s *KeychainStore) get(composed string) (any, error) {
	raw, err := s.kr.Get(composed)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return nil, fmt.Errorf(errWrapFormatWithID, ErrKeychainNotFound, composed, err)
		}
		return nil, fmt.Errorf(errWrapFormatWithID, ErrKeychainRead, composed, err)
	}
	return keychainDecodeValue(raw), nil
}

// Delete removes the value for the stack/component/key triple. It is idempotent.
func (s *KeychainStore) Delete(stack string, component string, key string) error {
	composed, err := s.composeKey(stack, component, key)
	if err != nil {
		return err
	}
	if err := s.kr.Delete(composed); err != nil {
		return fmt.Errorf(errWrapFormatWithID, ErrKeychainDelete, composed, err)
	}
	return nil
}

// Has reports whether a value exists for the stack/component/key triple. It uses the keyring's
// native existence check — no value is retrieved or decrypted.
func (s *KeychainStore) Has(stack string, component string, key string) (bool, error) {
	composed, err := s.composeKey(stack, component, key)
	if err != nil {
		return false, err
	}
	return s.kr.Has(composed)
}

// IsLocal reports that the OS keychain operates without network access or authentication, so
// `atmos secret list` can check its status for free (no --verify needed). Implements LocalStore.
func (s *KeychainStore) IsLocal() bool {
	return true
}

// keychainEncodeValue JSON-encodes a value for storage so any type round-trips through the
// string-valued keyring.
func keychainEncodeValue(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrKeychainWrite, err)
	}
	return string(data), nil
}

// keychainDecodeValue reverses keychainEncodeValue. A value that is not valid JSON (e.g. a raw
// token written out-of-band) is returned verbatim as a string.
func keychainDecodeValue(raw string) any {
	var result any
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return raw
	}
	return result
}
