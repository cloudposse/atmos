package exec

// terraform_execute_helpers_auth_test.go contains unit tests for setupTerraformAuth,
// covering all injectable-var branches without requiring real AWS/Azure infrastructure.
//
// Injection points tested:
//   - defaultComponentConfigFetcher (utils_auth.go): ErrInvalidComponent propagation
//   - defaultAuthManagerCreator (utils_auth.go): creator error → ErrFailedToInitializeAuthManager
//   - defaultAuthManagerCreator (success): identity stored, AuthManager set
//   - defaultAuthManagerCreator (nil): nil manager → no auth-bridge injection, no panic
//   - defaultMergedAuthConfigGetter (terraform_execute_helpers.go): non-ErrInvalidComponent
//     error → ErrInvalidAuthConfig wrap

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	auth "github.com/cloudposse/atmos/pkg/auth"
	mockTypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestSetupTerraformAuth_EmptyConfig_NoProviders verifies that with an empty
// AtmosConfiguration (no auth providers configured) the function completes
// without error and leaves info.AuthManager as nil.
func TestSetupTerraformAuth_EmptyConfig_NoProviders(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		// Empty stack and component → getMergedAuthConfig skips component lookup.
		Stack:            "",
		ComponentFromArg: "",
	}

	authMgr, err := setupTerraformAuth(&atmosConfig, &info)
	require.NoError(t, err)
	// With no auth providers configured the AuthManager should be nil.
	assert.Nil(t, authMgr)
	assert.Nil(t, info.AuthManager)
}

// TestSetupTerraformAuth_ErrInvalidComponent verifies that ErrInvalidComponent is
// propagated directly without additional sentinel wrapping.  This prevents auth prompts
// when the caller references a component that does not exist.
func TestSetupTerraformAuth_ErrInvalidComponent(t *testing.T) {
	orig := defaultComponentConfigFetcher
	t.Cleanup(func() { defaultComponentConfigFetcher = orig })
	defaultComponentConfigFetcher = func(_ *ExecuteDescribeComponentParams) (map[string]any, error) {
		return nil, errUtils.ErrInvalidComponent
	}

	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		Stack:            "dev",
		ComponentFromArg: "nonexistent",
	}

	_, err := setupTerraformAuth(&atmosConfig, &info)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidComponent), "expected ErrInvalidComponent but got: %v", err)
	// Must NOT be additionally wrapped — doing so would change the sentinel seen by callers.
	assert.False(t, errors.Is(err, errUtils.ErrInvalidAuthConfig), "ErrInvalidComponent must not be wrapped with ErrInvalidAuthConfig")
}

// TestSetupTerraformAuth_AuthCreatorError_WrapsWithSentinel verifies that errors
// returned by the auth manager creator are wrapped with ErrFailedToInitializeAuthManager,
// keeping parity with createAndAuthenticateAuthManagerWithDeps.
func TestSetupTerraformAuth_AuthCreatorError_WrapsWithSentinel(t *testing.T) {
	orig := defaultAuthManagerCreator
	t.Cleanup(func() { defaultAuthManagerCreator = orig })
	defaultAuthManagerCreator = func(_ string, _ *schema.AuthConfig, _ string, _ *schema.AtmosConfiguration) (auth.AuthManager, error) {
		return nil, errors.New("auth backend unavailable")
	}

	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{}

	_, err := setupTerraformAuth(&atmosConfig, &info)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrFailedToInitializeAuthManager), "expected ErrFailedToInitializeAuthManager but got: %v", err)
}

// TestSetupTerraformAuth_IdentityStoredAndManagerSet verifies that when the auth creator
// returns a non-nil AuthManager:
//   - info.Identity is set to the last element of the auth chain (auto-detection).
//   - info.AuthManager is populated with the returned manager.
//   - The returned manager matches what was injected.
func TestSetupTerraformAuth_IdentityStoredAndManagerSet(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockMgr := mockTypes.NewMockAuthManager(ctrl)
	mockMgr.EXPECT().GetChain().Return([]string{"base-role", "aws-dev"})

	orig := defaultAuthManagerCreator
	t.Cleanup(func() { defaultAuthManagerCreator = orig })
	defaultAuthManagerCreator = func(_ string, _ *schema.AuthConfig, _ string, _ *schema.AtmosConfiguration) (auth.AuthManager, error) {
		return mockMgr, nil
	}

	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{Identity: ""}

	mgr, err := setupTerraformAuth(&atmosConfig, &info)
	require.NoError(t, err)
	assert.Equal(t, mockMgr, mgr)
	// storeAutoDetectedIdentity takes the last element of the chain.
	assert.Equal(t, "aws-dev", info.Identity)
	assert.Equal(t, mockMgr, info.AuthManager)
}

