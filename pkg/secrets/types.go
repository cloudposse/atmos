package secrets

import (
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/secrets/providers"
)

// BackendType distinguishes a store-backed (track 1) declaration from a SOPS (track 2) one.
type BackendType string

const (
	// BackendStore is a store-backed secret (a `secret: true` store). It mirrors the
	// providers registry track key so the two never drift.
	BackendStore BackendType = BackendType(providers.TrackStore)
	// BackendSops is a SOPS file-backed secret. It mirrors the providers registry track key.
	BackendSops BackendType = BackendType(providers.TrackSops)
)

// Scope is the addressing level at which a secret value is stored (see providers.Scope). It is
// re-exported here so the secrets service and CLI can branch on a declaration's scope without
// importing the providers package directly.
type Scope = providers.Scope

const (
	// ScopeInstance stores a value per component instance (stack + component). Default.
	ScopeInstance = providers.ScopeInstance
	// ScopeStack stores a single value shared by every instance in a stack.
	ScopeStack = providers.ScopeStack
	// ScopeGlobal stores a single value shared by every stack and component using the backend.
	ScopeGlobal = providers.ScopeGlobal
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
	// Reference is an optional backend-specific address for the secret (e.g. a 1Password
	// `op://vault/item/field` reference). It may contain Go-template vars ({{ .atmos_stack }},
	// {{ .atmos_component }}). When set it overrides Name as the backend key; backends that key
	// off the declaration name (most stores, SOPS) ignore it.
	Reference string
	// Required marks the secret as required for validation.
	Required bool
	// Scope is the addressing level (stack vs instance). It is derived from declaration position
	// during stack processing (top-level `secrets:` → stack; component `secrets:` → instance) and
	// stamped onto the declaration; an empty Scope is treated as ScopeInstance.
	Scope Scope
}

// IsStackScoped reports whether the declaration is stored once per stack (shared by all instances).
func (d *Declaration) IsStackScoped() bool { //nolint:lintroller // trivial pure getter; perf.Track overhead is unwarranted.
	return d.Scope == ScopeStack
}

// Status describes whether a declared secret is initialized in its backend.
type Status struct {
	Declaration Declaration
	Coordinate  providers.Coordinate
	Initialized bool
	// Unknown is true when the secret's initialization state was not checked because doing so
	// would require contacting a remote backend (network + credentials), and verification was not
	// requested. Listing is credential-free by default: local backends (e.g. SOPS) are always
	// checked, but remote stores are reported as Unknown unless `--verify` is passed. When Unknown
	// is true, Initialized is meaningless.
	Unknown bool
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

// coordinateForDeclaration builds the backend coordinate for a declaration in a scope. A
// stack-scoped declaration omits the component segment so its value is stored once per stack and
// shared by every instance; a global declaration omits both the stack and component segments so
// its value is stored once per backend and shared everywhere; an instance-scoped declaration
// (the default) keeps both.
func coordinateForDeclaration(decl *Declaration, stack, component string) providers.Coordinate {
	defer perf.Track(nil, "secrets.coordinateForDeclaration")()

	// A reference (when set) overrides the declaration name as the backend key. Reference-based
	// backends (1Password) resolve it directly; name-keyed backends never set it.
	key := decl.Name
	if decl.Reference != "" {
		key = decl.Reference
	}
	scope := decl.Scope
	if scope == "" {
		scope = ScopeInstance
	}
	coord := providers.Coordinate{Key: key, Scope: scope}
	if scope != ScopeGlobal {
		coord.Stack = stack
	}
	if scope != ScopeStack && scope != ScopeGlobal {
		coord.Component = component
	}
	return coord
}
