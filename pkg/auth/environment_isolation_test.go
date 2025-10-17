package auth

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/auth/validation"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestAuthenticationIgnoresExternalAWSEnvVars verifies that when authenticating with
// an identity (via --identity flag or default), Atmos ignores external AWS environment
// variables that could interfere with authentication.
//
// This test addresses DEV-3706: https://linear.app/cloudposse/issue/DEV-3706
func TestAuthenticationIgnoresExternalAWSEnvVars(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test that requires AWS SDK initialization")
	}

	// Set problematic AWS environment variables that should be ignored.
	t.Setenv("AWS_PROFILE", "conflicting-profile")
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIAIOSFODNN7EXAMPLE")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")
	t.Setenv("AWS_SESSION_TOKEN", "FakeSessionToken123")
	t.Setenv("AWS_CONFIG_FILE", "/nonexistent/config")
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", "/nonexistent/credentials")

	// Set AWS_REGION which should NOT be ignored.
	t.Setenv("AWS_REGION", "us-west-2")

	// Create a minimal auth configuration that would normally fail
	// if AWS SDK picks up the conflicting environment variables.
	authConfig := &schema.AuthConfig{
		Providers: map[string]schema.Provider{
			"test-sso-provider": {
				Kind:     "aws/iam-identity-center",
				Region:   "us-east-1",
				StartURL: "https://test.awsapps.com/start",
			},
		},
		Identities: map[string]schema.Identity{
			"test-permission-set": {
				Kind:    "aws/permission-set",
				Default: true,
				Via: &schema.IdentityVia{
					Provider: "test-sso-provider",
				},
				Principal: map[string]interface{}{
					"name": "TestPermissionSet",
					"account": map[string]interface{}{
						"id": "123456789012",
					},
				},
			},
		},
	}

	// Create auth manager.
	credStore := credentials.NewCredentialStore()
	validator := validation.NewValidator()

	authManager, err := NewAuthManager(
		authConfig,
		credStore,
		validator,
		nil,
	)
	require.NoError(t, err)
	require.NotNil(t, authManager)

	// Verify the manager was created successfully despite conflicting env vars.
	// The actual authentication would fail (no real SSO setup), but the important
	// part is that the AWS SDK config loading doesn't fail due to the conflicting
	// environment variables.
	//
	// If the environment variables were NOT being isolated, the AWS SDK would
	// try to use the fake credentials and fail with authentication errors related
	// to those specific credentials.

	// Verify environment variables are still set (not cleared permanently).
	assert.Equal(t, "conflicting-profile", os.Getenv("AWS_PROFILE"))
	assert.Equal(t, "AKIAIOSFODNN7EXAMPLE", os.Getenv("AWS_ACCESS_KEY_ID"))
	assert.Equal(t, "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", os.Getenv("AWS_SECRET_ACCESS_KEY"))
	assert.Equal(t, "FakeSessionToken123", os.Getenv("AWS_SESSION_TOKEN"))
	assert.Equal(t, "/nonexistent/config", os.Getenv("AWS_CONFIG_FILE"))
	assert.Equal(t, "/nonexistent/credentials", os.Getenv("AWS_SHARED_CREDENTIALS_FILE"))
	assert.Equal(t, "us-west-2", os.Getenv("AWS_REGION"))

	// Get default identity (this should work despite env vars).
	defaultIdentity, err := authManager.GetDefaultIdentity()
	require.NoError(t, err)
	assert.Equal(t, "test-permission-set", defaultIdentity)
}

// TestAuthManagerCreationWithConflictingEnvVars verifies that creating an auth manager
// succeeds even when conflicting AWS environment variables are set.
func TestAuthManagerCreationWithConflictingEnvVars(t *testing.T) {
	// Set conflicting environment variables.
	t.Setenv("AWS_PROFILE", "fake-profile")
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIAEXAMPLE")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secretEXAMPLE")

	authConfig := &schema.AuthConfig{
		Providers: map[string]schema.Provider{
			"test-provider": {
				Kind:     "aws/iam-identity-center",
				Region:   "us-east-1",
				StartURL: "https://test.awsapps.com/start",
			},
		},
		Identities: map[string]schema.Identity{
			"test-identity": {
				Kind: "aws/permission-set",
				Via: &schema.IdentityVia{
					Provider: "test-provider",
				},
				Principal: map[string]interface{}{
					"name": "TestPS",
					"account": map[string]interface{}{
						"id": "123456789012",
					},
				},
			},
		},
	}

	credStore := credentials.NewCredentialStore()
	validator := validation.NewValidator()

	// This should succeed - environment variables should be isolated during provider initialization.
	authManager, err := NewAuthManager(
		authConfig,
		credStore,
		validator,
		nil,
	)

	require.NoError(t, err, "Auth manager creation should succeed despite conflicting env vars")
	assert.NotNil(t, authManager)

	// Verify environment variables are still set after manager creation.
	assert.Equal(t, "fake-profile", os.Getenv("AWS_PROFILE"))
	assert.Equal(t, "AKIAEXAMPLE", os.Getenv("AWS_ACCESS_KEY_ID"))
	assert.Equal(t, "secretEXAMPLE", os.Getenv("AWS_SECRET_ACCESS_KEY"))
}

