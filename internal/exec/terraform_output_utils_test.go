package exec

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewAuthContextWrapper_NilContext(t *testing.T) {
	result := newAuthContextWrapper(nil)
	assert.Nil(t, result)
}

func TestNewAuthContextWrapper_WithContext(t *testing.T) {
	authContext := &schema.AuthContext{
		AWS: &schema.AWSAuthContext{
			Profile:         "test-profile",
			CredentialsFile: "/path/to/credentials",
			ConfigFile:      "/path/to/config",
			Region:          "us-east-1",
		},
	}

	wrapper := newAuthContextWrapper(authContext)
	require.NotNil(t, wrapper)
	require.NotNil(t, wrapper.stackInfo)
	assert.Equal(t, authContext, wrapper.stackInfo.AuthContext)
}

func TestAuthContextWrapper_GetStackInfo(t *testing.T) {
	authContext := &schema.AuthContext{
		AWS: &schema.AWSAuthContext{
			Profile: "test-profile",
		},
	}

	wrapper := newAuthContextWrapper(authContext)
	require.NotNil(t, wrapper)

	stackInfo := wrapper.GetStackInfo()
	require.NotNil(t, stackInfo)
	assert.Equal(t, authContext, stackInfo.AuthContext)
}

func TestAuthContextWrapper_GetStackInfo_EmptyAuthContext(t *testing.T) {
	authContext := &schema.AuthContext{}

	wrapper := newAuthContextWrapper(authContext)
	require.NotNil(t, wrapper)

	stackInfo := wrapper.GetStackInfo()
	require.NotNil(t, stackInfo)
	assert.Equal(t, authContext, stackInfo.AuthContext)
}

func TestAuthContextWrapper_AuthenticateProvider(t *testing.T) {
	wrapper := &authContextWrapper{
		stackInfo: &schema.ConfigAndStacksInfo{},
	}

	ctx := context.Background()
	_, err := wrapper.AuthenticateProvider(ctx, "test-provider")

	// Should return an error, not panic.
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrNotImplemented)
}

func TestEnvironToMapFiltered(t *testing.T) {
	// Test environToMap (which calls environToMapFiltered internally).
	// We can't easily control the environment, but we can verify the function works.
	result := environToMap()
	assert.NotNil(t, result)
	// The result should not contain prohibited env vars.
	for _, prohibited := range prohibitedEnvVars {
		_, exists := result[prohibited]
		assert.False(t, exists, "Should not contain prohibited env var: %s", prohibited)
	}
}

func TestGetTerraformOutputVariable_SimpleKey(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	outputs := map[string]any{
		"vpc_id":      "vpc-12345",
		"subnet_ids":  []string{"subnet-1", "subnet-2"},
		"null_output": nil,
	}

	// Test simple key that exists.
	value, exists, err := getTerraformOutputVariable(atmosConfig, "vpc", "dev-us-east-1", outputs, "vpc_id")
	require.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, "vpc-12345", value)

	// Test simple key that doesn't exist.
	value, exists, err = getTerraformOutputVariable(atmosConfig, "vpc", "dev-us-east-1", outputs, "nonexistent")
	require.NoError(t, err)
	assert.False(t, exists)
	assert.Nil(t, value)

	// Test key with nil value.
	value, exists, err = getTerraformOutputVariable(atmosConfig, "vpc", "dev-us-east-1", outputs, "null_output")
	require.NoError(t, err)
	assert.True(t, exists)
	assert.Nil(t, value)
}

func TestGetTerraformOutputVariable_WithDotPrefix(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	outputs := map[string]any{
		"vpc_id": "vpc-12345",
	}

	// Test with dot prefix.
	value, exists, err := getTerraformOutputVariable(atmosConfig, "vpc", "dev-us-east-1", outputs, ".vpc_id")
	require.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, "vpc-12345", value)
}

func TestGetStaticRemoteStateOutput_SimpleKey(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	remoteState := map[string]any{
		"vpc_id":     "vpc-67890",
		"cidr_block": "10.0.0.0/16",
	}

	// Test simple key that exists.
	value, exists, err := GetStaticRemoteStateOutput(atmosConfig, "vpc", "prod-us-west-2", remoteState, "vpc_id")
	require.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, "vpc-67890", value)

	// Test simple key that doesn't exist.
	value, exists, err = GetStaticRemoteStateOutput(atmosConfig, "vpc", "prod-us-west-2", remoteState, "nonexistent")
	require.NoError(t, err)
	assert.False(t, exists)
	assert.Nil(t, value)
}

