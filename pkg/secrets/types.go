package secrets

import (
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/secrets/providers"
)

// BackendType distinguishes a store-backed (track 1) declaration from a SOPS (track 2) one.
type BackendType string

const (
	// BackendStore is a store-backed secret (a `secret: true` store).
	BackendStore BackendType = "store"
	// BackendSops is a SOPS file-backed secret.
	BackendSops BackendType = "sops"
)

// Declaration is a single declared secret resolved from a component's secrets.vars.
type Declaration struct {
	// Name is the secret name (the key used by `!secret NAME`).
	Name string
	// Description is a human-readable description.
	Description string
	// BackendType selects the backend track (store vs sops).
	BackendType BackendType
	// BackendName is the referenced store name (track 1) or SOPS provider name (track 2).
	BackendName string
	// Required marks the secret as required for validation.
	Required bool
}

// Status describes whether a declared secret is initialized in its backend.
type Status struct {
	Declaration Declaration
	Coordinate  providers.Coordinate
	Initialized bool
	// Err holds any error encountered while checking status (e.g. access denied).
	Err error
}

// ResolveOptions carries optional modifiers parsed from the !secret function or CLI flags.
type ResolveOptions struct {
	// Path is an optional YQ-style path expression applied to a structured secret value.
	Path string
	// Default is an optional fallback value used when the secret is missing.
	Default *string
}

// coordinateForDeclaration builds the backend coordinate for a declaration in a scope.
func coordinateForDeclaration(decl Declaration, stack, component string) providers.Coordinate {
	defer perf.Track(nil, "secrets.coordinateForDeclaration")()

	return providers.Coordinate{Stack: stack, Component: component, Key: decl.Name}
}
