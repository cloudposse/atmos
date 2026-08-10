package emulator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNew_NilConfig(t *testing.T) {
	_, err := New("x", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidAuthConfig)
}

func TestSetNameAndSetConfig(t *testing.T) {
	id := newAWSIdentity(t)

	id.SetName("renamed")
	assert.Equal(t, "renamed", id.Name())

	id.SetConfig(&schema.Identity{Kind: types.IdentityKindKubernetesEmulator, Emulator: "k3s"})
	assert.Equal(t, types.IdentityKindKubernetesEmulator, id.Kind())
}

func TestKindAndName_NilConfigFallback(t *testing.T) {
	// An identity with no config reports empty kind, and Name falls back to the kind.
	id := &Identity{}
	assert.Empty(t, id.Kind())
	assert.Empty(t, id.Name(), "Name falls back to the (empty) kind when unnamed")

	// With a kind but no name, Name falls back to the kind.
	id.SetConfig(&schema.Identity{Kind: types.IdentityKindAWSEmulator, Emulator: "aws"})
	assert.Equal(t, types.IdentityKindAWSEmulator, id.Name())
}

func TestEnvironmentAndPaths_Empty(t *testing.T) {
	id := newAWSIdentity(t)

	env, err := id.Environment()
	require.NoError(t, err)
	assert.Empty(t, env, "emulator identities expose no static environment")

	paths, err := id.Paths()
	require.NoError(t, err)
	assert.Empty(t, paths, "emulator identities expose no static credential paths")
}

func TestCredentialsExistAndLoad(t *testing.T) {
	id := newAWSIdentity(t)

	exists, err := id.CredentialsExist()
	require.NoError(t, err)
	assert.True(t, exists, "the emulator is the live credential source")

	creds, err := id.LoadCredentials(context.Background())
	require.NoError(t, err)
	assert.Nil(t, creds, "emulator identities have no stored credentials")
}

func TestValidate_NilConfigAndBadKind(t *testing.T) {
	nilCfg := &Identity{name: "id"}
	require.ErrorIs(t, nilCfg.Validate(), errUtils.ErrInvalidIdentityConfig)

	badKind := &Identity{name: "id", config: &schema.Identity{Kind: "aws/permission-set", Emulator: "aws"}}
	require.ErrorIs(t, badKind.Validate(), errUtils.ErrInvalidIdentityKind)
}

func TestLogout_CloudTargetNoOp(t *testing.T) {
	// A cloud emulator with no harvested kubeconfig: Logout removes nothing and
	// succeeds (the path simply does not exist).
	id := newAWSIdentity(t)
	id.SetRealm("test-realm")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	require.NoError(t, id.Logout(context.Background()))
}
