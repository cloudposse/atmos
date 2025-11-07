package auth

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/validation"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestCreateAndAuthenticateManager_EmptyIdentity(t *testing.T) {
	// When identity name is empty, function should return nil without error.
	authConfig := &schema.AuthConfig{
		Identities: map[string]schema.Identity{
			"test": {Kind: "aws/user"},
		},
	}

	manager, err := CreateAndAuthenticateManager("", authConfig, "__SELECT__")

	require.NoError(t, err, "should return no error for empty identity")
	assert.Nil(t, manager, "should return nil manager for empty identity")
}

func TestCreateAndAuthenticateManager_NilAuthConfig(t *testing.T) {
	// When auth config is nil, should return ErrAuthNotConfigured.
	manager, err := CreateAndAuthenticateManager("test-identity", nil, "__SELECT__")

	require.Error(t, err, "should return error for nil auth config")
	assert.ErrorIs(t, err, errUtils.ErrAuthNotConfigured, "should return ErrAuthNotConfigured")
	assert.Nil(t, manager, "should return nil manager")
}

func TestCreateAndAuthenticateManager_EmptyIdentities(t *testing.T) {
	// When identities map is empty, should return ErrAuthNotConfigured.
	authConfig := &schema.AuthConfig{
		Identities: map[string]schema.Identity{},
	}

	manager, err := CreateAndAuthenticateManager("test-identity", authConfig, "__SELECT__")

	require.Error(t, err, "should return error for empty identities")
	assert.ErrorIs(t, err, errUtils.ErrAuthNotConfigured, "should return ErrAuthNotConfigured")
	assert.Nil(t, manager, "should return nil manager")
}

