package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestGetTerraformState_ErrorPaths tests error handling in GetTerraformState.
func TestGetTerraformState_ErrorPaths(t *testing.T) {
	t.Run("component not found in stack", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{}

		// Try to get state for non-existent component
		result, err := GetTerraformState(
			atmosConfig,
			"!terraform.state",
			"non-existent-stack",
			"non-existent-component",
			"output_name",
			false,
			nil,
			nil,
		)

		require.Error(t, err)
		assert.Nil(t, result)
		// Error message will vary based on stack configuration
		assert.Contains(t, err.Error(), "failed to")
	})

	t.Run("with auth manager that resolves successfully", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Setup mock AuthManager
		mockAuthManager := types.NewMockAuthManager(ctrl)
		mockAuthContext := &schema.AuthContext{
			AWS: &schema.AWSAuthContext{
				Profile: "test-profile",
				Region:  "us-east-1",
			},
		}
		mockStackInfo := &schema.ConfigAndStacksInfo{
			AuthContext: mockAuthContext,
			Stack:       "test",
		}
		mockAuthManager.EXPECT().
			GetStackInfo().
			Return(mockStackInfo).
			AnyTimes()

		atmosConfig := &schema.AtmosConfiguration{}

		// Try to get state for non-existent component with authManager
		result, err := GetTerraformState(
			atmosConfig,
			"!terraform.state",
			"non-existent-stack",
			"non-existent-component",
			"output_name",
			false,
			mockAuthContext,
			mockAuthManager,
		)

		require.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("invalid auth manager type", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{}

		// Pass invalid authManager type (string instead of AuthManager)
		result, err := GetTerraformState(
			atmosConfig,
			"!terraform.state",
			"test-stack",
			"test-component",
			"output_name",
			false,
			nil,
			"invalid-auth-manager-type",
		)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "expected auth.AuthManager")
	})
}

// TestGetTerraformOutput_ErrorPaths tests error handling in GetTerraformOutput.
func TestGetTerraformOutput_ErrorPaths(t *testing.T) {
	t.Run("component not found in stack", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{}

		// Try to get output for non-existent component
		result, exists, err := GetTerraformOutput(
			atmosConfig,
			"non-existent-stack",
			"non-existent-component",
			"output_name",
			false,
			nil,
			nil,
		)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.False(t, exists)
		// Error message will vary based on stack configuration
		assert.Contains(t, err.Error(), "failed to")
	})

	t.Run("with auth manager that resolves successfully", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Setup mock AuthManager
		mockAuthManager := types.NewMockAuthManager(ctrl)
		mockAuthContext := &schema.AuthContext{
			AWS: &schema.AWSAuthContext{
				Profile: "test-profile",
				Region:  "us-east-1",
			},
		}
		mockStackInfo := &schema.ConfigAndStacksInfo{
			AuthContext: mockAuthContext,
			Stack:       "test",
		}
		mockAuthManager.EXPECT().
			GetStackInfo().
			Return(mockStackInfo).
			AnyTimes()

		atmosConfig := &schema.AtmosConfiguration{}

		// Try to get output for non-existent component with authManager
		result, exists, err := GetTerraformOutput(
			atmosConfig,
			"non-existent-stack",
			"non-existent-component",
			"output_name",
			false,
			mockAuthContext,
			mockAuthManager,
		)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.False(t, exists)
	})

	t.Run("invalid auth manager type", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{}

		// Pass invalid authManager type (string instead of AuthManager)
		result, exists, err := GetTerraformOutput(
			atmosConfig,
			"test-stack",
			"test-component",
			"output_name",
			false,
			nil,
			"invalid-auth-manager-type",
		)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.False(t, exists)
		assert.Contains(t, err.Error(), "expected auth.AuthManager")
	})
}

// TestGetTerraformState_CacheHit tests cache hit behavior.
func TestGetTerraformState_CacheHit(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	// Pre-populate the cache with test data
	stackSlug := "test-stack-test-component"
	cachedBackend := map[string]any{
		"vpc_id":    "vpc-12345",
		"subnet_id": "subnet-67890",
	}
	terraformStateCache.Store(stackSlug, cachedBackend)
	defer terraformStateCache.Delete(stackSlug) // Cleanup

	// Get cached output (skipCache = false)
	result, err := GetTerraformState(
		atmosConfig,
		"!terraform.state",
		"test-stack",
		"test-component",
		"vpc_id",
		false,
		nil,
		nil,
	)

	require.NoError(t, err)
	assert.Equal(t, "vpc-12345", result)
}

