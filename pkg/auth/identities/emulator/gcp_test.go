package emulator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinel: a kind rename must immediately fail the build.
var _ = schema.Identity{Kind: types.IdentityKindGCPEmulator, Emulator: "gcp"}

// TestPostAuthenticate_GCPPopulatesAuthContext verifies a gcp/emulator identity fills
// AuthContext.GCP with the storage emulator endpoint (STORAGE_EMULATOR_HOST) that the
// in-process GCS state-backend reader relies on — the GCP analog of the AWS endpoint.
func TestPostAuthenticate_GCPPopulatesAuthContext(t *testing.T) {
	id, err := New("local-gcp", &schema.Identity{Kind: types.IdentityKindGCPEmulator, Emulator: "gcp"})
	require.NoError(t, err)
	id.SetEmulatorResolver(&fakeResolver{env: map[string]string{
		"STORAGE_EMULATOR_HOST":             "http://localhost:34568",
		"GOOGLE_CLOUD_PROJECT":              "test-project",
		"CLOUDSDK_AUTH_DISABLE_CREDENTIALS": "true",
	}})

	ac := &schema.AuthContext{}
	require.NoError(t, id.PostAuthenticate(context.Background(), &types.PostAuthenticateParams{
		AuthContext:  ac,
		StackInfo:    &schema.ConfigAndStacksInfo{Stack: "dev"},
		ProviderName: "local-gcp",
		IdentityName: "local-gcp",
	}))

	require.NotNil(t, ac.GCP)
	assert.Equal(t, "http://localhost:34568", ac.GCP.StorageEmulatorHost)
	assert.Equal(t, "test-project", ac.GCP.ProjectID)
	assert.True(t, ac.GCP.WithoutAuthentication)
	// The GCP target must not bleed into the other clouds' contexts.
	assert.Nil(t, ac.AWS)
	assert.Nil(t, ac.Azure)
}

// TestPostAuthenticate_GCPProjectFallback verifies CLOUDSDK_CORE_PROJECT is used when
// GOOGLE_CLOUD_PROJECT is absent, and WithoutAuthentication defaults to false.
func TestPostAuthenticate_GCPProjectFallback(t *testing.T) {
	id, err := New("local-gcp", &schema.Identity{Kind: types.IdentityKindGCPEmulator, Emulator: "gcp"})
	require.NoError(t, err)
	id.SetEmulatorResolver(&fakeResolver{env: map[string]string{
		"CLOUDSDK_CORE_PROJECT": "core-project",
		"STORAGE_EMULATOR_HOST": "http://localhost:1",
	}})

	ac := &schema.AuthContext{}
	require.NoError(t, id.PostAuthenticate(context.Background(), &types.PostAuthenticateParams{
		AuthContext: ac, StackInfo: &schema.ConfigAndStacksInfo{Stack: "dev"}, IdentityName: "local-gcp",
	}))
	require.NotNil(t, ac.GCP)
	assert.Equal(t, "core-project", ac.GCP.ProjectID)
	assert.False(t, ac.GCP.WithoutAuthentication, "absent CLOUDSDK_AUTH_DISABLE_CREDENTIALS -> false")
}

// TestPostAuthenticate_GCPSkipsWhenNoStack verifies that with no resolvable stack the
// GCP auth context is left unpopulated (best-effort) rather than failing the auth flow.
func TestPostAuthenticate_GCPSkipsWhenNoStack(t *testing.T) {
	id, err := New("local-gcp", &schema.Identity{Kind: types.IdentityKindGCPEmulator, Emulator: "gcp"})
	require.NoError(t, err)
	id.SetEmulatorResolver(&fakeResolver{env: map[string]string{"STORAGE_EMULATOR_HOST": "x"}})

	ac := &schema.AuthContext{}
	require.NoError(t, id.PostAuthenticate(context.Background(), &types.PostAuthenticateParams{
		AuthContext: ac, IdentityName: "local-gcp",
	}))
	assert.Nil(t, ac.GCP, "no stack -> GCP auth context left unpopulated")
}
