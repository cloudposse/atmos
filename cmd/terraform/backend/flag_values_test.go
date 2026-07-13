package backend

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetCommandFlagString(t *testing.T) {
	t.Run("changed local flag returns its value", func(t *testing.T) {
		cmd := &cobra.Command{Use: "child"}
		cmd.Flags().String("stack", "", "stack")
		require.NoError(t, cmd.Flags().Set("stack", "dev"))

		assert.Equal(t, "dev", getCommandFlagString(cmd, "stack"))
	})

	t.Run("unchanged local flag returns empty", func(t *testing.T) {
		cmd := &cobra.Command{Use: "child"}
		cmd.Flags().String("stack", "", "stack")

		assert.Equal(t, "", getCommandFlagString(cmd, "stack"))
	})

	t.Run("changed inherited flag returns its value", func(t *testing.T) {
		parent := &cobra.Command{Use: "parent"}
		parent.PersistentFlags().String("stack", "", "stack")
		child := &cobra.Command{Use: "child"}
		parent.AddCommand(child)
		require.NoError(t, parent.PersistentFlags().Set("stack", "dev"))

		assert.Equal(t, "dev", getCommandFlagString(child, "stack"))
	})

	t.Run("missing flag returns empty", func(t *testing.T) {
		cmd := &cobra.Command{Use: "child"}

		assert.Equal(t, "", getCommandFlagString(cmd, "does-not-exist"))
	})
}

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
