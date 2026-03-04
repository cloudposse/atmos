package list

import (
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/tests"
)

// TestExecuteListMetadataCmd_WithStackPattern tests that --stack is forwarded correctly.
func TestExecuteListMetadataCmd_WithStackPattern(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	fixtureRelPath := "../../tests/fixtures/scenarios/complete"
	fixturePath, err := filepath.Abs(fixtureRelPath)
	require.NoError(t, err)
	tests.RequireFilePath(t, fixturePath, "test fixture directory")

	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	ui.InitFormatter(ioCtx)
	data.InitWriter(ioCtx)

	cmd := &cobra.Command{}
	metadataParser.RegisterFlags(cmd)
	cmd.Flags().String("base-path", "", "Base path")
	require.NoError(t, cmd.Flags().Set("base-path", fixturePath))
	cmd.Flags().StringSlice("config", []string{}, "Config files")
	cmd.Flags().StringSlice("config-path", []string{}, "Config paths")
	cmd.Flags().StringSlice("profile", []string{}, "Profiles")

	opts := &MetadataOptions{
		Format: "table",
		Stack:  "tenant1-ue2-dev",
	}

	err = executeListMetadataCmd(cmd, []string{}, opts)
	assert.NoError(t, err)
}

// TestExecuteListMetadataCmd_InvalidBasePath tests error handling for invalid base path.
func TestExecuteListMetadataCmd_InvalidBasePath(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	cmd := &cobra.Command{}
	metadataParser.RegisterFlags(cmd)
	cmd.Flags().String("base-path", "", "Base path")
	require.NoError(t, cmd.Flags().Set("base-path", "/nonexistent/path"))
	cmd.Flags().StringSlice("config", []string{}, "Config files")
	cmd.Flags().StringSlice("config-path", []string{}, "Config paths")
	cmd.Flags().StringSlice("profile", []string{}, "Profiles")

	opts := &MetadataOptions{
		Format: "table",
	}

	err := executeListMetadataCmd(cmd, []string{}, opts)
	assert.Error(t, err)
}

// TestColumnsCompletionForMetadata tests that columnsCompletionForMetadata doesn't panic without valid config.
func TestColumnsCompletionForMetadata(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}

	completions, directive := columnsCompletionForMetadata(cmd, []string{}, "")
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	_ = completions
}

// TestColumnsCompletionForMetadata_WithFixture tests completion with a valid fixture config.
func TestColumnsCompletionForMetadata_WithFixture(t *testing.T) {
	fixtureRelPath := "../../tests/fixtures/scenarios/complete"
	fixturePath, err := filepath.Abs(fixtureRelPath)
	require.NoError(t, err)
	tests.RequireFilePath(t, fixturePath, "test fixture directory")

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("base-path", "", "Base path")
	require.NoError(t, cmd.Flags().Set("base-path", fixturePath))
	cmd.Flags().StringSlice("config", []string{}, "Config files")
	cmd.Flags().StringSlice("config-path", []string{}, "Config paths")
	cmd.Flags().StringSlice("profile", []string{}, "Profiles")

	completions, directive := columnsCompletionForMetadata(cmd, []string{}, "")
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	_ = completions
}
