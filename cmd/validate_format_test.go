package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestValidationFormatPrecedence(t *testing.T) {
	original := atmosConfig
	t.Cleanup(func() { atmosConfig = original })
	atmosConfig = schema.AtmosConfiguration{Validate: schema.Validate{Format: validateFormatRich}}
	cmd := &cobra.Command{}
	addValidationFormatFlag(cmd)

	format, err := validationFormat(cmd)
	require.NoError(t, err)
	assert.Equal(t, validateFormatRich, format)

	t.Setenv("ATMOS_VALIDATE_FORMAT", validateFormatText)
	format, err = validationFormat(cmd)
	require.NoError(t, err)
	assert.Equal(t, validateFormatText, format)

	require.NoError(t, cmd.Flags().Set("format", validateFormatRich))
	format, err = validationFormat(cmd)
	require.NoError(t, err)
	assert.Equal(t, validateFormatRich, format)
}

func TestValidationFormatRejectsUnsupportedValue(t *testing.T) {
	cmd := &cobra.Command{}
	addValidationFormatFlag(cmd)
	require.NoError(t, cmd.Flags().Set("format", "sarif"))
	_, err := validationFormat(cmd)
	require.Error(t, err)
}
