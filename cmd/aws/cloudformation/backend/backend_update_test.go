package backend

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUpdateCmd_Structure(t *testing.T) {
	testCommandStructure(t, &commandTestParams{
		cmd:              updateCmd,
		parser:           updateParser,
		expectedUse:      "update [component]",
		expectedShort:    "Update the template-packaging backend bucket",
		requiredFlags:    []string{"target"},
		hasPositionalArg: true,
	})
}

func TestUpdateCmd_Init(t *testing.T) {
	assert.NotNil(t, updateParser, "updateParser should be initialized")
	assert.NotNil(t, updateCmd, "updateCmd should be initialized")
	assert.False(t, updateCmd.DisableFlagParsing, "DisableFlagParsing should be false")

	stackFlag := updateCmd.Flags().Lookup("stack")
	assert.NotNil(t, stackFlag, "stack flag should be registered")

	identityFlag := updateCmd.Flags().Lookup("identity")
	assert.NotNil(t, identityFlag, "identity flag should be registered")

	targetFlag := updateCmd.Flags().Lookup("target")
	assert.NotNil(t, targetFlag, "target flag should be registered")
}

// update dispatches through the same executeCreateOrUpdate function as
// create; its behavior is covered by backend_create_test.go.
