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
var _ = schema.Identity{Kind: types.IdentityKindAzureEmulator, Emulator: "azure"}

// TestPostAuthenticate_AzurePopulatesAuthContext verifies an azure/emulator identity
// fills AuthContext.Azure with the Azurite storage connection string that the in-process
// azurerm state-backend reader uses to reach the emulator — the Azure analog of the AWS
// endpoint.
func TestPostAuthenticate_AzurePopulatesAuthContext(t *testing.T) {
	const connStr = "DefaultEndpointsProtocol=http;AccountName=devstoreaccount1;" +
		"AccountKey=key;BlobEndpoint=http://localhost:10000/devstoreaccount1;"

	id, err := New("local-azure", &schema.Identity{Kind: types.IdentityKindAzureEmulator, Emulator: "azure"})
	require.NoError(t, err)
	id.SetEmulatorResolver(&fakeResolver{env: map[string]string{
		"AZURE_STORAGE_CONNECTION_STRING": connStr,
	}})

	ac := &schema.AuthContext{}
	require.NoError(t, id.PostAuthenticate(context.Background(), &types.PostAuthenticateParams{
		AuthContext:  ac,
		StackInfo:    &schema.ConfigAndStacksInfo{Stack: "dev"},
		ProviderName: "local-azure",
		IdentityName: "local-azure",
	}))

	require.NotNil(t, ac.Azure)
	assert.Equal(t, connStr, ac.Azure.StorageConnectionString)
	assert.Equal(t, "local-azure", ac.Azure.Profile)
	// The Azure target must not bleed into the other clouds' contexts.
	assert.Nil(t, ac.AWS)
	assert.Nil(t, ac.GCP)
}

// TestPostAuthenticate_AzureSkipsWhenNoStack verifies that with no resolvable stack the
// Azure auth context is left unpopulated (best-effort) rather than failing the auth flow.
func TestPostAuthenticate_AzureSkipsWhenNoStack(t *testing.T) {
	id, err := New("local-azure", &schema.Identity{Kind: types.IdentityKindAzureEmulator, Emulator: "azure"})
	require.NoError(t, err)
	id.SetEmulatorResolver(&fakeResolver{env: map[string]string{"AZURE_STORAGE_CONNECTION_STRING": "x"}})

	ac := &schema.AuthContext{}
	require.NoError(t, id.PostAuthenticate(context.Background(), &types.PostAuthenticateParams{
		AuthContext: ac, IdentityName: "local-azure",
	}))
	assert.Nil(t, ac.Azure, "no stack -> Azure auth context left unpopulated")
}
