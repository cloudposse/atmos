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