func TestGetTerraformState_CachesNotProvisionedState(t *testing.T) {
	ResetStateCache()
	t.Cleanup(ResetStateCache)

	setupTerraformYamlFunctionSandbox(t, "../../tests/fixtures/scenarios/terraform-state-jit-workdir")
	t.Chdir("../../tests/fixtures/scenarios/terraform-state-jit-workdir")

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	result, err := GetTerraformState(
		&atmosConfig,
		"!terraform.state",
		"test",
		"consumer",
		"vpc_id",
		false,
		nil,
		nil,
	)

	assert.Nil(t, result)
	assert.ErrorIs(t, err, errUtils.ErrTerraformStateNotProvisioned)

	cached, found := terraformStateCache.Load("test-consumer")
	require.True(t, found)
	_, isNotProvisioned := cached.(terraformStateNotProvisionedCacheEntry)
	require.True(t, isNotProvisioned)

	result, err = GetTerraformState(
		&atmosConfig,
		"!terraform.state",
		"test",
		"consumer",
		"vpc_id",
		false,
		nil,
		nil,
	)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, errUtils.ErrTerraformStateNotProvisioned)
}

// TestGetTerraformState_InvalidateClearsNotProvisionedSentinel is the negative path for
// the not-provisioned cache: invalidateTerraformStateCache (run by ExecuteTerraform after
// a successful apply/deploy) must remove the cached sentinel so the next read re-consults
// the backend instead of serving a frozen "not provisioned" answer, while invalidating a
// different stack or component must leave the sentinel in place.
func TestGetTerraformState_InvalidateClearsNotProvisionedSentinel(t *testing.T) {
	ResetStateCache()
	t.Cleanup(ResetStateCache)

	setupTerraformYamlFunctionSandbox(t, "../../tests/fixtures/scenarios/terraform-state-jit-workdir")
	t.Chdir("../../tests/fixtures/scenarios/terraform-state-jit-workdir")

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	// Seed the cache with the not-provisioned sentinel, mirroring
	// TestGetTerraformState_CachesNotProvisionedState.
	result, err := GetTerraformState(
		&atmosConfig,
		"!terraform.state",
		"test",
		"consumer",
		"vpc_id",
		false,
		nil,
		nil,
	)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, errUtils.ErrTerraformStateNotProvisioned)

	cached, found := terraformStateCache.Load("test-consumer")
	require.True(t, found)
	_, isNotProvisioned := cached.(terraformStateNotProvisionedCacheEntry)
	require.True(t, isNotProvisioned)

	// Invalidating a different stack or component must NOT clear the sentinel.
	invalidateTerraformStateCache("other-stack", "consumer")
	invalidateTerraformStateCache("test", "other-component")
	_, found = terraformStateCache.Load("test-consumer")
	require.True(t, found, "mismatched stack/component invalidation must leave the sentinel cached")

	// Invalidating the matching stack/component removes the frozen sentinel.
	invalidateTerraformStateCache("test", "consumer")
	_, found = terraformStateCache.Load("test-consumer")
	require.False(t, found, "matching invalidation must remove the not-provisioned sentinel from the cache")

	// A subsequent read finds no cache entry, re-consults the backend (still not
	// provisioned in this fixture), and re-populates the sentinel — proving the
	// answer came from the backend rather than a frozen cache entry.
	result, err = GetTerraformState(
		&atmosConfig,
		"!terraform.state",
		"test",
		"consumer",
		"vpc_id",
		false,
		nil,
		nil,
	)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, errUtils.ErrTerraformStateNotProvisioned)

	cached, found = terraformStateCache.Load("test-consumer")
	require.True(t, found, "the backend read after invalidation must re-cache the sentinel")
	_, isNotProvisioned = cached.(terraformStateNotProvisionedCacheEntry)
	require.True(t, isNotProvisioned)
}

// TestGetTerraformState_SkipCache tests cache skip behavior.
func TestGetTerraformState_SkipCache(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	// Pre-populate the cache with test data
	stackSlug := "test-stack-cached-component"
	cachedBackend := map[string]any{
		"vpc_id": "vpc-cached",
	}
	terraformStateCache.Store(stackSlug, cachedBackend)
	defer terraformStateCache.Delete(stackSlug) // Cleanup

	// Try to get output with skipCache=true (should not use cache)
	result, err := GetTerraformState(
		atmosConfig,
		"!terraform.state",
		"test-stack",
		"cached-component",
		"vpc_id",
		true,
		nil,
		nil,
	)

	// Should fail because component doesn't actually exist (cache was skipped)
	require.Error(t, err)
	assert.Nil(t, result)
}
