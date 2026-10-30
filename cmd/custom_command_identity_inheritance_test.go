package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestCustomCommandIdentityPreservesInheritedShorthand(t *testing.T) {
	for _, nested := range []bool{false, true} {
		t.Run(map[bool]string{false: "parent", true: "ancestor"}[nested], func(t *testing.T) {
			root := &cobra.Command{Use: "root"}
			root.PersistentFlags().StringP("inventory", "i", "", "inventory")
			parent := root
			if nested {
				parent = &cobra.Command{Use: "parent"}
				root.AddCommand(parent)
			}
			command, err := createCustomCommand(&schema.AtmosConfiguration{}, &schema.Command{Name: "run"}, parent)
			require.NoError(t, err)
			parent.AddCommand(command)
			require.NotPanics(t, func() {
				require.NoError(t, command.ParseFlags([]string{"-i", "hosts.ini", "--identity=role"}))
			})
			inventory, err := command.Flags().GetString("inventory")
			require.NoError(t, err)
			require.Equal(t, "hosts.ini", inventory)
			identity, err := command.Flags().GetString("identity")
			require.NoError(t, err)
			require.Equal(t, "role", identity)
			require.NoError(t, command.ParseFlags([]string{"--identity"}))
			identity, err = command.Flags().GetString("identity")
			require.NoError(t, err)
			require.Equal(t, cfg.IdentityFlagSelectValue, identity)
		})
	}
	// An unrelated command retains the normal shorthand: registration must not
	// mutate the shared identity flag definition.
	parent := &cobra.Command{Use: "other"}
	command, err := createCustomCommand(&schema.AtmosConfiguration{}, &schema.Command{Name: "run"}, parent)
	require.NoError(t, err)
	parent.AddCommand(command)
	require.NoError(t, command.ParseFlags([]string{"-i=other-role"}))
	identity, err := command.Flags().GetString("identity")
	require.NoError(t, err)
	require.Equal(t, "other-role", identity)
}
