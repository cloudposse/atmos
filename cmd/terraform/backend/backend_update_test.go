package backend

import (
	"testing"
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
