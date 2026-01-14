package aws

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestPermissionSetIdentity_Environment_NoProvider(t *testing.T) {
	// Test environment variables when provider name cannot be resolved.
	identity, err := NewPermissionSetIdentity("test-ps", &schema.Identity{
		Kind: "aws/permission-set",
		Principal: map[string]interface{}{
			"name":    "DevAccess",
			"account": map[string]interface{}{"id": "123456789012"},
		},
		Env: []schema.EnvironmentVariable{
			{Key: "CUSTOM_VAR", Value: "custom_value"},
		},
	})
	require.NoError(t, err)

	// Without manager, resolveRootProviderName should fail.
	// Environment should still return custom env vars.
	env, err := identity.Environment()
	assert.NoError(t, err)
	assert.Equal(t, "custom_value", env["CUSTOM_VAR"])
}

func TestPermissionSetIdentity_PrepareEnvironment_NoProvider(t *testing.T) {
	identity, err := NewPermissionSetIdentity("test-ps", &schema.Identity{
		Kind: "aws/permission-set",
		Principal: map[string]interface{}{
			"name":    "DevAccess",
			"account": map[string]interface{}{"id": "123456789012"},
		},
	})
	require.NoError(t, err)

	// Without manager, resolveRootProviderName should fail.
	environ := map[string]string{"TEST": "value"}
	_, err = identity.PrepareEnvironment(context.Background(), environ)
	assert.Error(t, err)
}

func TestPermissionSetIdentity_ResolveRootProviderName_NoManager(t *testing.T) {
	identity, err := NewPermissionSetIdentity("test-ps", &schema.Identity{
		Kind: "aws/permission-set",
		Principal: map[string]interface{}{
			"name":    "DevAccess",
			"account": map[string]interface{}{"id": "123456789012"},
		},
	})
	require.NoError(t, err)

	psIdentity := identity.(*permissionSetIdentity)

	// Without manager or cached root provider name, should return error.
	_, err = psIdentity.resolveRootProviderName()
	assert.Error(t, err)
}

func TestPermissionSetIdentity_ResolveRootProviderName_WithCachedProvider(t *testing.T) {
	identity, err := NewPermissionSetIdentity("test-ps", &schema.Identity{
		Kind: "aws/permission-set",
		Principal: map[string]interface{}{
			"name":    "DevAccess",
			"account": map[string]interface{}{"id": "123456789012"},
		},
	})
	require.NoError(t, err)

	psIdentity := identity.(*permissionSetIdentity)
	psIdentity.rootProviderName = "aws-sso"

	// With cached root provider name, should succeed.
	providerName, err := psIdentity.resolveRootProviderName()
	assert.NoError(t, err)
	assert.Equal(t, "aws-sso", providerName)
}

func TestPermissionSetIdentity_GetRootProviderFromVia_Cached(t *testing.T) {
	identity, err := NewPermissionSetIdentity("test-ps", &schema.Identity{
		Kind: "aws/permission-set",
		Principal: map[string]interface{}{
			"name":    "DevAccess",
			"account": map[string]interface{}{"id": "123456789012"},
		},
	})
	require.NoError(t, err)

	psIdentity := identity.(*permissionSetIdentity)
	psIdentity.rootProviderName = "aws-sso"

	// With cached value, should return it.
	providerName, err := psIdentity.getRootProviderFromVia()
	assert.NoError(t, err)
	assert.Equal(t, "aws-sso", providerName)
}

func TestPermissionSetIdentity_GetRootProviderFromVia_NoCached(t *testing.T) {
	identity, err := NewPermissionSetIdentity("test-ps", &schema.Identity{
		Kind: "aws/permission-set",
		Principal: map[string]interface{}{
			"name":    "DevAccess",
			"account": map[string]interface{}{"id": "123456789012"},
		},
	})
	require.NoError(t, err)

	psIdentity := identity.(*permissionSetIdentity)

	// Without cached value, should return error.
	_, err = psIdentity.getRootProviderFromVia()
	assert.Error(t, err)
}

func TestPermissionSetIdentity_SetManagerAndProvider(t *testing.T) {
	identity, err := NewPermissionSetIdentity("test-ps", &schema.Identity{
		Kind: "aws/permission-set",
		Principal: map[string]interface{}{
			"name":    "DevAccess",
			"account": map[string]interface{}{"id": "123456789012"},
		},
	})
	require.NoError(t, err)

	psIdentity := identity.(*permissionSetIdentity)

	// Set manager and provider name.
	psIdentity.SetManagerAndProvider(nil, "aws-sso")

	// Should be able to resolve root provider name now.
	assert.Equal(t, "aws-sso", psIdentity.rootProviderName)
}

