package auth

import (
	"context"
	"fmt"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
)

// authenticateChainRoot authenticates a standalone identity or a provider before
// passing its credentials to downstream identities. Standalone identities own
// session persistence, so their long-lived keyring credentials remain untouched.
func (m *manager) authenticateChainRoot(ctx context.Context) (types.ICredentials, error) {
	root := m.chain[0]
	if standalone, ok := m.identities[root].(types.StandaloneIdentity); ok && standalone.IsStandalone() {
		return standalone.AuthenticateStandalone(ctx)
	}

	// Allow providers to inspect the chain and prepare pre-auth preferences.
	if provider, exists := m.providers[root]; exists {
		if err := provider.PreAuthenticate(m); err != nil {
			errUtils.CheckErrorAndPrint(err, "Pre Authenticate", "")
			return nil, fmt.Errorf("%w: provider=%s: %w", errUtils.ErrAuthenticationFailed, root, err)
		}
	}
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