func TestGetStaticRemoteStateOutput_WithDotPrefix(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	remoteState := map[string]any{
		"vpc_id": "vpc-67890",
	}

	// Test with dot prefix.
	value, exists, err := GetStaticRemoteStateOutput(atmosConfig, "vpc", "prod-us-west-2", remoteState, ".vpc_id")
	require.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, "vpc-67890", value)
}

func TestProhibitedEnvVars(t *testing.T) {
	// Verify the prohibited env vars list is populated.
	assert.NotEmpty(t, prohibitedEnvVars)
	assert.Contains(t, prohibitedEnvVars, cliArgsEnvVar)
	assert.Contains(t, prohibitedEnvVars, inputEnvVar)
	assert.Contains(t, prohibitedEnvVars, automationEnvVar)
}

func TestProhibitedEnvVarPrefixes(t *testing.T) {
	// Verify the prohibited env var prefixes list is populated.
	assert.NotEmpty(t, prohibitedEnvVarPrefixes)
	assert.Contains(t, prohibitedEnvVarPrefixes, varEnvVarPrefix)
	assert.Contains(t, prohibitedEnvVarPrefixes, cliArgEnvVarPrefix)
}

func TestAuthContextWrapper_PanicMethods(t *testing.T) {
	wrapper := &authContextWrapper{
		stackInfo: &schema.ConfigAndStacksInfo{},
	}
	ctx := context.Background()

	// Test that GetCachedCredentials panics.
	assert.Panics(t, func() {
		_, _ = wrapper.GetCachedCredentials(ctx, "test")
	})

	// Test that Authenticate panics.
	assert.Panics(t, func() {
		_, _ = wrapper.Authenticate(ctx, "test")
	})

	// Test that Whoami panics.
	assert.Panics(t, func() {
		_, _ = wrapper.Whoami(ctx, "test")
	})

	// Test that Validate panics.
	assert.Panics(t, func() {
		_ = wrapper.Validate()
	})

	// Test that GetDefaultIdentity panics.
	assert.Panics(t, func() {
		_, _ = wrapper.GetDefaultIdentity(false)
	})

	// Test that ListProviders panics.
	assert.Panics(t, func() {
		_ = wrapper.ListProviders()
	})

	// Test that Logout panics.
	assert.Panics(t, func() {
		_ = wrapper.Logout(ctx, "test", false)
	})

	// Test that GetChain panics.
	assert.Panics(t, func() {
		_ = wrapper.GetChain()
	})

	// Test that ListIdentities panics.
	assert.Panics(t, func() {
		_ = wrapper.ListIdentities()
	})

	// Test that GetProviderForIdentity panics.
	assert.Panics(t, func() {
		_ = wrapper.GetProviderForIdentity("test")
	})

	// Test that GetFilesDisplayPath panics.
	assert.Panics(t, func() {
		_ = wrapper.GetFilesDisplayPath("test")
	})

	// Test that GetProviderKindForIdentity panics.
	assert.Panics(t, func() {
		_, _ = wrapper.GetProviderKindForIdentity("test")
	})

	// Test that GetIdentities panics.
	assert.Panics(t, func() {
		_ = wrapper.GetIdentities()
	})

	// Test that GetProviders panics.
	assert.Panics(t, func() {
		_ = wrapper.GetProviders()
	})

	// Test that LogoutProvider panics.
	assert.Panics(t, func() {
		_ = wrapper.LogoutProvider(ctx, "test", false)
	})

	// Test that LogoutAll panics.
	assert.Panics(t, func() {
		_ = wrapper.LogoutAll(ctx, false)
	})

	// Test that GetEnvironmentVariables panics.
	assert.Panics(t, func() {
		_, _ = wrapper.GetEnvironmentVariables("test")
	})

	// Test that PrepareShellEnvironment panics.
	assert.Panics(t, func() {
		_, _ = wrapper.PrepareShellEnvironment(ctx, "test", nil)
	})

	// Test that ExecuteIntegration panics.
	assert.Panics(t, func() {
		_ = wrapper.ExecuteIntegration(ctx, "test")
	})

	// Test that ExecuteIdentityIntegrations panics.
	assert.Panics(t, func() {
		_ = wrapper.ExecuteIdentityIntegrations(ctx, "test")
	})

	// Test that GetIntegration panics.
	assert.Panics(t, func() {
		_, _ = wrapper.GetIntegration("test")
	})
}
