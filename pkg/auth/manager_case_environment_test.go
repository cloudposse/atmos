package auth

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinel for the runtime case map these tests rely on.
var _ = schema.AuthConfig{IdentityCaseMap: map[string]string{}}

// caseManager returns a manager whose identity is configured as "Dev-Admin". As in a
// loaded atmos.yaml, the identity maps are keyed by the lower-cased name and
// IdentityCaseMap records the original spelling.
func caseManager() *manager {
	identity := &mockIdentityForShellEnv{
		prepareEnvFunc: func(_ context.Context, environ map[string]string) (map[string]string, error) {
			out := map[string]string{"AWS_PROFILE": "dev-admin"}
			for k, v := range environ {
				out[k] = v
			}
			return out, nil
		},
	}
	return &manager{
		config: &schema.AuthConfig{
			Providers: map[string]schema.Provider{"sso": {Kind: "aws/iam-identity-center"}},
			Identities: map[string]schema.Identity{
				"dev-admin": {Kind: "aws/permission-set", Via: &schema.IdentityVia{Provider: "sso"}},
			},
			IdentityCaseMap: map[string]string{"dev-admin": "Dev-Admin"},
		},
		identities: map[string]types.Identity{"dev-admin": identity},
	}
}

// TestPrepareShellEnvironment_CaseInsensitiveIdentity is the regression for `auth
// exec`, `auth shell`, and custom-command `identity:` failing with "identity not
// found" for a differently-cased identity name that `auth login` accepts.
func TestPrepareShellEnvironment_CaseInsensitiveIdentity(t *testing.T) {
	for _, name := range []string{"Dev-Admin", "DEV-ADMIN", "dev-admin"} {
		t.Run(name, func(t *testing.T) {
			env, err := caseManager().PrepareShellEnvironment(context.Background(), name, []string{"PATH=/usr/bin"})
			require.NoError(t, err)
			got := environListToMap(env)
			assert.Equal(t, "dev-admin", got["AWS_PROFILE"])
			assert.Equal(t, "/usr/bin", got["PATH"])
		})
	}
}

// TestGetEnvironmentVariables_CaseInsensitiveIdentity is the regression for `auth env`
// with a differently-cased identity name.
func TestGetEnvironmentVariables_CaseInsensitiveIdentity(t *testing.T) {
	env, err := caseManager().GetEnvironmentVariables("Dev-Admin")
	require.NoError(t, err)
	assert.NotNil(t, env)
}

// TestGetProviderForIdentity_CaseInsensitiveIdentity verifies the provider shown by
// `auth shell` resolves for a differently-cased identity name.
func TestGetProviderForIdentity_CaseInsensitiveIdentity(t *testing.T) {
	m := caseManager()
	assert.Equal(t, "sso", m.GetProviderForIdentity("Dev-Admin"))
	assert.Equal(t, "sso", m.GetProviderForIdentity("dev-admin"))
	assert.Empty(t, m.GetProviderForIdentity("someone-else"))
}

// TestCaseInsensitiveIdentity_UnknownNameStillFails is the negative path: a name that
// matches no identity in any case is still reported as not found.
func TestCaseInsensitiveIdentity_UnknownNameStillFails(t *testing.T) {
	m := caseManager()

	_, err := m.PrepareShellEnvironment(context.Background(), "Prod-Admin", nil)
	require.ErrorIs(t, err, errUtils.ErrIdentityNotFound)

	_, err = m.GetEnvironmentVariables("Prod-Admin")
	require.ErrorIs(t, err, errUtils.ErrIdentityNotFound)
}

// TestResolveIdentityName_NilConfig verifies case-insensitive resolution tolerates a
// manager without a config (for example one built for a single identity).
func TestResolveIdentityName_NilConfig(t *testing.T) {
	m := &manager{identities: map[string]types.Identity{"dev": nil}}
	resolved, found := m.resolveIdentityName("dev")
	assert.True(t, found)
	assert.Equal(t, "dev", resolved)

	_, found = m.resolveIdentityName("Dev")
	assert.False(t, found)
}
