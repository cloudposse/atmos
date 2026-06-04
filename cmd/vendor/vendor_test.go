package vendor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVendorPullCmd_ExecutorError tests that vendor pull executor handles unexpected args.
func TestVendorPullCmd_ExecutorError(t *testing.T) {
	stacksPath := "../../tests/fixtures/scenarios/terraform-apply-affected"

	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	err := vendorPullCmd.RunE(vendorPullCmd, []string{"unexpected-arg"})
	assert.Error(t, err, "vendor pull command should return an error with unexpected arguments")
}

// TestVendorCommandProvider tests the VendorCommandProvider interface methods.
func TestVendorCommandProvider(t *testing.T) {
	provider := &VendorCommandProvider{}

	t.Run("GetCommand returns vendorCmd", func(t *testing.T) {
		cmd := provider.GetCommand()
		require.NotNil(t, cmd)
		assert.Equal(t, "vendor", cmd.Use)
	})

	t.Run("GetName returns vendor", func(t *testing.T) {
		assert.Equal(t, "vendor", provider.GetName())
	})

	t.Run("GetGroup returns Component Lifecycle", func(t *testing.T) {
		assert.Equal(t, "Component Lifecycle", provider.GetGroup())
	})

	t.Run("GetFlagsBuilder returns nil", func(t *testing.T) {
		assert.Nil(t, provider.GetFlagsBuilder())
	})

	t.Run("GetPositionalArgsBuilder returns nil", func(t *testing.T) {
		assert.Nil(t, provider.GetPositionalArgsBuilder())
	})

	t.Run("GetCompatibilityFlags returns nil", func(t *testing.T) {
		assert.Nil(t, provider.GetCompatibilityFlags())
	})

	t.Run("GetAliases returns nil", func(t *testing.T) {
		assert.Nil(t, provider.GetAliases())
	})

	t.Run("IsExperimental returns false", func(t *testing.T) {
		assert.False(t, provider.IsExperimental())
	})
}
