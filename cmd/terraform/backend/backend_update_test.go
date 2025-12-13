package backend

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUpdateCmd_Structure(t *testing.T) {
	testCommandStructure(t, commandTestParams{
		cmd:           updateCmd,
		parser:        updateParser,
		expectedUse:   "update <component>",
		expectedShort: "Update backend configuration",
		requiredFlags: []string{},
	})
}

func TestUpdateCmd_Init(t *testing.T) {
	// Verify init() ran successfully by checking parser and flags are set up.
	assert.NotNil(t, updateParser, "updateParser should be initialized")
	assert.NotNil(t, updateCmd, "updateCmd should be initialized")
	assert.False(t, updateCmd.DisableFlagParsing, "DisableFlagParsing should be false")

	// Verify flags are registered.
	stackFlag := updateCmd.Flags().Lookup("stack")
	assert.NotNil(t, stackFlag, "stack flag should be registered")

	identityFlag := updateCmd.Flags().Lookup("identity")
	assert.NotNil(t, identityFlag, "identity flag should be registered")
}
