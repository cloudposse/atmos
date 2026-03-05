package list

import (
	"bytes"
	goio "io"
	"os"
	"path/filepath"
	"strings"
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

// TestExecuteListMetadataCmd_WithStackPattern tests that --stack filters output to the target stack.
func TestExecuteListMetadataCmd_WithStackPattern(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	fixturePath, err := filepath.Abs(filepath.Join("..", "..", "tests", "fixtures", "scenarios", "complete"))
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

	// Capture stdout to assert filtering behavior.
	oldStdout := os.Stdout
	r, w, pipeErr := os.Pipe()
	require.NoError(t, pipeErr)
	os.Stdout = w
	defer func() { os.Stdout = oldStdout }()

	err = executeListMetadataCmd(cmd, []string{}, opts)

	require.NoError(t, w.Close())
	var buf bytes.Buffer
	_, copyErr := goio.Copy(&buf, r)
	require.NoError(t, copyErr)
	os.Stdout = oldStdout

	require.NoError(t, err)
	output := buf.String()

	// Every data row must belong to the requested stack.
	assert.NotEmpty(t, output, "expected non-empty output for tenant1-ue2-dev")
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		assert.Contains(t, line, "tenant1-ue2-dev", "unexpected stack in output line: %q", line)
	}
}

// TestExecuteListMetadataCmd_InvalidBasePath tests error handling for invalid base path.
func TestExecuteListMetadataCmd_InvalidBasePath(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	cmd := &cobra.Command{}
	metadataParser.RegisterFlags(cmd)
	cmd.Flags().String("base-path", "", "Base path")
	require.NoError(t, cmd.Flags().Set("base-path", filepath.Join(t.TempDir(), "nonexistent", "path")))
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
	fixturePath, err := filepath.Abs(filepath.Join("..", "..", "tests", "fixtures", "scenarios", "complete"))
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