// TestSetupTerraformAuth_NilManager_NoAuthBridge verifies that when the auth creator
// returns nil (no auth configured), info.AuthManager is left nil and the store auth-bridge
// is not injected (no panic on nil Stores).
func TestSetupTerraformAuth_NilManager_NoAuthBridge(t *testing.T) {
	orig := defaultAuthManagerCreator
	t.Cleanup(func() { defaultAuthManagerCreator = orig })
	defaultAuthManagerCreator = func(_ string, _ *schema.AuthConfig, _ string, _ *schema.AtmosConfiguration) (auth.AuthManager, error) {
		return nil, nil
	}

	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{}

	assert.NotPanics(t, func() {
		mgr, err := setupTerraformAuth(&atmosConfig, &info)
		require.NoError(t, err)
		assert.Nil(t, mgr)
		assert.Nil(t, info.AuthManager)
	})
}

// TestSetupTerraformAuth_MergedConfigError_WrapsWithInvalidAuthConfig verifies that
// when getMergedAuthConfig fails with an error that is NOT ErrInvalidComponent,
// the error is wrapped with ErrInvalidAuthConfig (matching createAndAuthenticateAuthManagerWithDeps).
func TestSetupTerraformAuth_MergedConfigError_WrapsWithInvalidAuthConfig(t *testing.T) {
	orig := defaultMergedAuthConfigGetter
	t.Cleanup(func() { defaultMergedAuthConfigGetter = orig })
	defaultMergedAuthConfigGetter = func(_ *schema.AtmosConfiguration, _ *schema.ConfigAndStacksInfo) (*schema.AuthConfig, error) {
		return nil, errors.New("config merge failure")
	}

	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: "mycomp"}

	_, err := setupTerraformAuth(&atmosConfig, &info)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidAuthConfig), "expected ErrInvalidAuthConfig, got: %v", err)
	assert.False(t, errors.Is(err, errUtils.ErrInvalidComponent))
}

// TestSetupTerraformAuth_ExportedWrapper verifies that the exported SetupTerraformAuth
// delegates to setupTerraformAuth and returns the same result. This ensures the cmd/terraform
// code path (used by --format) exercises the same auth logic as ExecuteTerraform.
func TestSetupTerraformAuth_ExportedWrapper(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockMgr := mockTypes.NewMockAuthManager(ctrl)
	mockMgr.EXPECT().GetChain().Return([]string{"test-identity"})

	orig := defaultAuthManagerCreator
	t.Cleanup(func() { defaultAuthManagerCreator = orig })
	defaultAuthManagerCreator = func(_ string, _ *schema.AuthConfig, _ string, _ *schema.AtmosConfiguration) (auth.AuthManager, error) {
		return mockMgr, nil
	}

	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{}

	mgr, err := SetupTerraformAuth(&atmosConfig, &info)
	require.NoError(t, err)
	assert.Equal(t, mockMgr, mgr)
	assert.Equal(t, mockMgr, info.AuthManager)
	assert.Equal(t, "test-identity", info.Identity)
}

// TestSetupTerraformAuth_ExportedWrapper_Error verifies that the exported SetupTerraformAuth
// propagates errors from setupTerraformAuth.
func TestSetupTerraformAuth_ExportedWrapper_Error(t *testing.T) {
	orig := defaultMergedAuthConfigGetter
	t.Cleanup(func() { defaultMergedAuthConfigGetter = orig })
	defaultMergedAuthConfigGetter = func(_ *schema.AtmosConfiguration, _ *schema.ConfigAndStacksInfo) (*schema.AuthConfig, error) {
		return nil, errors.New("config error")
	}

	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: "comp"}

	_, err := SetupTerraformAuth(&atmosConfig, &info)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidAuthConfig))
}

// TestStoreAutoDetectedIdentity_ExistingIdentity_NotOverwritten verifies that when
// info.Identity is already set, storeAutoDetectedIdentity returns early without
// calling GetChain() — preventing user-supplied identities from being overwritten.
func TestStoreAutoDetectedIdentity_ExistingIdentity_NotOverwritten(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockMgr := mockTypes.NewMockAuthManager(ctrl)
	// GetChain must NOT be called. If it is, the mock controller fails the test.
	mockMgr.EXPECT().GetChain().Times(0)

	info := schema.ConfigAndStacksInfo{Identity: "user-supplied-role"}
	storeAutoDetectedIdentity(mockMgr, &info)

	assert.Equal(t, "user-supplied-role", info.Identity, "pre-set identity must not be overwritten")
}
