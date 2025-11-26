package backend

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestListCmd_Structure(t *testing.T) {
	t.Run("command is properly configured", func(t *testing.T) {
		assert.NotNil(t, listCmd)
		assert.Equal(t, "list", listCmd.Use)
		assert.Equal(t, "List all backends in stack", listCmd.Short)
		assert.NotEmpty(t, listCmd.Long)
		assert.NotEmpty(t, listCmd.Example)
		assert.False(t, listCmd.DisableFlagParsing)
	})

	t.Run("parser is configured with required flags", func(t *testing.T) {
		assert.NotNil(t, listParser)

		// Verify format flag exists.
		formatFlag := listCmd.Flags().Lookup("format")
		assert.NotNil(t, formatFlag, "format flag should be registered")
		assert.Equal(t, "string", formatFlag.Value.Type())

		// Verify stack flag exists.
		stackFlag := listCmd.Flags().Lookup("stack")
		assert.NotNil(t, stackFlag, "stack flag should be registered")

		// Verify identity flag exists.
		identityFlag := listCmd.Flags().Lookup("identity")
		assert.NotNil(t, identityFlag, "identity flag should be registered")
	})

	t.Run("command accepts no arguments", func(t *testing.T) {
		// The Args field should be set to cobra.NoArgs.
		assert.NotNil(t, listCmd.Args)

		// Test with no args (should succeed).
		err := listCmd.Args(listCmd, []string{})
		assert.NoError(t, err, "should accept no arguments")

		// Test with args (should fail).
		err = listCmd.Args(listCmd, []string{"extra"})
		assert.Error(t, err, "should error with arguments")
	})
}

func TestListCmd_FlagDefaults(t *testing.T) {
	tests := []struct {
		name          string
		flagName      string
		expectedType  string
		expectedValue string
	}{
		{
			name:          "format flag has table default",
			flagName:      "format",
			expectedType:  "string",
			expectedValue: "table",
		},
		{
			name:          "stack flag is string",
			flagName:      "stack",
			expectedType:  "string",
			expectedValue: "",
		},
		{
			name:          "identity flag is string",
			flagName:      "identity",
			expectedType:  "string",
			expectedValue: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := listCmd.Flags().Lookup(tt.flagName)
			assert.NotNil(t, flag, "flag %s should exist", tt.flagName)
			assert.Equal(t, tt.expectedType, flag.Value.Type())
			assert.Equal(t, tt.expectedValue, flag.DefValue)
		})
	}
}

func TestListCmd_Shorthand(t *testing.T) {
	t.Run("format flag has shorthand", func(t *testing.T) {
		flag := listCmd.Flags().Lookup("format")
		assert.NotNil(t, flag)
		assert.Equal(t, "f", flag.Shorthand, "format flag should have 'f' shorthand")
	})
}
