package auth

import (
	"context"
	"fmt"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
)

// Reuse rules for persisted chain steps (#3720 item 2.1), applied only to steps before the
// target; the target is always re-authenticated, so its behavior is unchanged:
//   - Keyring entries are reused only when the envelope's digest equals the digest of the step's
//     current configuration. Envelopes written by older versions have no digest and are treated
//     as a miss: whether the configuration changed cannot be determined, because every step can
//     be overridden by stack or component auth. The step is re-authenticated once and stored
//     with a digest, so an upgrade costs at most one extra authentication of each such step.
//   - Credentials an identity keeps in its own files are reused only when the digest record
//     written after that identity's last login matches the current configuration and the
//     credentials' expiration and non-secret credential ID. Files without a record are treated
//     the same way as above. The record is written after the identity's own login (the target of
//     a login) and after each intermediate step whose credentials are persisted outside the keyring.
//   - A store that cannot persist digests (a custom store, or the noop keyring, which persists
//     nothing) keeps the previous name-only behavior, because no digest could ever match.

// configDigestStore returns the credential store as a ConfigDigestStore when it can persist
// configuration digests.
func (m *manager) configDigestStore() (credentials.ConfigDigestStore, bool) {
	store, ok := m.credentialStore.(credentials.ConfigDigestStore)
	if !ok || !store.SupportsConfigDigests() {
		return nil, false
	}
	return store, true
}

// storeChainStepCredentials persists the credentials of chain[index] in the keyring together
// with the digest of its current configuration.
func (m *manager) storeChainStepCredentials(chain []string, index int, creds types.ICredentials) error {
	return m.storeWithDigest(chain[index], creds, func() (string, bool) { return m.chainPrefixDigest(chain, index) })
}

// storeNamedCredentials persists the credentials of the named identity or provider in the
// keyring together with the digest of its current configuration.
func (m *manager) storeNamedCredentials(name string, creds types.ICredentials) error {
	return m.storeWithDigest(name, creds, func() (string, bool) { return m.namedStepDigest(name) })
}

// storeWithDigest stores credentials with the digest returned by digestOf when the store keeps
// digests. A configuration that cannot be digested is stored without one, so it is never
// reused as an intermediate step.
func (m *manager) storeWithDigest(name string, creds types.ICredentials, digestOf func() (string, bool)) error {
	if store, ok := m.configDigestStore(); ok {
		if digest, ok := digestOf(); ok {
			return store.StoreWithConfigDigest(name, creds, m.realm.Value, digest)
		}
	}
	return m.credentialStore.Store(name, creds, m.realm.Value)
}

// recordExternalStepDigest records, after a successful login of the named identity, the digest
// of the configuration that produced the credentials its PostAuthenticate wrote to the
// identity's own files. Credentials persisted in the keyring carry their digest in the envelope
// instead. Best effort: without a record the files are simply not reused as an intermediate step.
func (m *manager) recordExternalStepDigest(name string, creds types.ICredentials) {
	store, ok := m.configDigestStore()
	if !ok || creds == nil || m.chainRootIsAmbient() || (!isSessionToken(creds) && m.identityPersistsCredentials(name)) {
		return
	}
	digest, ok := m.namedStepDigest(name)
	if !ok {
		return
	}
	record := credentials.ExternalConfigDigest{Digest: digest, Expiration: expirationStamp(creds), CredentialID: credentialID(creds)}
	if err := store.StoreExternalConfigDigest(name, m.realm.Value, record); err != nil {
		log.Debug("Failed to record configuration digest of identity credentials", logKeyIdentity, name, "error", err)
	}
}

// loadChainStepCredentials loads the persisted credentials of chain[index] for reuse as the
// source of the next step, and refuses them with ErrChainStepConfigChanged when they cannot be
// shown to match the step's current configuration.
func (m *manager) loadChainStepCredentials(ctx context.Context, chain []string, index int) (types.ICredentials, error) {
	name := chain[index]
	loaded, err := m.loadCredentialsWithSource(ctx, name)
	if err != nil {
		return nil, err
	}
	if index == len(chain)-1 {
		return loaded.creds, nil
	}
	if err := m.verifyChainStepDigest(chain, index, loaded); err != nil {
		return nil, err
	}
	return loaded.creds, nil
}

// verifyChainStepDigest checks persisted credentials of an intermediate chain step against the
// digest of the step's current configuration (see the reuse rules above).
func (m *manager) verifyChainStepDigest(chain []string, index int, loaded loadedCredentials) error {
	store, ok := m.configDigestStore()
	if !ok {
		return nil
	}
	name := chain[index]
	want, ok := m.chainPrefixDigest(chain, index)
	if !ok {
		return fmt.Errorf("%w: %q: configuration cannot be digested", errUtils.ErrChainStepConfigChanged, name)
	}
	got, reason := loaded.digest, "keyring entry"
	if loaded.source == credentialSourceIdentityStorage {
		reason = "identity credential files"
		record, err := store.RetrieveExternalConfigDigest(name, m.realm.Value)
		if err != nil || record.Expiration != expirationStamp(loaded.creds) || record.CredentialID != credentialID(loaded.creds) {
			got = ""
		} else {
			got = record.Digest
		}
	}
	if got == want {
		return nil
	}
	if got == "" {
		return fmt.Errorf("%w: %q: %s has no matching configuration digest (written by an older version or another tool)", errUtils.ErrChainStepConfigChanged, name, reason)
	}
	return fmt.Errorf("%w: %q: %s was written for a different configuration", errUtils.ErrChainStepConfigChanged, name, reason)
}

// expirationStamp formats the credentials' expiration for an ExternalConfigDigest record, at
// second precision in UTC, so the stamp survives a round trip through credential files.
func expirationStamp(creds types.ICredentials) string {
	if creds == nil {
		return ""
	}
	expiration, err := creds.GetExpiration()
	if err != nil || expiration == nil {
		return ""
	}
	return expiration.UTC().Truncate(time.Second).Format(time.RFC3339)
}

// credentialID returns the non-secret identifier of a credential set for an ExternalConfigDigest
// record: the AWS access key ID, which names the principal or session and is not a secret. It is
// empty for credentials without one.
func credentialID(creds types.ICredentials) string {
	if awsCreds, ok := creds.(*types.AWSCredentials); ok {
		return awsCreds.AccessKeyID
	}
	return ""
}
