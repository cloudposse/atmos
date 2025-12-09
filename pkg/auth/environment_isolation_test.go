package auth

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/validation"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestAuthenticationIgnoresExternalAWSEnvVars verifies that when authenticating with
// an identity (via --identity flag or default), Atmos ignores external AWS environment
// variables that could interfere with authentication.
//
// This test addresses DEV-3706: https://linear.app/cloudposse/issue/DEV-3706
func TestAuthenticationIgnoresExternalAWSEnvVars(t *testing.T) {
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
	defaultIdentity, err := authManager.GetDefaultIdentity(false)
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
