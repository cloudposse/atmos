package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestNewAuthContextWrapper verifies that authContextWrapper is properly created.
func TestNewAuthContextWrapper(t *testing.T) {
	t.Run("nil authContext returns nil wrapper", func(t *testing.T) {
		wrapper := newAuthContextWrapper(nil)
		assert.Nil(t, wrapper, "Should return nil for nil authContext")
	})

	t.Run("valid authContext creates wrapper with stackInfo", func(t *testing.T) {
		authContext := &schema.AuthContext{
			AWS: &schema.AWSAuthContext{
				Profile: "test-profile",
				Region:  "us-east-1",
			},
		}

		wrapper := newAuthContextWrapper(authContext)

		require.NotNil(t, wrapper, "Should create wrapper for valid authContext")
		require.NotNil(t, wrapper.stackInfo, "Wrapper should have stackInfo")
		assert.Equal(t, authContext, wrapper.stackInfo.AuthContext, "StackInfo should contain the provided authContext")
	})
}

// TestAuthContextWrapperGetStackInfo verifies GetStackInfo returns the correct stackInfo.
func TestAuthContextWrapperGetStackInfo(t *testing.T) {
	authContext := &schema.AuthContext{
		AWS: &schema.AWSAuthContext{
			Profile:         "test-identity",
			Region:          "us-west-2",
			CredentialsFile: "/tmp/creds",
			ConfigFile:      "/tmp/config",
		},
	}

	wrapper := newAuthContextWrapper(authContext)

	stackInfo := wrapper.GetStackInfo()

	require.NotNil(t, stackInfo, "GetStackInfo should return non-nil")
	assert.Equal(t, authContext, stackInfo.AuthContext, "GetStackInfo should return stackInfo with authContext")
	assert.Equal(t, "test-identity", stackInfo.AuthContext.AWS.Profile)
	assert.Equal(t, "us-west-2", stackInfo.AuthContext.AWS.Region)
}

// TestAuthContextWrapperResolvePrincipalSetting verifies ResolvePrincipalSetting returns nil, false.
// The wrapper doesn't have access to identity/provider configuration, only auth context.
func TestAuthContextWrapperResolvePrincipalSetting(t *testing.T) {
	authContext := &schema.AuthContext{
		AWS: &schema.AWSAuthContext{
			Profile: "test-identity",
			Region:  "us-west-2",
		},
	}

	wrapper := newAuthContextWrapper(authContext)

	// ResolvePrincipalSetting should always return nil, false for the wrapper.
	// It only propagates existing auth context, not full identity configuration.
	val, found := wrapper.ResolvePrincipalSetting("any-identity", "region")
	assert.Nil(t, val, "ResolvePrincipalSetting should return nil")
	assert.False(t, found, "ResolvePrincipalSetting should return false")

	val, found = wrapper.ResolvePrincipalSetting("test-identity", "any-key")
	assert.Nil(t, val, "ResolvePrincipalSetting should return nil for any key")
	assert.False(t, found, "ResolvePrincipalSetting should return false for any key")
}

// TestAuthContextWrapperResolveProviderConfig verifies ResolveProviderConfig returns nil, false.
// The wrapper doesn't have access to provider configuration.
func TestAuthContextWrapperResolveProviderConfig(t *testing.T) {
	authContext := &schema.AuthContext{
		AWS: &schema.AWSAuthContext{
			Profile: "test-identity",
			Region:  "us-west-2",
		},
	}

	wrapper := newAuthContextWrapper(authContext)

	// ResolveProviderConfig should always return nil, false for the wrapper.
	// It only propagates existing auth context, not provider configuration.
	provider, found := wrapper.ResolveProviderConfig("any-identity")
	assert.Nil(t, provider, "ResolveProviderConfig should return nil")
	assert.False(t, found, "ResolveProviderConfig should return false")

	provider, found = wrapper.ResolveProviderConfig("test-identity")
	assert.Nil(t, provider, "ResolveProviderConfig should return nil for any identity")
	assert.False(t, found, "ResolveProviderConfig should return false for any identity")
}
