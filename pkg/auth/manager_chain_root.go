package auth

import (
	"context"
	"fmt"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
)

// authenticateChainRoot authenticates the root of the current chain (m.chain[0]) before its
// credentials are passed to downstream identities.
//
// A registered provider always wins. When the root is not a provider but a standalone identity
// (aws/user with YAML keys, aws/credential-process, ...), the identity is the credential source
// and is authenticated through the StandaloneIdentity interface instead. Standalone identities
// own session persistence, so their long-lived keyring credentials remain untouched. Anything
// else reports the "provider not registered" error.
func (m *manager) authenticateChainRoot(ctx context.Context) (types.ICredentials, error) {
	root := m.chain[0]

	if provider, exists := m.providers[root]; exists {
		// Allow providers to inspect the chain and prepare pre-auth preferences.
		if err := provider.PreAuthenticate(m); err != nil {
			errUtils.CheckErrorAndPrint(err, "Pre Authenticate", "")
			return nil, fmt.Errorf("%w: provider=%s: %w", errUtils.ErrAuthenticationFailed, root, err)
		}
		return m.authenticateWithProvider(ctx, root)
	}

	if standalone, ok := m.identities[root].(types.StandaloneIdentity); ok && standalone.IsStandalone() {
		log.Debug("Authenticating standalone identity at chain root", logKeyIdentity, root, logKeyChain, m.chain)
		creds, err := standalone.AuthenticateStandalone(ctx)
		if err != nil {
			return nil, errUtils.WrapAuthenticationFailed(err, "identity %q", root)
		}
		return creds, nil
	}

	// Neither a provider nor a standalone identity: report the "provider not registered" error.
	return m.authenticateWithProvider(ctx, root)
}

// isChainCredentialValid distinguishes reusable authenticated sessions from the
// long-lived IAM keys used to create them. Passing user keys directly to the next
// identity would skip GetSessionToken and any configured MFA challenge.
func (m *manager) isChainCredentialValid(name string, creds types.ICredentials) (bool, *time.Time) {
	if m.config != nil && m.config.Identities[name].Kind == types.IdentityKindAWSUser {
		awsCreds, ok := creds.(*types.AWSCredentials)
		if !ok || awsCreds == nil || awsCreds.SessionToken == "" {
			return false, nil
		}
		if expiration, err := awsCreds.GetExpiration(); err != nil || expiration == nil {
			return false, nil
		}
	}
	return m.isCredentialValid(name, creds)
}

// chainRootIsAmbient reports whether the provider at the root of the current chain
// resolves credentials from ambient environment state on every authentication
// (e.g. gcp/adc, azure/cli, and the OIDC providers).
//
// Chains rooted at such a provider are never served from — nor written to — the
// persistent keyring: the provider's token is only a snapshot of whatever principal the
// environment resolved to at that moment, and every credential derived from it inherits
// that snapshot. Reusing a persisted entry would replay a stale principal after the
// ambient identity changes.
func (m *manager) chainRootIsAmbient() bool {
	if len(m.chain) == 0 {
		return false
	}
	provider, exists := m.providers[m.chain[0]]
	if !exists {
		return false
	}
	return types.ProviderIsAmbient(provider)
}

// chainRootIsStandalone reports whether the root of the current chain is a standalone identity
// rather than a registered provider. Examples are aws/user, aws/credential-process, and
// emulator-bound identities.
//
// A standalone root owns its credential caching inside AuthenticateStandalone: aws/credential-process
// reuses unexpired file credentials or re-runs its helper. Its stored credentials must therefore
// never be fed to the next chain step as if they were a valid cached starting point, because helper
// credentials that carry no expiration would otherwise be reused forever. The exception is aws/user,
// whose cached session is revalidated by isChainCredentialValid, which rejects long-lived IAM keys
// so the MFA session is never bypassed.
func (m *manager) chainRootIsStandalone() bool {
	if len(m.chain) == 0 {
		return false
	}
	rootName := m.chain[0]
	if _, isProvider := m.providers[rootName]; isProvider {
		return false
	}
	if m.config != nil && m.config.Identities[rootName].Kind == types.IdentityKindAWSUser {
		return false
	}
	standalone, ok := m.identities[rootName].(types.StandaloneIdentity)
	return ok && standalone.IsStandalone()
}

// chainRootHasPrivateCache forces configuration-bound session validation before cache reuse.
func (m *manager) chainRootHasPrivateCache() bool {
	if len(m.chain) == 0 {
		return false
	}
	return types.UsesPrivateCredentialCache(m.providers[m.chain[0]])
}
