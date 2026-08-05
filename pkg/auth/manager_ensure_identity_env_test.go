package auth

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/integrations"
	"github.com/cloudposse/atmos/pkg/auth/validation"
	"github.com/cloudposse/atmos/pkg/schema"
)

// newProManager builds a fully-initialized manager with an atmos/pro provider + identity and a
// github/sts integration, backed by a fresh in-test credential store.
func newProManager(t *testing.T) *manager {
	t.Helper()

	cfg := &schema.AuthConfig{
		Realm: "test-realm",
		Providers: map[string]schema.Provider{
			"atmos-pro": {Kind: "atmos/pro"},
		},
		Identities: map[string]schema.Identity{
			"atmos-pro": {Kind: "atmos/pro", Via: &schema.IdentityVia{Provider: "atmos-pro"}},
		},
		Integrations: map[string]schema.Integration{
			"github-sts": {Kind: integrations.KindGitHubSTS, Via: &schema.IntegrationVia{Provider: "atmos-pro"}},
		},
	}

	m, err := NewAuthManager(cfg, credentials.NewCredentialStore(), validation.NewValidator(), nil, "")
	require.NoError(t, err)
	return m.(*manager)
}

func TestEnsureIdentityEnvironment_EmptyIdentity(t *testing.T) {
	m := newProManager(t)

	_, err := m.EnsureIdentityEnvironment(context.Background(), "")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrNilParam)
}

func TestEnsureIdentityEnvironment_AuthenticateFailsWhenNotProvisionable(t *testing.T) {
	// Ensure the provider cannot silently authenticate from ambient env: clear the workspace ID
	// so the atmos/pro provider fails fast (before any network) with a deterministic error.
	t.Setenv("ATMOS_PRO_WORKSPACE_ID", "")
	t.Setenv("ATMOS_XDG_DATA_HOME", t.TempDir())

	m := newProManager(t)

	// No cached credentials exist, so EnsureIdentityEnvironment falls through to Authenticate,
	// which fails because the atmos/pro provider has no workspace ID. The error is wrapped as
	// an identity-auth failure rather than panicking or returning a partial environment.
	_, err := m.EnsureIdentityEnvironment(context.Background(), "atmos-pro")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrIdentityAuthFailed)
}
