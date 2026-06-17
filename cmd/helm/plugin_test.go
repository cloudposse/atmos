package helm

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// findSubcommand returns the named direct subcommand of parent, or nil.
func findSubcommand(parent *cobra.Command, name string) *cobra.Command {
	for _, c := range parent.Commands() {
		if c.Name() == name {
			return c
		}
	}
	return nil
}

func TestPluginCommandStructure(t *testing.T) {
	pluginCmd := findSubcommand(helmCmd, "plugin")
	require.NotNil(t, pluginCmd, "helm should have a plugin subcommand")

	var subs []string
	for _, c := range pluginCmd.Commands() {
		subs = append(subs, c.Name())
	}
	assert.ElementsMatch(t, []string{"list", "install"}, subs)

	installCmd := findSubcommand(pluginCmd, "install")
	require.NotNil(t, installCmd)
	assert.NotNil(t, installCmd.Flag(flagComponent), "install should expose --component")
}

func TestCollectInstallSpecs(t *testing.T) {
	t.Run("explicit args take precedence", func(t *testing.T) {
		cmd := &cobra.Command{Use: "install"}
		cmd.Flags().String(flagComponent, "", "")
		specs, err := collectInstallSpecs(cmd, []string{"diff@v3.9.4", "secrets"})
		require.NoError(t, err)
		assert.Equal(t, []string{"diff@v3.9.4", "secrets"}, specs)
	})

	t.Run("no args and no component returns nil", func(t *testing.T) {
		cmd := &cobra.Command{Use: "install"}
		cmd.Flags().String(flagComponent, "", "")
		specs, err := collectInstallSpecs(cmd, nil)
		require.NoError(t, err)
		assert.Nil(t, specs)
	})
}
