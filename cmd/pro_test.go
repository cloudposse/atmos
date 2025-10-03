package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestProCmd tests the pro command initialization.
func TestProCmd(t *testing.T) {
	t.Run("pro command is properly initialized", func(t *testing.T) {
		assert.NotNil(t, proCmd)
		assert.Equal(t, "pro", proCmd.Use)
		assert.Contains(t, proCmd.Short, "premium features")
		assert.False(t, proCmd.FParseErrWhitelist.UnknownFlags)
	})
}
