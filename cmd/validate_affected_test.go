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

func TestAffectedValidationSelectors(t *testing.T) {
	originalAtmosConfig := atmosConfig
	t.Cleanup(func() { atmosConfig = originalAtmosConfig })

	project := t.TempDir()
	t.Chdir(project)
	require.NoError(t, os.MkdirAll(filepath.Join(project, "stacks"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(project, ".github", "workflows"), 0o755))
	require.NoError(t, os.WriteFile("atmos.yaml", []byte("base_path: .\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join("stacks", "dev.yaml"), []byte("vars: {}\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(".github", "workflows", "test.yaml"), []byte("name: test\n"), 0o600))
	require.NoError(t, os.WriteFile("main.go", []byte("package main\n"), 0o600))

	atmosConfig = schema.AtmosConfiguration{StacksBaseAbsolutePath: filepath.Join(project, "stacks")}

	assert.True(t, affectedStacksApplicable([]string{"stacks/dev.yaml"}))
	assert.False(t, affectedStacksApplicable([]string{"main.go"}))
	assert.True(t, affectedStacksApplicable([]string{"atmos.yaml"}))

	files, validateAll := affectedSchemaFiles([]string{"stacks/dev.yaml"}, "")
	assert.Equal(t, []string{"stacks/dev.yaml"}, files)
	assert.False(t, validateAll)
	files, validateAll = affectedSchemaFiles([]string{"atmos.yaml"}, "")
	assert.Nil(t, files)
	assert.True(t, validateAll)
	files, validateAll = affectedSchemaFiles([]string{"atmos.yaml"}, validateConfigSchemaKey)
	assert.Equal(t, []string{"atmos.yaml"}, files)
	assert.False(t, validateAll)

	assert.Equal(t, []string{".github/workflows/test.yaml"}, affectedWorkflowPaths([]string{".github/workflows/test.yaml", "main.go"}))
	assert.True(t, affectedWorkflowConfigChanged([]string{".github/actionlint.yaml"}))

	files, validateAll = affectedEditorConfigFiles([]string{"main.go"})
	assert.Equal(t, []string{"main.go"}, files)
	assert.False(t, validateAll)
	files, validateAll = affectedEditorConfigFiles([]string{".editorconfig"})
	assert.Nil(t, files)
	assert.True(t, validateAll)
}

func TestValidateCommandsExposeAffectedFlags(t *testing.T) {
	for _, command := range []*cobra.Command{validateCmd, ValidateConfigCmd, ValidateSchemaCmd, ValidateStacksCmd, editorConfigCmd} {
		t.Run(command.Name(), func(t *testing.T) {
			assert.NotNil(t, command.Flags().Lookup("affected"))
			assert.NotNil(t, command.Flags().Lookup("base"))
		})
	}
}
