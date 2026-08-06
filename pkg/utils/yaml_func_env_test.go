package utils

import (
	"testing"

	"github.com/cloudposse/atmos/pkg/function/parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessTagEnvWrapsParserError(t *testing.T) {
	_, err := ProcessTagEnv(`!env "unterminated`, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidAtmosYAMLFunction)

	var parseErr *parser.Error
	require.ErrorAs(t, err, &parseErr)
	assert.Equal(t, "unterminated quoted value", parseErr.Message)
}
