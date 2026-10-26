package keyring

import "errors"

var (
	// ErrNotFound indicates the requested key does not exist in the backend.
	ErrNotFound = errors.New("keyring: key not found")
	// ErrListNotSupported indicates the backend cannot enumerate keys (e.g. the OS keychain).
	ErrListNotSupported = errors.New("keyring: listing keys is not supported by this backend")
	// ErrUnknownBackend indicates an unrecognized Config.Type.
	ErrUnknownBackend = errors.New("keyring: unknown backend type")
	// ErrUnavailable indicates a backend could not be initialized (e.g. no dbus in a container,
	// or the file directory could not be created).
	ErrUnavailable = errors.New("keyring: backend unavailable")
	// ErrPasswordRequired indicates the file backend needs a password but none was provided.
	ErrPasswordRequired = errors.New("keyring: password required")
	// ErrPasswordTooShort indicates the file backend password is below the minimum length.
	ErrPasswordTooShort = errors.New("keyring: password too short")
)
