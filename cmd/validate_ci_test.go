package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateCIAlias(t *testing.T) {
	command, _, err := validateCmd.Find([]string{"ci"})
	assert.NoError(t, err)
	assert.Same(t, validateCICmd, command)
	assert.NotNil(t, validateCICmd.Flag("format"))
	assert.Equal(t, "ci [workflow-file ...]", validateCICmd.Use)
}
