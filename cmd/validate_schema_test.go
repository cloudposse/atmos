package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestValidateSchemaCmd_FlagParsing(t *testing.T) {
	cmd := ValidateSchemaCmd

	// Test that the schemas-atmos-manifest flag exists and has the correct properties
	flag := cmd.PersistentFlags().Lookup("schemas-atmos-manifest")
	assert.NotNil(t, flag)
	assert.Equal(t, "string", flag.Value.Type())
	assert.Equal(t, "", flag.DefValue)
	assert.Equal(t, "Specifies the path to a JSON schema file used to validate the structure and content of the Atmos manifest file", flag.Usage)
}

func TestValidateSchemaCmd_UnknownFlags(t *testing.T) {
	cmd := ValidateSchemaCmd

	// Verify that unknown flags are not allowed
	assert.False(t, cmd.FParseErrWhitelist.UnknownFlags)
}

func TestIsBuiltinConfigSchemaValidation(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "config", args: []string{"config"}, want: true},
		{name: "nil", args: nil, want: false},
		{name: "custom", args: []string{"custom"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isBuiltinConfigSchemaValidation(tt.args))
		})
	}
}

func TestRunValidateSchemaForFiles_ConfigSkipsConfigCheck(t *testing.T) {
	original := atmosConfig
	t.Cleanup(func() { atmosConfig = original })
	atmosConfig = schema.AtmosConfiguration{}

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "atmos.yaml"), []byte("logs:\n  level: Info\n"), 0o600))
	t.Chdir(dir)

	cmd := &cobra.Command{}
	cmd.Flags().String("schemas-atmos-manifest", "", "")
	addValidationFormatFlag(cmd)
	require.NoError(t, runValidateSchemaForFiles(cmd, []string{"config"}, nil, false, nil))
}
