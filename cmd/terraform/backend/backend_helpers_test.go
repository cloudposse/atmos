package backend

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestCreateDescribeComponentFunc(t *testing.T) {
	t.Run("creates function with nil auth", func(t *testing.T) {
		// Create the describe function with nil auth manager.
		describeFunc := CreateDescribeComponentFunc(nil)

		// Verify it returns a non-nil function.
		assert.NotNil(t, describeFunc)
	})

	t.Run("returned function reaches ExecuteDescribeComponent", func(t *testing.T) {
		// Run from an empty directory with no Atmos config, so this stays a fast, isolated
		// unit test: it doesn't need a real stack to prove the closure actually invokes
		// ExecuteDescribeComponent (rather than just verifying it's non-nil).
		t.Chdir(t.TempDir())

		describeFunc := CreateDescribeComponentFunc(nil)
		_, err := describeFunc("nonexistent-component", "nonexistent-stack")

		assert.Error(t, err)
	})
}

func TestInitConfigAndAuth_FailsFastWithoutRealConfig(t *testing.T) {
	// Run from an empty directory with no Atmos config and a component/stack that can't
	// exist, so this stays a fast, isolated unit test that still exercises the real
	// InitCliConfig -> ExecuteDescribeComponent wiring (rather than mocking it away).
	t.Chdir(t.TempDir())

	atmosConfig, authContext, err := InitConfigAndAuth("nonexistent-component", "nonexistent-stack", "")

	assert.Error(t, err)
	assert.Nil(t, atmosConfig)
	assert.Nil(t, authContext)
}

func TestDefaultConfigInitializer_InitConfigAndAuth(t *testing.T) {
	t.Chdir(t.TempDir())

	ci := &defaultConfigInitializer{}
	atmosConfig, authContext, err := ci.InitConfigAndAuth("nonexistent-component", "nonexistent-stack", "")

	assert.Error(t, err)
	assert.Nil(t, atmosConfig)
	assert.Nil(t, authContext)
}

func TestInitConfigAndAuth_SucceedsWithNoAuthConfigured(t *testing.T) {
	// This fixture has no `auth:` key anywhere (atmos.yaml or stack files), so
	// InitConfigAndAuth runs its full real (unmocked) body: InitCliConfig ->
	// ExecuteDescribeComponent -> MergeComponentAuthFromConfig -> the stack-aware
	// auth call, which hits the early "no identity resolved" return since no
	// identities are configured.
	t.Chdir(filepath.Join("..", "..", "..", "tests", "fixtures", "scenarios", "atmos-overrides-section"))
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	atmosConfig, authContext, err := InitConfigAndAuth("c1", "dev", "")

	require.NoError(t, err)
	require.NotNil(t, atmosConfig)
	assert.Nil(t, authContext)
}

func TestInitConfigAndAuth_FailsWhenAuthConfiguredButIdentityMissing(t *testing.T) {
	// Same fixture (no auth configured), but pass an explicit identity name. Since
	// authConfig still has zero identities configured, resolveIdentityName returns
	// the identity as-is, and CreateAndAuthenticateManagerWithAtmosConfigForStack
	// then hits the ErrAuthNotConfigured branch.
	t.Chdir(filepath.Join("..", "..", "..", "tests", "fixtures", "scenarios", "atmos-overrides-section"))
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	atmosConfig, authContext, err := InitConfigAndAuth("c1", "dev", "nonexistent-identity")

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAuthNotConfigured)
	assert.Nil(t, atmosConfig)
	assert.Nil(t, authContext)
}

func TestSetConfigInitializer_NilResetsToDefault(t *testing.T) {
	t.Cleanup(ResetDependencies)

	// Override with a non-default value first so the nil reset is observable.
	SetConfigInitializer(NewMockConfigInitializer(nil))
	assert.IsType(t, &MockConfigInitializer{}, configInit)

	SetConfigInitializer(nil)

	assert.IsType(t, &defaultConfigInitializer{}, configInit)
}

func TestSetProvisioner_NilResetsToDefault(t *testing.T) {
	t.Cleanup(ResetDependencies)

	// Override with a non-default value first so the nil reset is observable.
	SetProvisioner(NewMockProvisioner(nil))
	assert.IsType(t, &MockProvisioner{}, prov)

	SetProvisioner(nil)

	assert.IsType(t, &defaultProvisioner{}, prov)
}
