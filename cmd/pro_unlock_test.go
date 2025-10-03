package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestProUnlockCmd tests the pro unlock command initialization.
func TestProUnlockCmd(t *testing.T) {
	t.Run("unlock command is properly initialized", func(t *testing.T) {
		assert.NotNil(t, proUnlockCmd)
		assert.Equal(t, "unlock", proUnlockCmd.Use)
		assert.Contains(t, proUnlockCmd.Short, "Unlock")
		assert.False(t, proUnlockCmd.FParseErrWhitelist.UnknownFlags)
	})

	t.Run("unlock command has required flags", func(t *testing.T) {
		componentFlag := proUnlockCmd.PersistentFlags().Lookup("component")
		assert.NotNil(t, componentFlag)
		assert.Equal(t, "c", componentFlag.Shorthand)

		stackFlag := proUnlockCmd.PersistentFlags().Lookup("stack")
		assert.NotNil(t, stackFlag)
		assert.Equal(t, "s", stackFlag.Shorthand)
	})

	t.Run("unlock command has default values", func(t *testing.T) {
		componentFlag := proUnlockCmd.PersistentFlags().Lookup("component")
		assert.Equal(t, "", componentFlag.DefValue)
	})
}