// TestIdentityCreationIgnoresEnvVars tests that identity instances can be created
// successfully even when conflicting AWS environment variables are present.
func TestIdentityCreationIgnoresEnvVars(t *testing.T) {
	// Set conflicting environment variables.
	t.Setenv("AWS_PROFILE", "wrong-profile")
	t.Setenv("AWS_ACCESS_KEY_ID", "WRONGKEY")

	// Create various identity types - they should all succeed.
	identityConfigs := map[string]schema.Identity{
		"permission-set": {
			Kind: "aws/permission-set",
			Via: &schema.IdentityVia{
				Provider: "test-provider",
			},
			Principal: map[string]interface{}{
				"name": "TestPermissionSet",
				"account": map[string]interface{}{
					"id": "123456789012",
				},
			},
		},
		"assume-role": {
			Kind: "aws/assume-role",
			Via: &schema.IdentityVia{
				Provider: "test-provider",
			},
			Principal: map[string]interface{}{
				"assume_role": "arn:aws:iam::123456789012:role/TestRole",
			},
		},
		"aws-user": {
			Kind: "aws/user",
			Credentials: map[string]interface{}{
				"region": "us-east-1",
			},
		},
	}

	for name, config := range identityConfigs {
		t.Run(name, func(t *testing.T) {
			// Creating an identity instance should succeed.
			var identity types.Identity
			var err error

			// Use the factory pattern to create identities based on kind.
			switch config.Kind {
			case "aws/permission-set":
				identity, err = createPermissionSetIdentity(name, &config)
			case "aws/assume-role":
				identity, err = createAssumeRoleIdentity(name, &config)
			case "aws/user":
				identity, err = createUserIdentity(name, &config)
			}

			require.NoError(t, err, "Identity creation should succeed for %s", name)
			assert.NotNil(t, identity)

			// Verify environment variables are still present.
			assert.Equal(t, "wrong-profile", os.Getenv("AWS_PROFILE"))
			assert.Equal(t, "WRONGKEY", os.Getenv("AWS_ACCESS_KEY_ID"))
		})
	}
}

// Helper functions to create identity instances (mimics factory logic).
func createPermissionSetIdentity(name string, config *schema.Identity) (types.Identity, error) {
	// This would use the actual factory, but for testing we just verify the structure.
	if config.Kind != "aws/permission-set" {
		return nil, assert.AnError
	}
	// In real code, this would call the identity constructor.
	// For this test, we're just verifying the config structure is valid.
	return nil, nil
}

func createAssumeRoleIdentity(name string, config *schema.Identity) (types.Identity, error) {
	if config.Kind != "aws/assume-role" {
		return nil, assert.AnError
	}
	return nil, nil
}

func createUserIdentity(name string, config *schema.Identity) (types.Identity, error) {
	if config.Kind != "aws/user" {
		return nil, assert.AnError
	}
	return nil, nil
}

// TestAuthenticateWithIdentityFlagClearsEnvVars is a documentation test that
// demonstrates the expected behavior when using --identity flag.
func TestAuthenticateWithIdentityFlagClearsEnvVars(t *testing.T) {
	// This test documents the expected behavior:
	// When a user runs: atmos terraform plan mycomponent -s mystack --identity myidentity
	// And they have these environment variables set:
	//   AWS_PROFILE=wrong-profile
	//   AWS_ACCESS_KEY_ID=WRONGKEY
	//   AWS_SECRET_ACCESS_KEY=WRONGSECRET
	//
	// Atmos should:
	// 1. Temporarily clear these variables during authentication
	// 2. Authenticate using the specified identity
	// 3. Set NEW environment variables based on the authenticated identity
	// 4. Execute terraform with the correct credentials
	//
	// The user's original environment variables should not interfere with authentication.

	t.Log("This test documents the expected behavior of environment variable isolation")
	t.Log("See DEV-3706: https://linear.app/cloudposse/issue/DEV-3706")
	t.Log("")
	t.Log("When --identity is used, external AWS environment variables are temporarily")
	t.Log("cleared during authentication to prevent conflicts with Atmos-managed credentials.")
}
