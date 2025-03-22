package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProcessArgsAndFlags(t *testing.T) {
	inputArgsAndFlags := []string{
		"--deploy-run-init=true",
		"--init-pass-vars=true",
		"--logs-level",
		"Debug",
	}

	info, err := processArgsAndFlags(
		"terraform",
		inputArgsAndFlags,
	)
	assert.NoError(t, err)
	assert.Equal(t, info.DeployRunInit, "true")
	assert.Equal(t, info.InitPassVars, "true")
	assert.Equal(t, info.LogsLevel, "Debug")
}
