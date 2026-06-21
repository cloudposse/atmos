// Package providers implements the secret backend providers for the Atmos secrets subsystem.
// It is a leaf package (it does not import pkg/secrets) exposing a backend-agnostic Provider
// interface plus a store-backed adapter (track 1) and a native SOPS provider (track 2).
package providers

import (
	"errors"
)

// Scope identifies the addressing level at which a secret value is stored. Atmos exposes three
// scopes forming a ladder of sharing (instance → stack → global); each provider maps a scope to
// its native primitive (file path, key path, environment) and declares which scopes it supports
// via Provider.SupportsScope. An empty Scope is treated as ScopeInstance for back-compat.
type Scope string

const (
	// ScopeInstance stores a value per component instance (stack + component). Default.
	ScopeInstance Scope = "instance"
	// ScopeStack stores a single value shared by every instance in a stack (no component segment).
	ScopeStack Scope = "stack"
	// ScopeGlobal stores a single value shared by every stack and component that resolves through
	// the same backend (no stack or component segment). Sharing is bounded by the backend the
	// store points at (account/project/prefix), which remains the isolation boundary.
	ScopeGlobal Scope = "global"
)

// Coordinate identifies a single secret value within a backend's namespace.
type Coordinate struct {
	Stack     string
	Component string
	Key       string
	// Scope is the addressing level (instance, stack, or global). Providers interpret it in
	// their own terms. Empty is treated as ScopeInstance. For ScopeStack, Component is empty;
	// for ScopeGlobal, both Stack and Component are empty.
	Scope Scope
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
	// SupportsScope reports whether the provider can represent the given scope. A declared
	// secret whose resolved scope is unsupported is rejected with ErrScopeUnsupported before any
	// write. An empty scope (ScopeInstance) must always be supported.
	SupportsScope(scope Scope) bool
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
	// ErrKeygenNotSupported indicates a provider implements the keygen capability but cannot
	// generate for this particular vault/kind (e.g. a KMS/GPG-backed SOPS vault). Callers should
	// surface it as a friendly "not implemented" message, not a hard failure.
	ErrKeygenNotSupported = errors.New("key generation is not supported for this vault")
	// ErrSecretFileNotFound indicates the referenced SOPS file does not exist.
	ErrSecretFileNotFound = errors.New("SOPS file not found")
	// ErrSecretNotInitialized indicates the secret key is absent from its backend.
	ErrSecretNotInitialized = errors.New("secret is not initialized in its backend")
	// ErrScopeUnsupported indicates a declared secret's resolved scope is not supported by its
	// backend (e.g. an instance-scoped secret on a backend that only scopes by environment).
	ErrScopeUnsupported = errors.New("secret scope not supported by backend")
)

// FileResettable is an optional capability for file-based providers (e.g. SOPS) that can rewrite
// their whole backing file to a clean, empty state (creating it if missing). Store-backed
// providers do not implement it. Callers type-assert for this capability.
type FileResettable interface {
	// Reset overwrites the provider's backing file with an empty document for the coordinate's scope.
	Reset(coord Coordinate) error
}

// LocalStatus is an optional capability marking providers whose Status() existence check is
// credential-free — it needs no network access, no authentication, and no decryption. SOPS
// reports local because "is the key present?" is answered from the cleartext key names in the
// encrypted file. Store-backed providers report local only when their underlying store is local
// (e.g. the OS keychain); remote stores (SSM, Secrets Manager, Key Vault, GCP, Vault, 1Password)
// are not local. `atmos secret list` always checks local providers, but reports non-local
// providers as Unknown unless verification is explicitly requested (`--verify`).
type LocalStatus interface {
	// LocalStatusCheck reports whether Status() is credential-free for this provider instance.
	LocalStatusCheck() bool
}

// FilePathProvider is an optional capability for file-backed providers (e.g. SOPS) that can
// report the on-disk path a coordinate resolves to. `describe affected` uses it to treat the
// backing file as an automatic dependency of every component that consumes the secret, so a
// changed secret file marks its consumers affected. Store-backed providers do not implement it.
type FilePathProvider interface {
	// FilePath returns the backing file path the coordinate resolves to.
	FilePath(coord Coordinate) (string, error)
}
