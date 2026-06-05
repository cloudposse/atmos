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

	// ErrInvalidSecretArgs indicates the !secret function received invalid arguments.
	ErrInvalidSecretArgs = errors.New("invalid !secret arguments")

	// ErrEmptyName indicates an empty secret name.
	ErrEmptyName = errors.New("secret name cannot be empty")
)
