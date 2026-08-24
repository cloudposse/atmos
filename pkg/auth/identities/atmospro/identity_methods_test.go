package atmospro

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewIdentity_Errors(t *testing.T) {
	_, err := NewIdentity("x", nil)
	require.ErrorIs(t, err, errUtils.ErrInvalidAuthConfig)

	_, err = NewIdentity("x", &schema.Identity{Kind: "wrong", Via: &schema.IdentityVia{Provider: "atmos-pro"}})
	require.ErrorIs(t, err, errUtils.ErrInvalidIdentityKind)
}

func TestIdentity_TrivialMethods(t *testing.T) {
	id, err := NewIdentity("atmos-pro", &schema.Identity{Kind: IdentityKind, Via: &schema.IdentityVia{Provider: "atmos-pro"}})
	require.NoError(t, err)

	require.NoError(t, id.Validate())
	assert.Equal(t, IdentityKind, id.Kind())

	paths, err := id.Paths()
	require.NoError(t, err)
	assert.Empty(t, paths)

	require.NoError(t, id.PostAuthenticate(context.Background(), nil))
	require.NoError(t, id.Logout(context.Background()))

	exists, err := id.CredentialsExist()
	require.NoError(t, err)
	assert.False(t, exists)

	creds, err := id.LoadCredentials(context.Background())
	require.NoError(t, err)
	assert.Nil(t, creds)

	// SetRealm is not on the interface; call it on the concrete type.
	id.(*proIdentity).SetRealm("realmA")
}

// TestIdentity_ConfigErrorBranches exercises the via-missing branches in Validate and
// GetProviderName by constructing the identity directly (bypassing the NewIdentity guard).
func TestIdentity_ConfigErrorBranches(t *testing.T) {
	id := &proIdentity{name: "x", config: &schema.Identity{Kind: IdentityKind}}

	require.ErrorIs(t, id.Validate(), errUtils.ErrInvalidIdentityConfig)

	_, err := id.GetProviderName()
	require.ErrorIs(t, err, errUtils.ErrInvalidIdentityConfig)
}
