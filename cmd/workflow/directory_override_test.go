package workflow

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestWorkflowDirectoryOverride(t *testing.T) {
	for _, discover := range []bool{false, true} {
		t.Run(map[bool]string{false: "explicit file", true: "discovery"}[discover], func(t *testing.T) {
			viper.Reset()
			t.Cleanup(viper.Reset)
			root := t.TempDir()
			dir := filepath.Join(root, "external workflows")
			require.NoError(t, os.MkdirAll(dir, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(root, "atmos.yaml"), []byte("base_path: .\nworkflows:\n  base_path: missing-config-directory\n"), 0o644))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "test.yaml"), []byte(`workflows:
  test:
    steps:
      - type: shell
        command: echo selected > "$ATMOS_WORKFLOW_TEST_OUTPUT"
`), 0o644))
			output := filepath.Join(root, "output.txt")
			t.Setenv("ATMOS_CLI_CONFIG_PATH", root)
			t.Setenv("ATMOS_BASE_PATH", root)
			t.Setenv("ATMOS_WORKFLOWS_BASE_PATH", "missing-env-directory")
			t.Setenv("ATMOS_WORKFLOW_TEST_OUTPUT", output)
			cmd := &cobra.Command{Use: "workflow", RunE: workflowCmd.RunE}
			workflowParser.RegisterFlags(cmd)
			cmd.Flags().String("base-path", "", "")
			cmd.Flags().StringSlice("config", nil, "")
			cmd.Flags().StringSlice("config-path", nil, "")
			cmd.Flags().StringSlice("profile", nil, "")
			args := []string{"--workflows-dir", dir}
			if !discover {
				args = append(args, "--file", "test")
			}
			require.NoError(t, cmd.ParseFlags(args))
			require.NoError(t, cmd.RunE(cmd, []string{"test"}))
			content, err := os.ReadFile(output)
			require.NoError(t, err)
			require.Equal(t, "selected\n", string(content))
		})
	}
}
