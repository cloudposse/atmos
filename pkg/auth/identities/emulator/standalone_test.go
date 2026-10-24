package emulator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestIsStandalone verifies emulator-bound identities report as standalone: they
// authenticate without an upstream provider step (the container is the source).
func TestIsStandalone(t *testing.T) {
	id := newAWSIdentity(t)
	assert.True(t, id.IsStandalone(), "emulator identities are standalone")
}

// TestAuthenticateStandalone_NoOpReturnsNilCredentials verifies that standalone
// authentication mints no credentials — the connection profile is injected at
// environment-preparation time, so the call is a no-op that returns nil creds.
func TestAuthenticateStandalone_NoOpReturnsNilCredentials(t *testing.T) {
	id := newAWSIdentity(t)

	creds, err := id.AuthenticateStandalone(context.Background())
	require.NoError(t, err)
	assert.Nil(t, creds, "standalone emulator auth mints no credentials")
}

// TestAuthenticateStandalone_SatisfiesStandaloneInterface verifies the concrete
// Identity is assignable to the types.StandaloneIdentity interface the chain
// manager dispatches through, and both interface methods round-trip.
func TestAuthenticateStandalone_SatisfiesStandaloneInterface(t *testing.T) {
	var standalone types.StandaloneIdentity = newAWSIdentity(t)

	assert.True(t, standalone.IsStandalone())

	creds, err := standalone.AuthenticateStandalone(context.Background())
	require.NoError(t, err)
	assert.Nil(t, creds)
}

// TestAuthenticateStandalone_KubernetesEmulator verifies standalone auth behaves
// identically for a non-AWS emulator target.
func TestAuthenticateStandalone_KubernetesEmulator(t *testing.T) {
	id, err := New("local-k8s", &schema.Identity{Kind: types.IdentityKindKubernetesEmulator, Emulator: "k3s"})
	require.NoError(t, err)

	creds, err := id.AuthenticateStandalone(context.Background())
	require.NoError(t, err)
	assert.Nil(t, creds, "kubernetes emulator standalone auth mints no credentials")
}
