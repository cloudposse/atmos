package toolchain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Provider and structure tests are in command_provider_test.go.
// This file contains path-specific flag tests.

func TestPathCommand_Flags(t *testing.T) {
	t.Run("has export flag", func(t *testing.T) {
		flag := pathCmd.Flags().Lookup("export")
		require.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})

	t.Run("has json flag", func(t *testing.T) {
		flag := pathCmd.Flags().Lookup("json")
		require.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})

	t.Run("has relative flag", func(t *testing.T) {
		flag := pathCmd.Flags().Lookup("relative")
		require.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})
}

func TestPathCommand_FlagDescriptions(t *testing.T) {
	tests := []struct {
		flagName string
		contains string
	}{
		{"export", "export"},
		{"json", "JSON"},
		{"relative", "relative"},
	}

	for _, tt := range tests {
		t.Run(tt.flagName+" has description", func(t *testing.T) {
			flag := pathCmd.Flags().Lookup(tt.flagName)
			require.NotNil(t, flag)
			assert.Contains(t, flag.Usage, tt.contains)
		})
	}
}
