package stack

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestStackValidateCmd_RegisteredUnderStack(t *testing.T) {
	found := false
	for _, c := range stackCmd.Commands() {
		if c.Name() == "validate" {
			found = true
			break
		}
	}
	assert.True(t, found, "expected \"validate\" to be registered as a subcommand of \"stack\"")
}

func TestStackValidateCmd_HasSchemaOverrideFlag(t *testing.T) {
	// The alias must accept the same schema-override flag as `atmos validate
	// stacks`, which the shared executor reads from the command's flag set.
	flag := stackValidateCmd.PersistentFlags().Lookup("schemas-atmos-manifest")
	require.NotNil(t, flag, "expected the schemas-atmos-manifest flag to be defined")
	assert.NotNil(t, stackValidateCmd.PersistentFlags().Lookup("affected"))
	assert.NotNil(t, stackValidateCmd.PersistentFlags().Lookup("base"))
}

func TestStackValidateCmdRichFormat(t *testing.T) {
	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{StacksBaseAbsolutePath: filepath.Join(t.TempDir(), "missing-stacks")}

	output := &bytes.Buffer{}
	command := &cobra.Command{}
	command.Flags().String("format", "rich", "")
	command.SetOut(output)
	require.NoError(t, stackValidateCmd.RunE(command, nil))
	assert.Contains(t, output.String(), "No stack manifests found")

	invalid := &cobra.Command{}
	invalid.Flags().String("format", "xml", "")
	err := stackValidateCmd.RunE(invalid, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected text or rich")
}

// The end-to-end behavior (delegating to `atmos validate stacks`) requires the
// root command's inherited global flags, so it is exercised through the CLI
// integration tests rather than by invoking the bare subcommand's RunE here.
