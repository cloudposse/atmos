package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestProLockCmd tests the pro lock command initialization.
func TestProLockCmd(t *testing.T) {
	t.Run("lock command is properly initialized", func(t *testing.T) {
		assert.NotNil(t, proLockCmd)
		assert.Equal(t, "lock", proLockCmd.Use)
		assert.Contains(t, proLockCmd.Short, "Lock")
		assert.False(t, proLockCmd.FParseErrWhitelist.UnknownFlags)
	})

	t.Run("lock command has required flags", func(t *testing.T) {
		componentFlag := proLockCmd.PersistentFlags().Lookup("component")
		assert.NotNil(t, componentFlag)
		assert.Equal(t, "c", componentFlag.Shorthand)

		stackFlag := proLockCmd.PersistentFlags().Lookup("stack")
		assert.NotNil(t, stackFlag)
		assert.Equal(t, "s", stackFlag.Shorthand)

		messageFlag := proLockCmd.PersistentFlags().Lookup("message")
		assert.NotNil(t, messageFlag)
		assert.Equal(t, "m", messageFlag.Shorthand)

		ttlFlag := proLockCmd.PersistentFlags().Lookup("ttl")
		assert.NotNil(t, ttlFlag)
		assert.Equal(t, "t", ttlFlag.Shorthand)
	})

	t.Run("lock command has default values", func(t *testing.T) {
		componentFlag := proLockCmd.PersistentFlags().Lookup("component")
		assert.Equal(t, "", componentFlag.DefValue)

		messageFlag := proLockCmd.PersistentFlags().Lookup("message")
		assert.Equal(t, "", messageFlag.DefValue)

		ttlFlag := proLockCmd.PersistentFlags().Lookup("ttl")
		assert.Equal(t, "0", ttlFlag.DefValue)
	})
}
