package atmospro

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewIdentity_RequiresViaProvider(t *testing.T) {
	_, err := NewIdentity("atmos-pro", &schema.Identity{Kind: IdentityKind})
	require.ErrorIs(t, err, errUtils.ErrInvalidIdentityConfig)

	_, err = NewIdentity("atmos-pro", &schema.Identity{Kind: IdentityKind, Via: &schema.IdentityVia{Identity: "x"}})
	require.ErrorIs(t, err, errUtils.ErrInvalidIdentityConfig)

	id, err := NewIdentity("atmos-pro", &schema.Identity{Kind: IdentityKind, Via: &schema.IdentityVia{Provider: "atmos-pro"}})
	require.NoError(t, err)
	assert.Equal(t, "atmos/pro", id.Kind())
}

func TestIdentity_PassthroughAuthenticate(t *testing.T) {
	id, err := NewIdentity("atmos-pro", &schema.Identity{Kind: IdentityKind, Via: &schema.IdentityVia{Provider: "atmos-pro"}})
	require.NoError(t, err)

	creds := &types.ProCredentials{Token: "session-jwt", BaseURL: "https://pro", WorkspaceID: "ws"}
	out, err := id.Authenticate(context.Background(), creds)
	require.NoError(t, err)
	assert.Same(t, creds, out, "identity must pass the provider credentials through unchanged")

	provider, err := id.GetProviderName()
	require.NoError(t, err)
	assert.Equal(t, "atmos-pro", provider)
}

func TestIdentity_EnvironmentEmptyAndPassthrough(t *testing.T) {
	id, err := NewIdentity("atmos-pro", &schema.Identity{Kind: IdentityKind, Via: &schema.IdentityVia{Provider: "atmos-pro"}})
	require.NoError(t, err)

	env, err := id.Environment()
	require.NoError(t, err)
	assert.Empty(t, env)

	in := map[string]string{"FOO": "bar"}
	out, err := id.PrepareEnvironment(context.Background(), in)
	require.NoError(t, err)
	assert.Equal(t, in, out)
	// Must be a copy, not the same map.
	out["FOO"] = "mutated"
	assert.Equal(t, "bar", in["FOO"], "PrepareEnvironment must not mutate the input map")
}
