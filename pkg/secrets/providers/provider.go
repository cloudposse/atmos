// Package providers implements the secret backend providers for the Atmos secrets subsystem.
// It is a leaf package (it does not import pkg/secrets) exposing a backend-agnostic Provider
// interface plus a store-backed adapter (track 1) and a native SOPS provider (track 2).
package providers

import (
	"errors"
)

// Coordinate identifies a single secret value within a backend's namespace.
type Coordinate struct {
	Stack     string
	Component string
	Key       string
}

// Provider is the backend-agnostic CRUD interface the secrets service operates against.
// Track-1 (store-backed) and track-2 (SOPS) providers both implement it.
type Provider interface {
	// Set stores a value at the coordinate.
	Set(coord Coordinate, value any) error
	// Get retrieves a value at the coordinate.
	Get(coord Coordinate) (any, error)
	// Delete removes a value at the coordinate.
	Delete(coord Coordinate) error
	// Status reports whether a value exists at the coordinate.
	Status(coord Coordinate) (bool, error)
	// Kind returns the provider kind (e.g. aws/ssm, sops/age) for display/observability.
	Kind() string
}

// Provider-construction errors.
var (
	// ErrStoreNotFound indicates the referenced store is not configured.
	ErrStoreNotFound = errors.New("referenced store is not configured")
	// ErrStoreNotSecret indicates the referenced store is not marked `secret: true`.
	ErrStoreNotSecret = errors.New("referenced store is not a secret store (set `secret: true`)")
	// ErrProviderNotFound indicates the referenced SOPS provider is not configured.
	ErrProviderNotFound = errors.New("referenced secrets provider is not configured")
	// ErrDeleteNotSupported indicates the backend cannot delete values.
	ErrDeleteNotSupported = errors.New("backend does not support delete")
	// ErrSopsFilePathTemplate indicates the SOPS `spec.file` Go template could not be rendered.
	ErrSopsFilePathTemplate = errors.New("failed to render SOPS file path template")
	// ErrSopsDecrypt indicates the SOPS file could not be decrypted in-process.
	ErrSopsDecrypt = errors.New("failed to decrypt SOPS file")
	// ErrSopsMacMismatch indicates the SOPS file MAC did not match the computed MAC.
	ErrSopsMacMismatch = errors.New("SOPS MAC mismatch")
	// ErrSopsEncrypt indicates the SOPS file could not be encrypted in-process.
	ErrSopsEncrypt = errors.New("failed to encrypt SOPS file")
	// ErrSopsRecipients indicates encryption recipients could not be resolved for a fresh file.
	ErrSopsRecipients = errors.New("failed to resolve SOPS recipients (set `spec.age_recipients` or add a matching .sops.yaml creation rule)")
	// ErrSopsAgeKeyFile indicates the configured `spec.age_key_file` could not be read or parsed.
	ErrSopsAgeKeyFile = errors.New("failed to load SOPS age key file (`spec.age_key_file`)")
	// ErrSopsAgeKey indicates the inline `spec.age_key` material could not be parsed.
	ErrSopsAgeKey = errors.New("failed to parse SOPS age key (`spec.age_key`)")
	// ErrSecretFileNotFound indicates the referenced SOPS file does not exist.
	ErrSecretFileNotFound = errors.New("SOPS file not found")
	// ErrSecretNotInitialized indicates the secret key is absent from its backend.
	ErrSecretNotInitialized = errors.New("secret is not initialized in its backend")
)

// FileResettable is an optional capability for file-based providers (e.g. SOPS) that can rewrite
// their whole backing file to a clean, empty state (creating it if missing). Store-backed
// providers do not implement it. Callers type-assert for this capability.
type FileResettable interface {
	// Reset overwrites the provider's backing file with an empty document for the coordinate's scope.
	Reset(coord Coordinate) error
}
