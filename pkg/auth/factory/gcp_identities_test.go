package factory

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/types"
)

func TestRegisterGCPIdentities(t *testing.T) {
	f := NewFactory()

	// Verify gcp/service-account is registered.
	assert.True(t, f.HasIdentity(types.IdentityKindGCPServiceAccount))

	// Verify gcp/project is registered.
	assert.True(t, f.HasIdentity(types.IdentityKindGCPProject))
}

func TestCreateGCPServiceAccountIdentity(t *testing.T) {
	f := NewFactory()

	principal := map[string]any{
		"service_account_email": "deployer@my-project.iam.gserviceaccount.com",
		"scopes": []any{
			"https://www.googleapis.com/auth/cloud-platform",
		},
	}

	identity, err := f.CreateIdentity(types.IdentityKindGCPServiceAccount, "prod-deployer", principal)
	require.NoError(t, err)
	require.NotNil(t, identity)

	assert.Equal(t, types.IdentityKindGCPServiceAccount, identity.Kind())
}

func TestCreateGCPServiceAccountIdentity_MissingEmail(t *testing.T) {
	f := NewFactory()

	principal := map[string]any{
		"scopes": []any{"scope1"},
	}

	identity, err := f.CreateIdentity(types.IdentityKindGCPServiceAccount, "bad-sa", principal)
	// Parser returns error for missing service_account_email; or identity fails Validate().
	if err != nil {
		assert.Contains(t, err.Error(), "service_account_email")
		return
	}
	require.NotNil(t, identity)
	err = identity.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "service_account_email")
}

func TestCreateGCPProjectIdentity(t *testing.T) {
	f := NewFactory()

	principal := map[string]any{
		"project_id": "my-project",
		"region":     "us-west1",
		"zone":       "us-west1-a",
	}

	identity, err := f.CreateIdentity(types.IdentityKindGCPProject, "prod-context", principal)
	require.NoError(t, err)
	require.NotNil(t, identity)

	assert.Equal(t, types.IdentityKindGCPProject, identity.Kind())
}

func TestCreateGCPProjectIdentity_MissingProjectID(t *testing.T) {
	f := NewFactory()

	principal := map[string]any{
		"region": "us-central1",
	}

	identity, err := f.CreateIdentity(types.IdentityKindGCPProject, "bad-project", principal)
	// Parser returns error for missing project_id; or identity fails Validate().
	if err != nil {
		assert.Contains(t, err.Error(), "project_id")
		return
	}
	require.NotNil(t, identity)
	err = identity.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "project_id")
}

func TestCreateGCPProjectIdentity_NoProvider(t *testing.T) {
	f := NewFactory()

	principal := map[string]any{
		"project_id": "standalone-project",
	}

	identity, err := f.CreateIdentity(types.IdentityKindGCPProject, "standalone", principal)
	require.NoError(t, err)

	// Verify it doesn't require a provider.
	providerName, err := identity.GetProviderName()
	require.NoError(t, err)
	assert.Empty(t, providerName)
}