func TestPermissionSetIdentity_PostAuthenticate_NilParams(t *testing.T) {
	identity, err := NewPermissionSetIdentity("test-ps", &schema.Identity{
		Kind: "aws/permission-set",
		Principal: map[string]interface{}{
			"name":    "DevAccess",
			"account": map[string]interface{}{"id": "123456789012"},
		},
	})
	require.NoError(t, err)

	// Nil parameters should return error.
	err = identity.PostAuthenticate(context.Background(), nil)
	assert.Error(t, err)
}

func TestPermissionSetIdentity_PostAuthenticate_NilCredentials(t *testing.T) {
	identity, err := NewPermissionSetIdentity("test-ps", &schema.Identity{
		Kind: "aws/permission-set",
		Principal: map[string]interface{}{
			"name":    "DevAccess",
			"account": map[string]interface{}{"id": "123456789012"},
		},
	})
	require.NoError(t, err)

	params := &types.PostAuthenticateParams{
		Credentials: nil,
	}

	// Nil credentials should return error.
	err = identity.PostAuthenticate(context.Background(), params)
	assert.Error(t, err)
}

func TestPermissionSetIdentity_CredentialsExist_NoProvider(t *testing.T) {
	identity, err := NewPermissionSetIdentity("test-ps", &schema.Identity{
		Kind: "aws/permission-set",
		Principal: map[string]interface{}{
			"name":    "DevAccess",
			"account": map[string]interface{}{"id": "123456789012"},
		},
	})
	require.NoError(t, err)

	// Without provider name, credentials don't exist yet.
	exists, err := identity.CredentialsExist()
	assert.NoError(t, err)
	assert.False(t, exists)
}

func TestPermissionSetIdentity_CredentialsExist_EmptySection(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ATMOS_XDG_CONFIG_HOME", tmpDir)

	identity, err := NewPermissionSetIdentity("test-ps", &schema.Identity{
		Kind: "aws/permission-set",
		Via:  &schema.IdentityVia{Provider: "aws-sso"},
		Principal: map[string]interface{}{
			"name":    "DevAccess",
			"account": map[string]interface{}{"id": "123456789012"},
		},
	})
	require.NoError(t, err)

	psIdentity := identity.(*permissionSetIdentity)
	psIdentity.SetManagerAndProvider(nil, "aws-sso")

	// Create credentials file with empty section.
	credPath := filepath.Join(tmpDir, "atmos", "aws", "aws-sso", "credentials")
	require.NoError(t, os.MkdirAll(filepath.Dir(credPath), 0o700))
	require.NoError(t, os.WriteFile(credPath, []byte("[test-ps]\n"), 0o600))

	// Empty section (no aws_access_key_id) should return false.
	exists, err := identity.CredentialsExist()
	assert.NoError(t, err)
	assert.False(t, exists)
}

func TestPermissionSetIdentity_LoadCredentials_NoProvider(t *testing.T) {
	identity, err := NewPermissionSetIdentity("test-ps", &schema.Identity{
		Kind: "aws/permission-set",
		Principal: map[string]interface{}{
			"name":    "DevAccess",
			"account": map[string]interface{}{"id": "123456789012"},
		},
	})
	require.NoError(t, err)

	// Without provider name, should fail to get environment variables.
	_, err = identity.LoadCredentials(context.Background())
	assert.Error(t, err)
}

func TestPermissionSetIdentity_Logout_NoProvider(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ATMOS_XDG_CONFIG_HOME", tmpDir)

	identity, err := NewPermissionSetIdentity("test-ps", &schema.Identity{
		Kind: "aws/permission-set",
		Via:  &schema.IdentityVia{Provider: "aws-sso"},
		Principal: map[string]interface{}{
			"name":    "DevAccess",
			"account": map[string]interface{}{"id": "123456789012"},
		},
	})
	require.NoError(t, err)

	// Without setting rootProviderName, logout should still work (uses empty string).
	err = identity.Logout(context.Background())
	assert.NoError(t, err)
}

func TestPermissionSetIdentity_Logout_WithFiles(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ATMOS_XDG_CONFIG_HOME", tmpDir)

	identity, err := NewPermissionSetIdentity("test-ps", &schema.Identity{
		Kind: "aws/permission-set",
		Via:  &schema.IdentityVia{Provider: "aws-sso"},
		Principal: map[string]interface{}{
			"name":    "DevAccess",
			"account": map[string]interface{}{"id": "123456789012"},
		},
	})
	require.NoError(t, err)

	psIdentity := identity.(*permissionSetIdentity)
	psIdentity.SetManagerAndProvider(nil, "aws-sso")

	// Create credentials file with test-ps section.
	credPath := filepath.Join(tmpDir, "atmos", "aws", "aws-sso", "credentials")
	require.NoError(t, os.MkdirAll(filepath.Dir(credPath), 0o700))
	credContent := `[test-ps]
aws_access_key_id = AKIAIOSFODNN7EXAMPLE
aws_secret_access_key = secret
`
	require.NoError(t, os.WriteFile(credPath, []byte(credContent), 0o600))

	// Logout should remove the section.
	err = identity.Logout(context.Background())
	assert.NoError(t, err)

	// Verify credentials no longer exist.
	exists, err := identity.CredentialsExist()
	assert.NoError(t, err)
	assert.False(t, exists)
}

