package backend

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
