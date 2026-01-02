package exec

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	assert.Contains(t, err.Error(), "not implemented")
}

// TestAuthContextWrapper_StubMethodsPanic verifies that stub methods panic as expected.
// These methods should never be called in normal operation.
func TestAuthContextWrapper_StubMethodsPanic(t *testing.T) {
	wrapper := &authContextWrapper{
		stackInfo: &schema.ConfigAndStacksInfo{},
	}
	ctx := context.Background()

	tests := []struct {
		name string
		fn   func()
	}{
		{"GetCachedCredentials", func() { _, _ = wrapper.GetCachedCredentials(ctx, "test") }},
		{"Authenticate", func() { _, _ = wrapper.Authenticate(ctx, "test") }},
		{"Whoami", func() { _, _ = wrapper.Whoami(ctx, "test") }},
		{"Validate", func() { _ = wrapper.Validate() }},
		{"GetDefaultIdentity", func() { _, _ = wrapper.GetDefaultIdentity(false) }},
		{"ListProviders", func() { _ = wrapper.ListProviders() }},
		{"Logout", func() { _ = wrapper.Logout(ctx, "test", false) }},
		{"GetChain", func() { _ = wrapper.GetChain() }},
		{"ListIdentities", func() { _ = wrapper.ListIdentities() }},
		{"GetProviderForIdentity", func() { _ = wrapper.GetProviderForIdentity("test") }},
		{"GetFilesDisplayPath", func() { _ = wrapper.GetFilesDisplayPath("test") }},
		{"GetProviderKindForIdentity", func() { _, _ = wrapper.GetProviderKindForIdentity("test") }},
		{"GetIdentities", func() { _ = wrapper.GetIdentities() }},
		{"GetProviders", func() { _ = wrapper.GetProviders() }},
		{"LogoutProvider", func() { _ = wrapper.LogoutProvider(ctx, "test", false) }},
		{"LogoutAll", func() { _ = wrapper.LogoutAll(ctx, false) }},
		{"GetEnvironmentVariables", func() { _, _ = wrapper.GetEnvironmentVariables("test") }},
		{"PrepareShellEnvironment", func() { _, _ = wrapper.PrepareShellEnvironment(ctx, "test", nil) }},
		{"ExecuteIntegration", func() { _ = wrapper.ExecuteIntegration(ctx, "test") }},
		{"ExecuteIdentityIntegrations", func() { _ = wrapper.ExecuteIdentityIntegrations(ctx, "test") }},
		{"GetIntegration", func() { _, _ = wrapper.GetIntegration("test") }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Panics(t, tt.fn, "Expected %s to panic", tt.name)
		})
	}
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
