package types

import "context"

// CredentialPersistence is the optional interface implemented by identities whose
// credentials must never be written to the Atmos credential store (the keyring).
//
// Some identities do not own their credentials: an aws/credential-process identity runs an
// external helper that is the source of truth, so persisting its output in the keyring would
// copy secrets the user kept out of Atmos on purpose and would keep reusing them after the
// helper's own session is gone. Such identities report false and the generic auth manager
// skips every keyring write (login, whoami, chain steps) and read for them. Identities that
// do not implement this interface are persisted as before.
type CredentialPersistence interface {
	// PersistsCredentials reports whether credentials for this identity may be stored in the keyring.
	PersistsCredentials() bool
}

// PersistsCredentialsInKeyring reports whether credentials of the given identity may be written to
// the keyring. Identities that do not implement CredentialPersistence default to true.
func PersistsCredentialsInKeyring(identity Identity) bool {
	if policy, ok := identity.(CredentialPersistence); ok {
		return policy.PersistsCredentials()
	}
	return true
}

// CredentialRestorer is implemented by identities whose runtime state is derived from the stored
// credential and is kept only in memory (for example a credential made of environment variables).
// The auth manager calls it when a valid stored credential is reused without authenticating, so
// the identity can serve Environment and PrepareEnvironment in a fresh process. A non-nil error
// means the stored credential cannot be used and the caller should authenticate instead.
type CredentialRestorer interface {
	RestoreCredentials(ctx context.Context, creds ICredentials) error
}
