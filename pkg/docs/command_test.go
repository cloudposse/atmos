package docs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// newGenerateCmd mimics the persistent root flags ExecuteGenerate reads.
func newGenerateCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("base-path", "", "")
	cmd.Flags().StringSlice("config", nil, "")
	cmd.Flags().StringSlice("config-path", nil, "")
	cmd.Flags().StringSlice("profile", nil, "")
	return cmd
}

func TestExecuteGenerate(t *testing.T) {
	dir := t.TempDir()
	atmosYAML := "base_path: .\ndocs:\n  generate:\n    readme:\n      base-dir: .\n      output: OUT.md\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "atmos.yaml"), []byte(atmosYAML), 0o600))
	t.Setenv("ATMOS_CLI_CONFIG_PATH", dir)
	t.Setenv("ATMOS_BASE_PATH", dir)
	t.Chdir(dir)

	t.Run("missing doc type", func(t *testing.T) {
		require.ErrorIs(t, ExecuteGenerate(newGenerateCmd(), nil), errUtils.ErrMissingDocType)
	})
	t.Run("unknown doc type lists configured types", func(t *testing.T) {
		err := ExecuteGenerate(newGenerateCmd(), []string{"nope"})
		require.ErrorIs(t, err, errUtils.ErrNoDocsGenerateEntry)
		assert.Contains(t, strings.Join(errUtils.AllHints(err), "\n"), "readme")
	})
	t.Run("generates configured document", func(t *testing.T) {
		require.NoError(t, ExecuteGenerate(newGenerateCmd(), []string{"readme"}))
		assert.FileExists(t, filepath.Join(dir, "OUT.md"))
	})
	t.Run("missing flag definitions", func(t *testing.T) {
		require.Error(t, ExecuteGenerate(&cobra.Command{}, []string{"readme"}))
	})
}

func TestConfiguredDocTypesHint(t *testing.T) {
	assert.Contains(t, configuredDocTypesHint(map[string]schema.DocsGenerate{"b": {}, "a": {}}), "a, b")
	assert.Contains(t, configuredDocTypesHint(nil), "No docs.generate")
}
