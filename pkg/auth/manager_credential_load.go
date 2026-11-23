package auth

import (
	"context"
	"errors"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
)

// loadCredentialsWithFallback retrieves credentials with keyring → identity storage fallback.
// This is the single source of truth for credential retrieval across all auth operations.
// It ensures consistent behavior whether credentials are in keyring or identity storage.
//
// For AWS credentials, session tokens (with expiration) are stored in files but NOT in keyring.
// Keyring stores long-lived IAM credentials for re-authentication. This method prefers
// session credentials from files when they exist and are not expired, so users can see
// accurate expiration times in whoami output.
func (m *manager) loadCredentialsWithFallback(ctx context.Context, identityName string) (types.ICredentials, error) {
	loaded, err := m.loadCredentialsWithSource(ctx, identityName)
	return loaded.creds, err
}

// credentialSource tells where loadCredentialsWithSource found credentials.
type credentialSource int

const (
	// The credentialSourceKeyring source is the credential store (keyring).
	credentialSourceKeyring credentialSource = iota
	// The credentialSourceIdentityStorage source is the identity's own storage (AWS files, etc.).
	credentialSourceIdentityStorage
)

// loadedCredentials are credentials with their source and, for keyring entries, the
// configuration digest stored with them ("" when there is none).
type loadedCredentials struct {
	creds  types.ICredentials
	source credentialSource
	digest string
}

// loadCredentialsWithSource implements loadCredentialsWithFallback and also reports where the
// credentials came from, which the chain needs to verify them (see verifyChainStepDigest).
func (m *manager) loadCredentialsWithSource(ctx context.Context, identityName string) (loadedCredentials, error) {
	// Fast path: Try keyring cache first. Identities that do not persist credentials (the helper
	// owns them) skip it so a stale entry written by an older version is never reused.
	keyringErr := credentials.ErrCredentialsNotFound
	var keyringCreds types.ICredentials
	var keyringDigest string
	if m.identityPersistsCredentials(identityName) {
		keyringCreds, keyringDigest, keyringErr = m.retrieveKeyringCredentials(identityName)
	}
	if keyringErr == nil {
		log.Debug("Retrieved credentials from keyring", logKeyIdentity, identityName)

		// For AWS credentials without session token, also check files for session credentials.
		// Session tokens have expiration info that users want to see in whoami.
		if awsCreds, ok := keyringCreds.(*types.AWSCredentials); ok && awsCreds.SessionToken == "" {
			fileCreds := m.loadSessionCredsFromFiles(ctx, identityName)
			if fileCreds != nil {
				log.Debug("Using session credentials from files (have expiration)", logKeyIdentity, identityName)
				return loadedCredentials{creds: fileCreds, source: credentialSourceIdentityStorage}, nil
			}
		}
		return loadedCredentials{creds: keyringCreds, source: credentialSourceKeyring, digest: keyringDigest}, nil
	}

	// If keyring returned an error other than "not found", propagate it.
	if !errors.Is(keyringErr, credentials.ErrCredentialsNotFound) {
		return loadedCredentials{}, fmt.Errorf("keyring error for identity %q: %w", identityName, keyringErr)
	}

	// Slow path: Fall back to identity storage (AWS files, etc.).
	// This handles cases where credentials were created outside of Atmos
	// (e.g., via AWS CLI/SSO) but are available in standard credential files.
	log.Debug("Credentials not in keyring, trying identity storage", logKeyIdentity, identityName)

	identity, exists := m.identities[identityName]
	if !exists {
		return loadedCredentials{}, fmt.Errorf(errUtils.ErrWrapWithNameFormat, errUtils.ErrIdentityNotInConfig, identityName)
	}

	// Ensure the identity has access to manager for resolving provider information.
	// This builds the authentication chain and sets manager reference so the identity
	// can resolve the root provider for file-based credentials.
	// This is best-effort - if it fails, LoadCredentials will fail with a clear error.
	_ = m.ensureIdentityHasManager(identityName)

	// Delegate to identity's LoadCredentials method.
	// Each identity type knows how to load its own credentials from storage.
	loadedCreds, loadErr := identity.LoadCredentials(ctx)
	if loadErr != nil {
		m.emitRealmMismatchWarning(identityName)
		return loadedCredentials{}, fmt.Errorf("failed to load credentials from identity storage for %q: %w", identityName, loadErr)
	}

	if loadedCreds == nil {
		m.emitRealmMismatchWarning(identityName)
		return loadedCredentials{}, fmt.Errorf("%w: credentials loaded from storage are nil for identity %q", errUtils.ErrNoCredentialsFound, identityName)
	}

	log.Debug("Successfully loaded credentials from identity storage", logKeyIdentity, identityName)
	return loadedCredentials{creds: loadedCreds, source: credentialSourceIdentityStorage}, nil
}

// retrieveKeyringCredentials reads the keyring entry of name in the manager's realm, with the
// configuration digest stored with it when the store keeps digests.
func (m *manager) retrieveKeyringCredentials(name string) (types.ICredentials, string, error) {
	if store, ok := m.configDigestStore(); ok {
		return store.RetrieveWithConfigDigest(name, m.realm.Value)
	}
	creds, err := m.credentialStore.Retrieve(name, m.realm.Value)
	return creds, "", err
}

// loadSessionCredsFromFiles attempts to load session credentials from identity storage files.
// Returns nil if loading fails or credentials are expired.
func (m *manager) loadSessionCredsFromFiles(ctx context.Context, identityName string) types.ICredentials {
	identity, exists := m.identities[identityName]
	if !exists {
		return nil
	}

	_ = m.ensureIdentityHasManager(identityName)

	fileCreds, err := identity.LoadCredentials(ctx)
	if err != nil {
		log.Debug("Could not load credentials from files", logKeyIdentity, identityName, "error", err)
		return nil
	}

	if fileCreds == nil || fileCreds.IsExpired() {
		return nil
	}

	// Only return if these are session credentials (have session token).
	if awsCreds, ok := fileCreds.(*types.AWSCredentials); ok && awsCreds.SessionToken != "" {
		return fileCreds
	}

	return nil
}