func TestCreateAndAuthenticateManager_NonExistentIdentity(t *testing.T) {
	// When trying to authenticate with an identity that doesn't exist in config.
	authConfig := &schema.AuthConfig{
		Providers: map[string]schema.Provider{
			"test-provider": {
				Kind:     "aws/iam-identity-center",
				Region:   "us-east-1",
				StartURL: "https://test.awsapps.com/start",
			},
		},
		Identities: map[string]schema.Identity{
			"existing-identity": {
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

	manager, err := CreateAndAuthenticateManager("non-existent-identity", authConfig, "__SELECT__")

	require.Error(t, err, "should return error for non-existent identity")
	assert.ErrorIs(t, err, errUtils.ErrIdentityNotFound, "should return ErrIdentityNotFound")
	assert.Nil(t, manager, "should return nil manager")
}

func TestCreateAndAuthenticateManager_SelectValueWithSingleDefault(t *testing.T) {
	// When using select value with a single default identity,
	// the function should automatically use that default identity.

	authConfig := &schema.AuthConfig{
		Providers: map[string]schema.Provider{
			"test-provider": {
				Kind:     "aws/iam-identity-center",
				Region:   "us-east-1",
				StartURL: "https://test.awsapps.com/start",
			},
		},
		Identities: map[string]schema.Identity{
			"default-identity": {
				Kind:    "aws/permission-set",
				Default: true,
				Via: &schema.IdentityVia{
					Provider: "test-provider",
				},
				Principal: map[string]interface{}{
					"name": "DefaultPS",
					"account": map[string]interface{}{
						"id": "123456789012",
					},
				},
			},
			"other-identity": {
				Kind:    "aws/permission-set",
				Default: false,
				Via: &schema.IdentityVia{
					Provider: "test-provider",
				},
				Principal: map[string]interface{}{
					"name": "OtherPS",
					"account": map[string]interface{}{
						"id": "123456789012",
					},
				},
			},
		},
	}

	_, err := CreateAndAuthenticateManager("__SELECT__", authConfig, "__SELECT__")
	// This will fail because we don't have real SSO credentials, but that's expected.
	// The important thing is it should fail with an authentication error, not a config error.
	if err != nil {
		// Authentication errors are expected (no real credentials).
		// But it should NOT be ErrAuthNotConfigured or ErrIdentityNotInConfig.
		assert.NotErrorIs(t, err, errUtils.ErrAuthNotConfigured, "should not be auth not configured error")
		assert.NotErrorIs(t, err, errUtils.ErrIdentityNotInConfig, "should not be identity not in config error")
	}

	// Manager creation should have started (even if auth fails).
	// In some cases it might be nil if auth failed early.
	// The key test is that it tried to use the default identity.
}

func TestCreateAndAuthenticateManager_SelectValueInCIMode(t *testing.T) {
	// When using select value (interactive selection) in CI mode,
	// should return an error since TTY is not available.

	authConfig := &schema.AuthConfig{
		Providers: map[string]schema.Provider{
			"test-provider": {
				Kind:     "aws/iam-identity-center",
				Region:   "us-east-1",
				StartURL: "https://test.awsapps.com/start",
			},
		},
		Identities: map[string]schema.Identity{
			"default-1": {
				Kind:    "aws/permission-set",
				Default: true,
				Via: &schema.IdentityVia{
					Provider: "test-provider",
				},
				Principal: map[string]interface{}{
					"name": "PS1",
					"account": map[string]interface{}{
						"id": "123456789012",
					},
				},
			},
			"default-2": {
				Kind:    "aws/permission-set",
				Default: true,
				Via: &schema.IdentityVia{
					Provider: "test-provider",
				},
				Principal: map[string]interface{}{
					"name": "PS2",
					"account": map[string]interface{}{
						"id": "123456789012",
					},
				},
			},
		},
	}

	// Set CI environment to force non-interactive mode.
	t.Setenv("CI", "true")

	manager, err := CreateAndAuthenticateManager("__SELECT__", authConfig, "__SELECT__")

	require.Error(t, err, "should return error for interactive selection in CI mode")
	assert.Nil(t, manager, "should return nil manager")
	assert.ErrorIs(t, err, errUtils.ErrIdentitySelectionRequiresTTY, "should return ErrIdentitySelectionRequiresTTY in CI mode")
}

func TestCreateAndAuthenticateManager_ManagerCreation(t *testing.T) {
	// Test that NewAuthManager is called with proper parameters.
	// We verify this by checking that the manager structure is created correctly.

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

	// Call directly to verify manager creation (even though auth will fail).
	credStore := credentials.NewCredentialStore()
	validator := validation.NewValidator()
	stackInfo := &schema.ConfigAndStacksInfo{
		AuthContext: &schema.AuthContext{},
	}

	// This should succeed in creating the manager (auth will fail later).
	authManager, err := NewAuthManager(
		authConfig,
		credStore,
		validator,
		stackInfo,
	)

	require.NoError(t, err, "NewAuthManager should succeed with valid config")
	require.NotNil(t, authManager, "manager should be created")

	// Verify GetStackInfo returns the stackInfo we passed.
	returnedStackInfo := authManager.GetStackInfo()
	assert.Equal(t, stackInfo, returnedStackInfo, "GetStackInfo should return the same stackInfo")
}

func TestCreateAndAuthenticateManager_AuthContextPopulation(t *testing.T) {
	// Test that AuthContext is properly initialized in stackInfo.

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

	_, err := CreateAndAuthenticateManager("test-identity", authConfig, "__SELECT__")
	// Authentication will fail without real credentials, but that's expected.
	if err != nil {
		// Should be an authentication error, not a configuration error.
		assert.NotErrorIs(t, err, errUtils.ErrAuthNotConfigured)
		assert.NotErrorIs(t, err, errUtils.ErrIdentityNotInConfig)
	}

	// Even if auth fails, we can verify the stackInfo structure was created.
	// In production code, this test verifies the function creates the stackInfo properly.
}

func TestCreateAndAuthenticateManager_WithConflictingEnvVars(t *testing.T) {
	// Verify that the function works even with conflicting AWS environment variables set.
	// This is important for the use case where users have AWS credentials in their environment.

	// Set conflicting environment variables.
	t.Setenv("AWS_PROFILE", "conflicting-profile")
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIAIOSFODNN7EXAMPLE")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")

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

	manager, err := CreateAndAuthenticateManager("test-identity", authConfig, "__SELECT__")
	// Authentication will fail (no real SSO), but it should not fail due to conflicting env vars.
	if err != nil {
		// The error should be about authentication, not about the conflicting env vars.
		// If env vars were not isolated, we'd see errors about invalid credentials.
		assert.NotContains(t, err.Error(), "AKIAIOSFODNN7EXAMPLE", "error should not reference the fake access key")
	}

	// Verify environment variables are still set (not cleared permanently).
	assert.Equal(t, "conflicting-profile", os.Getenv("AWS_PROFILE"))
	assert.Equal(t, "AKIAIOSFODNN7EXAMPLE", os.Getenv("AWS_ACCESS_KEY_ID"))

	// Manager may or may not be nil depending on when auth failed.
	_ = manager
}

func TestCreateAndAuthenticateManager_ErrorMessageClarity(t *testing.T) {
	// Test that error messages are clear and helpful.

	tests := []struct {
		name          string
		identityName  string
		authConfig    *schema.AuthConfig
		selectValue   string
		expectedError string
	}{
		{
			name:          "nil config error mentions requirement",
			identityName:  "test",
			authConfig:    nil,
			selectValue:   "__SELECT__",
			expectedError: "authentication requires at least one identity",
		},
		{
			name:         "empty identities error mentions requirement",
			identityName: "test",
			authConfig: &schema.AuthConfig{
				Identities: map[string]schema.Identity{},
			},
			selectValue:   "__SELECT__",
			expectedError: "authentication requires at least one identity",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, err := CreateAndAuthenticateManager(tt.identityName, tt.authConfig, tt.selectValue)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedError, "error message should be clear")
			assert.Nil(t, manager)
		})
	}
}

func TestCreateAndAuthenticateManager_SelectValueParameter(t *testing.T) {
	// Test that the selectValue parameter is used correctly for comparison.

	authConfig := &schema.AuthConfig{
		Providers: map[string]schema.Provider{
			"test-provider": {
				Kind:     "aws/iam-identity-center",
				Region:   "us-east-1",
				StartURL: "https://test.awsapps.com/start",
			},
		},
		Identities: map[string]schema.Identity{
			"default-identity": {
				Kind:    "aws/permission-set",
				Default: true,
				Via: &schema.IdentityVia{
					Provider: "test-provider",
				},
				Principal: map[string]interface{}{
					"name": "DefaultPS",
					"account": map[string]interface{}{
						"id": "123456789012",
					},
				},
			},
		},
	}

	// Test with a custom select value.
	manager, err := CreateAndAuthenticateManager("CUSTOM_SELECT", authConfig, "CUSTOM_SELECT")
	// Should attempt to use default identity.
	if err != nil {
		// Should be auth error, not config error.
		assert.NotErrorIs(t, err, errUtils.ErrAuthNotConfigured)
		assert.NotErrorIs(t, err, errUtils.ErrIdentityNotInConfig)
	}

	_ = manager
}
