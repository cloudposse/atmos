package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDocsGenerateCmd_Error(t *testing.T) {
	err := docsGenerateCmd.RunE(docsGenerateCmd, []string{})
	assert.Error(t, err, "docs generate command should return an error when called with no parameters")
}
