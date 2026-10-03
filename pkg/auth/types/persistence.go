package types

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
