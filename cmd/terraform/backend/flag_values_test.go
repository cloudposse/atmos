package backend

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetCommandFlagBool(t *testing.T) {
	t.Run("changed local flag returns value and true", func(t *testing.T) {
		cmd := &cobra.Command{Use: "child"}
		cmd.Flags().Bool("force", false, "force")
		require.NoError(t, cmd.Flags().Set("force", "true"))

		value, changed := getCommandFlagBool(cmd, "force")
		assert.True(t, changed)
		assert.True(t, value)
	})

	t.Run("unchanged local flag returns false, false", func(t *testing.T) {
		cmd := &cobra.Command{Use: "child"}
		cmd.Flags().Bool("force", false, "force")

		value, changed := getCommandFlagBool(cmd, "force")
		assert.False(t, changed)
		assert.False(t, value)
	})

	t.Run("changed inherited flag returns value and true", func(t *testing.T) {
		parent := &cobra.Command{Use: "parent"}
		parent.PersistentFlags().Bool("force", false, "force")
		child := &cobra.Command{Use: "child"}
		parent.AddCommand(child)
		require.NoError(t, parent.PersistentFlags().Set("force", "true"))

		value, changed := getCommandFlagBool(child, "force")
		assert.True(t, changed)
		assert.True(t, value)
	})

	t.Run("missing flag returns false, false", func(t *testing.T) {
		cmd := &cobra.Command{Use: "child"}

		value, changed := getCommandFlagBool(cmd, "does-not-exist")
		assert.False(t, changed)
		assert.False(t, value)
	})
}
