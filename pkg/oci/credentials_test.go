package oci_test

import (
	"testing"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/keyring"
	"github.com/cloudposse/atmos/pkg/oci"
)

func TestOptionalSystemKeyringDoesNotBlockPublicRegistry(t *testing.T) {
	t.Parallel()
	resource, err := name.NewRepository("registry.example.com/team/plugin")
	require.NoError(t, err)
	unavailable := func() (keyring.Keyring, error) { return nil, keyring.ErrUnavailable }
	optional := oci.CredentialKeychain{Open: unavailable, Optional: true}
	auth, err := optional.Resolve(resource)
	require.NoError(t, err)
	require.Equal(t, authn.Anonymous, auth)
	required := oci.CredentialKeychain{Open: unavailable}
	_, err = required.Resolve(resource)
	require.ErrorIs(t, err, keyring.ErrUnavailable)
	// An unavailable optional backend is different from an encrypted store needing a password.
	optional.Open = func() (keyring.Keyring, error) { return nil, keyring.ErrPasswordRequired }
	_, err = optional.Resolve(resource)
	require.ErrorIs(t, err, keyring.ErrPasswordRequired)
}
