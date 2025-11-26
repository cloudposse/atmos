package backend

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDescribeCmd_Structure(t *testing.T) {
	t.Run("command is properly configured", func(t *testing.T) {
		assert.NotNil(t, describeCmd)
		assert.Equal(t, "describe <component>", describeCmd.Use)
		assert.Equal(t, "Describe backend configuration", describeCmd.Short)
		assert.NotEmpty(t, describeCmd.Long)
		assert.NotEmpty(t, describeCmd.Example)
		assert.False(t, describeCmd.DisableFlagParsing)
	})

	t.Run("parser is configured with required flags", func(t *testing.T) {
		assert.NotNil(t, describeParser)

		// Verify format flag exists.
		formatFlag := describeCmd.Flags().Lookup("format")
		assert.NotNil(t, formatFlag, "format flag should be registered")
		assert.Equal(t, "string", formatFlag.Value.Type())

		// Verify stack flag exists.
		stackFlag := describeCmd.Flags().Lookup("stack")
		assert.NotNil(t, stackFlag, "stack flag should be registered")

		// Verify identity flag exists.
		identityFlag := describeCmd.Flags().Lookup("identity")
		assert.NotNil(t, identityFlag, "identity flag should be registered")
	})

	t.Run("command requires exactly one argument", func(t *testing.T) {
		// The Args field should be set to cobra.ExactArgs(1).
		assert.NotNil(t, describeCmd.Args)

		// Test with no args.
		err := describeCmd.Args(describeCmd, []string{})
		assert.Error(t, err, "should error with no arguments")

		// Test with one arg.
		err = describeCmd.Args(describeCmd, []string{"vpc"})
		assert.NoError(t, err, "should accept exactly one argument")

		// Test with multiple args.
		err = describeCmd.Args(describeCmd, []string{"vpc", "extra"})
		assert.Error(t, err, "should error with multiple arguments")
	})
}

func TestDescribeCmd_FlagDefaults(t *testing.T) {
	tests := []struct {
		name          string
		flagName      string
		expectedType  string
		expectedValue string
	}{
		{
			name:          "format flag has yaml default",
			flagName:      "format",
			expectedType:  "string",
			expectedValue: "yaml",
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
			flag := describeCmd.Flags().Lookup(tt.flagName)
			assert.NotNil(t, flag, "flag %s should exist", tt.flagName)
			assert.Equal(t, tt.expectedType, flag.Value.Type())
			assert.Equal(t, tt.expectedValue, flag.DefValue)
		})
	}
}

func TestDescribeCmd_Shorthand(t *testing.T) {
	t.Run("format flag has shorthand", func(t *testing.T) {
		flag := describeCmd.Flags().Lookup("format")
		assert.NotNil(t, flag)
		assert.Equal(t, "f", flag.Shorthand, "format flag should have 'f' shorthand")
	})
}
