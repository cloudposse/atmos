package backend

import (
	"testing"
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
