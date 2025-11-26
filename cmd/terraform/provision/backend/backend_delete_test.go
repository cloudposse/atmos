package backend

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDeleteCmd_Structure(t *testing.T) {
	t.Run("command is properly configured", func(t *testing.T) {
		assert.NotNil(t, deleteCmd)
		assert.Equal(t, "delete <component>", deleteCmd.Use)
		assert.Equal(t, "Delete backend infrastructure", deleteCmd.Short)
		assert.NotEmpty(t, deleteCmd.Long)
		assert.NotEmpty(t, deleteCmd.Example)
		assert.False(t, deleteCmd.DisableFlagParsing)
	})

	t.Run("parser is configured with required flags", func(t *testing.T) {
		assert.NotNil(t, deleteParser)

		// Verify force flag exists.
		forceFlag := deleteCmd.Flags().Lookup("force")
		assert.NotNil(t, forceFlag, "force flag should be registered")
		assert.Equal(t, "bool", forceFlag.Value.Type())

		// Verify stack flag exists.
		stackFlag := deleteCmd.Flags().Lookup("stack")
		assert.NotNil(t, stackFlag, "stack flag should be registered")

		// Verify identity flag exists.
		identityFlag := deleteCmd.Flags().Lookup("identity")
		assert.NotNil(t, identityFlag, "identity flag should be registered")
	})

	t.Run("command requires exactly one argument", func(t *testing.T) {
		// The Args field should be set to cobra.ExactArgs(1).
		assert.NotNil(t, deleteCmd.Args)

		// Test with no args.
		err := deleteCmd.Args(deleteCmd, []string{})
		assert.Error(t, err, "should error with no arguments")

		// Test with one arg.
		err = deleteCmd.Args(deleteCmd, []string{"vpc"})
		assert.NoError(t, err, "should accept exactly one argument")

		// Test with multiple args.
		err = deleteCmd.Args(deleteCmd, []string{"vpc", "extra"})
		assert.Error(t, err, "should error with multiple arguments")
	})
}

func TestDeleteCmd_FlagDefaults(t *testing.T) {
	tests := []struct {
		name         string
		flagName     string
		expectedType string
		hasDefault   bool
	}{
		{
			name:         "force flag is boolean",
			flagName:     "force",
			expectedType: "bool",
			hasDefault:   true,
		},
		{
			name:         "stack flag is string",
			flagName:     "stack",
			expectedType: "string",
			hasDefault:   true,
		},
		{
			name:         "identity flag is string",
			flagName:     "identity",
			expectedType: "string",
			hasDefault:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := deleteCmd.Flags().Lookup(tt.flagName)
			assert.NotNil(t, flag, "flag %s should exist", tt.flagName)
			assert.Equal(t, tt.expectedType, flag.Value.Type())
		})
	}
}
