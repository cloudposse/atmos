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
	t.Parallel()

	result := newAuthContextWrapper(nil)
	assert.Nil(t, result)
}

func TestNewAuthContextWrapper_WithContext(t *testing.T) {
	t.Parallel()

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

// TestPropagateAuthDisabledManager covers the shared decision logic used by !terraform.output,
// !terraform.state, and !aws.cloudformation.output to preserve an enclosing component's
// AuthDisabled opt-out when no AuthManager was created: only a nil authManager combined with
// AuthDisabled gets wrapped; a non-nil authManager and/or AuthDisabled=false pass through
// unchanged so the target component's own auth section can still be resolved normally.
func TestPropagateAuthDisabledManager(t *testing.T) {
	t.Parallel()

	t.Run("nil authManager and AuthDisabled wraps stackInfo", func(t *testing.T) {
		t.Parallel()
		stackInfo := &schema.ConfigAndStacksInfo{AuthDisabled: true}

		got := propagateAuthDisabledManager(nil, stackInfo)

		wrapper, ok := got.(*authContextWrapper)
		require.True(t, ok, "nil authManager with AuthDisabled must be wrapped")
		assert.Same(t, stackInfo, wrapper.stackInfo)
		assert.True(t, wrapper.GetStackInfo().AuthDisabled)
	})

	t.Run("nil authManager with AuthDisabled false stays nil", func(t *testing.T) {
		t.Parallel()
		stackInfo := &schema.ConfigAndStacksInfo{AuthDisabled: false}

		got := propagateAuthDisabledManager(nil, stackInfo)

		assert.Nil(t, got, "with auth enabled, nil must be passed through so the target's own auth resolves")
	})

	t.Run("non-nil authManager passes through even when AuthDisabled", func(t *testing.T) {
		t.Parallel()
		stackInfo := &schema.ConfigAndStacksInfo{AuthDisabled: true}
		existing := &authContextWrapper{stackInfo: &schema.ConfigAndStacksInfo{}}

		got := propagateAuthDisabledManager(existing, stackInfo)

		assert.Same(t, existing, got, "an already-populated AuthManager must never be overwritten")
	})

	t.Run("nil stackInfo is a no-op", func(t *testing.T) {
		t.Parallel()

		got := propagateAuthDisabledManager(nil, nil)

		assert.Nil(t, got)
	})
}

func TestAuthContextWrapper_GetStackInfo(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

	authContext := &schema.AuthContext{}

	wrapper := newAuthContextWrapper(authContext)
	require.NotNil(t, wrapper)

	stackInfo := wrapper.GetStackInfo()
	require.NotNil(t, stackInfo)
	assert.Equal(t, authContext, stackInfo.AuthContext)
}

func TestAuthContextWrapper_AuthenticateProvider(t *testing.T) {
	t.Parallel()

	wrapper := &authContextWrapper{
		stackInfo: &schema.ConfigAndStacksInfo{},
	}

	ctx := context.Background()
	_, err := wrapper.AuthenticateProvider(ctx, "test-provider")

	// Should return an error, not panic.
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrNotImplemented)
}

// TestAuthContextWrapper_GetChain_NoLongerPanics is a regression test for Issue #1921.
// When !terraform.output processes a component with auth configured, it creates an authContextWrapper
// to propagate auth context. If the nested component has its own auth config with a default identity,
// resolveAuthManagerForNestedComponent calls GetChain() on the parentAuthManager to inherit the identity.
// Previously, GetChain() panicked, causing the reported bug. Now it returns an empty slice.
func TestAuthContextWrapper_GetChain_NoLongerPanics(t *testing.T) {
	t.Parallel()

	authContext := &schema.AuthContext{
		AWS: &schema.AWSAuthContext{
			Profile: "test-profile",
			Region:  "us-east-1",
		},
	}

	wrapper := newAuthContextWrapper(authContext)
	require.NotNil(t, wrapper)

	// This should NOT panic (was the bug in #1921).
	require.NotPanics(t, func() {
		chain := wrapper.GetChain()
		// Should return empty slice, indicating no inherited identity chain.
		assert.Empty(t, chain)
	})
}

func TestAuthContextWrapper_PanicMethods(t *testing.T) {
	t.Parallel()

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

	// Test that GetChain returns empty slice (no panic).
	// Fixed in #1921: GetChain is called by resolveAuthManagerForNestedComponent,
	// so it must not panic. An empty slice means no inherited identity.
	chain := wrapper.GetChain()
	assert.Empty(t, chain, "GetChain should return empty slice")

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
