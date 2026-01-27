package skill

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUninstallCmd_BasicProperties(t *testing.T) {
	assert.Equal(t, "uninstall <name>", uninstallCmd.Use)
	assert.Equal(t, "Remove an installed skill", uninstallCmd.Short)
	assert.NotEmpty(t, uninstallCmd.Long)
	assert.NotNil(t, uninstallCmd.RunE)
}

func TestUninstallCmd_Flags(t *testing.T) {
	t.Run("has force flag with shorthand", func(t *testing.T) {
		flag := uninstallCmd.Flags().Lookup("force")
		require.NotNil(t, flag, "force flag should be registered")
		assert.Equal(t, "bool", flag.Value.Type())
		assert.Equal(t, "false", flag.DefValue)
		assert.Equal(t, "f", flag.Shorthand)
	})
}

func TestUninstallCmd_LongDescription(t *testing.T) {
	// Verify long description contains important information.
	assert.Contains(t, uninstallCmd.Long, "Uninstall a community-contributed skill")
	assert.Contains(t, uninstallCmd.Long, "~/.atmos/skills/")
	assert.Contains(t, uninstallCmd.Long, "registry entry")
	assert.Contains(t, uninstallCmd.Long, "prompted to confirm")
	assert.Contains(t, uninstallCmd.Long, "--force")
}

func TestUninstallCmd_ArgsValidation(t *testing.T) {
	// The command expects exactly 1 argument.
	assert.NotNil(t, uninstallCmd.Args)
}

func TestUninstallCmd_Examples(t *testing.T) {
	// Verify the long description contains examples.
	assert.Contains(t, uninstallCmd.Long, "atmos ai skill uninstall terraform-optimizer")
	assert.Contains(t, uninstallCmd.Long, "--force")
}

func TestUninstallCmd_ReferencesListCommand(t *testing.T) {
	// Verify it references the list command for finding skill names.
	assert.Contains(t, uninstallCmd.Long, "atmos ai skill list")
}
