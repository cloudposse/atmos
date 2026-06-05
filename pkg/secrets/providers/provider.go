// Package providers implements the secret backend providers for the Atmos secrets subsystem.
// It is a leaf package (it does not import pkg/secrets) exposing a backend-agnostic Provider
// interface plus a store-backed adapter (track 1) and a native SOPS provider (track 2).
package providers

import (
	"errors"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
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
	// ErrSopsBinaryNotFound indicates the sops binary is required but not on PATH.
	ErrSopsBinaryNotFound = errors.New("sops binary not found on PATH (install via the Atmos toolchain or your package manager)")
	// ErrSopsOperation indicates a sops binary invocation failed.
	ErrSopsOperation = errors.New("sops operation failed")
	// ErrSerialize indicates a value could not be serialized for a backend.
	ErrSerialize = errors.New("failed to serialize secret value")
)

// NewStore builds a store-backed provider (track 1) for a `secret: true` store named `name`.
func NewStore(atmosConfig *schema.AtmosConfiguration, name string) (Provider, error) {
	defer perf.Track(atmosConfig, "providers.NewStore")()

	return newStoreProvider(atmosConfig, name)
}

// NewSops builds a SOPS provider (track 2) named `name`. Provider definitions are resolved
// from `sectionProviders` (a stack/component `secrets.providers` map) first, then from the
// top-level `secrets.providers` in atmos.yaml.
func NewSops(atmosConfig *schema.AtmosConfiguration, name string, sectionProviders map[string]any) (Provider, error) {
	defer perf.Track(atmosConfig, "providers.NewSops")()

	return newSopsProvider(atmosConfig, name, sectionProviders)
}
