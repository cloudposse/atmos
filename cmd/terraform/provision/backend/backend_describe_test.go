package backend

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDescribeCmd_Structure(t *testing.T) {
	testCommandStructure(t, commandTestParams{
		cmd:           describeCmd,
		parser:        describeParser,
		expectedUse:   "describe <component>",
		expectedShort: "Describe backend configuration",
		requiredFlags: []string{"format"},
	})

	t.Run("format flag is string", func(t *testing.T) {
		formatFlag := describeCmd.Flags().Lookup("format")
		assert.NotNil(t, formatFlag, "format flag should be registered")
		assert.Equal(t, "string", formatFlag.Value.Type())
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
