package secrets

import (
	"errors"

	"github.com/cloudposse/atmos/pkg/secrets/providers"
)

// Provider-construction errors are owned by the providers package; re-export the ones callers
// and tests assert on so they remain reachable from pkg/secrets.
var (
	ErrStoreNotFound      = providers.ErrStoreNotFound
	ErrStoreNotSecret     = providers.ErrStoreNotSecret
	ErrProviderNotFound   = providers.ErrProviderNotFound
	ErrDeleteNotSupported = providers.ErrDeleteNotSupported
)

// Sentinel errors for the secrets subsystem.
var (
	// ErrSecretNotDeclared indicates a secret name was used but not declared in the
	// component's secrets.vars (or an inherited declaration).
	ErrSecretNotDeclared = errors.New("secret is not declared")

	// ErrSecretMissing indicates a declared secret has no value in its backend.
	ErrSecretMissing = errors.New("secret is not initialized in its backend")

	// ErrUndeclaredKey indicates an imported/pushed key has no matching declaration.
	ErrUndeclaredKey = errors.New("key is not declared as a secret")

	// ErrNoBackend indicates a declaration references no backend (neither store nor sops).
	ErrNoBackend = errors.New("secret declaration has no backend (set `store:` or `sops:`)")

	// ErrAmbiguousBackend indicates a declaration references more than one backend.
	ErrAmbiguousBackend = errors.New("secret declaration must set exactly one of `store:` or `sops:`")

	// ErrKeygenUnsupported indicates the referenced vault's backend cannot generate a key.
	ErrKeygenUnsupported = errors.New("vault backend does not support key generation")

	// ErrInvalidSecretArgs indicates the !secret function received invalid arguments.
	ErrInvalidSecretArgs = errors.New("invalid !secret arguments")

	// ErrEmptyName indicates an empty secret name.
	ErrEmptyName = errors.New("secret name cannot be empty")

	// ErrScopeConflict indicates a declaration carries an explicit `scope` that conflicts with the
	// scope implied by its position (the one-way rule: an instance-declared secret can never be
	// stack-scoped, and a stack-level declaration can't be instance-scoped).
	ErrScopeConflict = errors.New("secret scope conflicts with declaration position")

	// ErrSecretNotOverridable indicates an attempt to set an instance-level value for a secret that
	// is stack-scoped at the targeted component. Overriding requires the instance to opt in by
	// declaring the secret under the component; otherwise the write is rejected (no silent shadow).
	ErrSecretNotOverridable = errors.New("secret is stack-scoped and not overridable at this instance")

	// ErrSopsCollision indicates two scopes resolve to a colliding SOPS file: distinct instances
	// sharing a file (no isolation), or a stack-scoped secret resolving per-component (not shared).
	ErrSopsCollision = errors.New("SOPS secret files collide across scopes")
)
