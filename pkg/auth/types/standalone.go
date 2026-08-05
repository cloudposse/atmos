package types

import "context"

// Standalone identity kinds: root identities that authenticate without an upstream
// provider step (no `via`). Centralizing this vocabulary here (the shared types layer)
// keeps the generic auth chain manager free of imports on concrete identity
// implementations — the manager asks `types`, never `identities/aws|ambient|emulator`.
const (
	IdentityKindAWSUser    = "aws/user"
	IdentityKindAWSAmbient = "aws/ambient"
	IdentityKindAmbient    = "ambient"

	// ProviderNameAWSUser is the synthetic provider name an aws/user identity reports.
	// Such identities own their credential files directly rather than through a
	// configured provider, so the chain root maps to this name instead of the identity's.
	ProviderNameAWSUser = "aws-user"
)

// IsStandaloneIdentityKind reports whether an identity of the given kind authenticates
// without an upstream provider step (no `via`). Standalone identities form a
// single-element chain and are dispatched directly by the manager. Covers aws/user,
// aws/ambient, generic ambient, and every emulator-bound kind.
//
// This is the config-level (kind string) detection used while building chains, where
// constructed identity instances are not always available. The instance-level
// counterpart is the StandaloneIdentity interface below.
func IsStandaloneIdentityKind(kind string) bool {
	switch kind {
	case IdentityKindAWSUser, IdentityKindAWSAmbient, IdentityKindAmbient:
		return true
	default:
		return IsEmulatorIdentityKind(kind)
	}
}

// StandaloneProviderName returns the synthetic provider name a standalone identity
// reports when it differs from the identity's own name. An aws/user identity owns a
// dedicated "aws-user" provider; other standalone identities report their own name, so
// this returns ok=false for them (callers fall back to the identity name).
func StandaloneProviderName(kind string) (string, bool) {
	if kind == IdentityKindAWSUser {
		return ProviderNameAWSUser, true
	}
	return "", false
}

// StandaloneIdentity is the optional interface implemented by identities that
// authenticate without an upstream provider step. The auth chain manager dispatches to
// these directly through this interface instead of special-casing concrete identity
// kinds, so adding a new standalone identity needs no edit to the generic manager.
// Implemented by aws/user, aws/ambient, generic ambient, and the emulator-bound identities.
type StandaloneIdentity interface {
	// IsStandalone reports whether this identity authenticates without a provider step.
	IsStandalone() bool

	// AuthenticateStandalone authenticates this identity directly, with no upstream
	// credentials. The returned credentials may be nil for identities that mint nothing
	// (ambient, emulator).
	AuthenticateStandalone(ctx context.Context) (ICredentials, error)
}