func TestPermissionSetIdentity_Validate_EmptyPermissionSetName(t *testing.T) {
	identity, err := NewPermissionSetIdentity("test-ps", &schema.Identity{
		Kind: "aws/permission-set",
		Principal: map[string]interface{}{
			"name":    "", // Empty name.
			"account": map[string]interface{}{"id": "123456789012"},
		},
	})
	require.NoError(t, err)

	err = identity.Validate()
	assert.Error(t, err)
}

func TestPermissionSetIdentity_Validate_BothAccountNameAndID(t *testing.T) {
	identity, err := NewPermissionSetIdentity("test-ps", &schema.Identity{
		Kind: "aws/permission-set",
		Principal: map[string]interface{}{
			"name": "DevAccess",
			"account": map[string]interface{}{
				"name": "production",
				"id":   "123456789012",
			},
		},
	})
	require.NoError(t, err)

	// Both name and ID provided should be valid.
	err = identity.Validate()
	assert.NoError(t, err)
}

func TestPermissionSetIdentity_Validate_EmptyAccountNameAndID(t *testing.T) {
	identity, err := NewPermissionSetIdentity("test-ps", &schema.Identity{
		Kind: "aws/permission-set",
		Principal: map[string]interface{}{
			"name": "DevAccess",
			"account": map[string]interface{}{
				"name": "",
				"id":   "",
			},
		},
	})
	require.NoError(t, err)

	err = identity.Validate()
	assert.Error(t, err)
}

func TestPermissionSetIdentity_Authenticate_InvalidBaseCreds(t *testing.T) {
	identity, err := NewPermissionSetIdentity("test-ps", &schema.Identity{
		Kind: "aws/permission-set",
		Principal: map[string]interface{}{
			"name":    "DevAccess",
			"account": map[string]interface{}{"id": "123456789012"},
		},
	})
	require.NoError(t, err)

	// Non-AWS credentials should fail.
	type invalidCreds struct {
		types.ICredentials
	}
	_, err = identity.Authenticate(context.Background(), &invalidCreds{})
	assert.Error(t, err)
}

func TestPermissionSetIdentity_GetAccountDetails_BothProvided(t *testing.T) {
	identity, err := NewPermissionSetIdentity("test-ps", &schema.Identity{
		Kind: "aws/permission-set",
		Principal: map[string]interface{}{
			"name": "DevAccess",
			"account": map[string]interface{}{
				"name": "production",
				"id":   "123456789012",
			},
		},
	})
	require.NoError(t, err)

	psIdentity := identity.(*permissionSetIdentity)
	name, id, err := psIdentity.getAccountDetails()
	assert.NoError(t, err)
	assert.Equal(t, "production", name)
	assert.Equal(t, "123456789012", id)
}

func TestPermissionSetIdentity_PrepareEnvironment_WithRegion(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ATMOS_XDG_CONFIG_HOME", tmpDir)

	identity, err := NewPermissionSetIdentity("test-ps", &schema.Identity{
		Kind: "aws/permission-set",
		Via:  &schema.IdentityVia{Provider: "aws-sso"},
		Principal: map[string]interface{}{
			"name":    "DevAccess",
			"account": map[string]interface{}{"id": "123456789012"},
			"region":  "us-west-2",
		},
	})
	require.NoError(t, err)

	psIdentity := identity.(*permissionSetIdentity)
	psIdentity.SetManagerAndProvider(nil, "aws-sso")

	environ := map[string]string{"TEST": "value"}
	result, err := identity.PrepareEnvironment(context.Background(), environ)
	assert.NoError(t, err)
	assert.Equal(t, "us-west-2", result["AWS_REGION"])
}

func TestPermissionSetIdentity_ResolveAccountID_EmptyName(t *testing.T) {
	identity, err := NewPermissionSetIdentity("test-ps", &schema.Identity{
		Kind: "aws/permission-set",
		Principal: map[string]interface{}{
			"name":    "DevAccess",
			"account": map[string]interface{}{"id": "123456789012"},
		},
	})
	require.NoError(t, err)

	psIdentity := identity.(*permissionSetIdentity)

	// Empty account name with ID provided should return the ID immediately.
	id, err := psIdentity.resolveAccountID(context.Background(), nil, "", "123456789012", "token")
	assert.NoError(t, err)
	assert.Equal(t, "123456789012", id)
}
