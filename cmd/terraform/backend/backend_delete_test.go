package backend

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDeleteCmd_Structure(t *testing.T) {
	testCommandStructure(t, commandTestParams{
		cmd:           deleteCmd,
		parser:        deleteParser,
		expectedUse:   "delete <component>",
		expectedShort: "Delete backend infrastructure",
		requiredFlags: []string{"force"},
	})

	t.Run("force flag is boolean", func(t *testing.T) {
		forceFlag := deleteCmd.Flags().Lookup("force")
		assert.NotNil(t, forceFlag, "force flag should be registered")
		assert.Equal(t, "bool", forceFlag.Value.Type())
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

func TestDeleteCmd_Init(t *testing.T) {
	// Verify init() ran successfully by checking parser and flags are set up.
	assert.NotNil(t, deleteParser, "deleteParser should be initialized")
	assert.NotNil(t, deleteCmd, "deleteCmd should be initialized")
	assert.False(t, deleteCmd.DisableFlagParsing, "DisableFlagParsing should be false")

	// Verify flags are registered.
	stackFlag := deleteCmd.Flags().Lookup("stack")
	assert.NotNil(t, stackFlag, "stack flag should be registered")

	identityFlag := deleteCmd.Flags().Lookup("identity")
	assert.NotNil(t, identityFlag, "identity flag should be registered")

	forceFlag := deleteCmd.Flags().Lookup("force")
	assert.NotNil(t, forceFlag, "force flag should be registered")
}
