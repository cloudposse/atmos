package toolchain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Provider and structure tests are in command_provider_test.go.
// This file contains get-specific tests.

func TestGetCommand_Flags(t *testing.T) {
	t.Run("has all flag", func(t *testing.T) {
		flag := getCmd.Flags().Lookup("all")
		require.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})

	t.Run("has limit flag", func(t *testing.T) {
		flag := getCmd.Flags().Lookup("limit")
		require.NotNil(t, flag)
		assert.Equal(t, "10", flag.DefValue)
	})
}

func TestGetCommand_FlagDescriptions(t *testing.T) {
	tests := []struct {
		flagName string
		contains string
	}{
		{"all", "all"},
		{"limit", "Limit"},
	}

	for _, tt := range tests {
		t.Run(tt.flagName+" has description", func(t *testing.T) {
			flag := getCmd.Flags().Lookup(tt.flagName)
			require.NotNil(t, flag)
			assert.Contains(t, flag.Usage, tt.contains)
		})
	}
}

func TestGetCommand_Args(t *testing.T) {
	t.Run("accepts zero arguments", func(t *testing.T) {
		err := getCmd.Args(getCmd, []string{})
		assert.NoError(t, err)
	})

	t.Run("accepts one argument", func(t *testing.T) {
		err := getCmd.Args(getCmd, []string{"terraform"})
		assert.NoError(t, err)
	})

	t.Run("rejects two arguments", func(t *testing.T) {
		err := getCmd.Args(getCmd, []string{"terraform", "helm"})
		assert.Error(t, err)
	})
}

func TestGetCommand_DefaultVersionLimit(t *testing.T) {
	assert.Equal(t, 10, defaultVersionLimit)
}
