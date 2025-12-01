package exec

import (
	"context"
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

// TestAuthContextWrapperStubMethodsPanic verifies that stub methods panic when called.
// These methods should never be called since the wrapper is only used for propagating
// existing auth context, not for performing authentication.
func TestAuthContextWrapperStubMethodsPanic(t *testing.T) {
	wrapper := newAuthContextWrapper(&schema.AuthContext{})

	t.Run("GetCachedCredentials panics", func(t *testing.T) {
		assert.Panics(t, func() {
			_, _ = wrapper.GetCachedCredentials(context.TODO(), "test")
		}, "GetCachedCredentials should panic")
	})

	t.Run("Authenticate panics", func(t *testing.T) {
		assert.Panics(t, func() {
			_, _ = wrapper.Authenticate(context.TODO(), "test")
		}, "Authenticate should panic")
	})

	t.Run("Whoami panics", func(t *testing.T) {
		assert.Panics(t, func() {
			_, _ = wrapper.Whoami(context.TODO(), "test")
		}, "Whoami should panic")
	})

	t.Run("Validate panics", func(t *testing.T) {
		assert.Panics(t, func() {
			_ = wrapper.Validate()
		}, "Validate should panic")
	})

	t.Run("GetDefaultIdentity panics", func(t *testing.T) {
		assert.Panics(t, func() {
			_, _ = wrapper.GetDefaultIdentity(false)
		}, "GetDefaultIdentity should panic")
	})

	t.Run("ListProviders panics", func(t *testing.T) {
		assert.Panics(t, func() {
			_ = wrapper.ListProviders()
		}, "ListProviders should panic")
	})

	t.Run("Logout panics", func(t *testing.T) {
		assert.Panics(t, func() {
			_ = wrapper.Logout(context.TODO(), "test", false)
		}, "Logout should panic")
	})

	t.Run("GetChain panics", func(t *testing.T) {
		assert.Panics(t, func() {
			_ = wrapper.GetChain()
		}, "GetChain should panic")
	})

	t.Run("ListIdentities panics", func(t *testing.T) {
		assert.Panics(t, func() {
			_ = wrapper.ListIdentities()
		}, "ListIdentities should panic")
	})

	t.Run("GetProviderForIdentity panics", func(t *testing.T) {
		assert.Panics(t, func() {
			_ = wrapper.GetProviderForIdentity("test")
		}, "GetProviderForIdentity should panic")
	})

	t.Run("GetFilesDisplayPath panics", func(t *testing.T) {
		assert.Panics(t, func() {
			_ = wrapper.GetFilesDisplayPath("test")
		}, "GetFilesDisplayPath should panic")
	})

	t.Run("GetProviderKindForIdentity panics", func(t *testing.T) {
		assert.Panics(t, func() {
			_, _ = wrapper.GetProviderKindForIdentity("test")
		}, "GetProviderKindForIdentity should panic")
	})

	t.Run("GetIdentities panics", func(t *testing.T) {
		assert.Panics(t, func() {
			_ = wrapper.GetIdentities()
		}, "GetIdentities should panic")
	})

	t.Run("GetProviders panics", func(t *testing.T) {
		assert.Panics(t, func() {
			_ = wrapper.GetProviders()
		}, "GetProviders should panic")
	})

	t.Run("LogoutProvider panics", func(t *testing.T) {
		assert.Panics(t, func() {
			_ = wrapper.LogoutProvider(context.TODO(), "test", false)
		}, "LogoutProvider should panic")
	})

	t.Run("LogoutAll panics", func(t *testing.T) {
		assert.Panics(t, func() {
			_ = wrapper.LogoutAll(context.TODO(), false)
		}, "LogoutAll should panic")
	})

	t.Run("GetEnvironmentVariables panics", func(t *testing.T) {
		assert.Panics(t, func() {
			_, _ = wrapper.GetEnvironmentVariables("test")
		}, "GetEnvironmentVariables should panic")
	})

	t.Run("PrepareShellEnvironment panics", func(t *testing.T) {
		assert.Panics(t, func() {
			_, _ = wrapper.PrepareShellEnvironment(context.TODO(), "test", nil)
		}, "PrepareShellEnvironment should panic")
	})
}
