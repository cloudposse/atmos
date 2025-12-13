package backend

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateCmd_Structure(t *testing.T) {
	testCommandStructure(t, commandTestParams{
		cmd:           createCmd,
		parser:        createParser,
		expectedUse:   "<component>",
		expectedShort: "Provision backend infrastructure",
		requiredFlags: []string{},
	})
}

func TestCreateCmd_Init(t *testing.T) {
	// Verify init() ran successfully by checking parser and flags are set up.
	assert.NotNil(t, createParser, "createParser should be initialized")
	assert.NotNil(t, createCmd, "createCmd should be initialized")
	assert.False(t, createCmd.DisableFlagParsing, "DisableFlagParsing should be false")

	// Verify flags are registered.
	stackFlag := createCmd.Flags().Lookup("stack")
	assert.NotNil(t, stackFlag, "stack flag should be registered")

	identityFlag := createCmd.Flags().Lookup("identity")
	assert.NotNil(t, identityFlag, "identity flag should be registered")
}
