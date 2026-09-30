package workflow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/diagnostics"
)

func TestWorkflowSelectorFlags(t *testing.T) {
	tags := workflowCmd.Flags().Lookup("tags")
	require.NotNil(t, tags)
	assert.Equal(t, "stringSlice", tags.Value.Type())

	labels := workflowCmd.Flags().Lookup("labels")
	require.NotNil(t, labels)
	assert.Equal(t, "stringSlice", labels.Value.Type())
}

// TestWorkflowLabelsFlag_BoundToEnvVar is a regression test: bindFlagToViper only
// binds environment variables listed by a flag's GetEnvVars(), so --labels must be
// registered with flags.WithEnvVars("labels", "ATMOS_WORKFLOW_LABELS") for
// ATMOS_WORKFLOW_LABELS to configure this filter at all.
func TestWorkflowLabelsFlag_BoundToEnvVar(t *testing.T) {
	labelsFlag := workflowParser.Registry().Get("labels")
	require.NotNil(t, labelsFlag)
	assert.Contains(t, labelsFlag.GetEnvVars(), "ATMOS_WORKFLOW_LABELS")
}

func TestWorkflowLabelsForwarded(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "environment", want: "team = platform,owner=platform engineering"},
		{name: "explicit flags override environment", args: []string{"--labels=team=cli", "--labels=owner=cli engineering"}, want: "team=cli,owner=cli engineering"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			viper.Reset()
			t.Cleanup(viper.Reset)
			root := t.TempDir()
			eventsPath := filepath.Join(root, "events.jsonl")
			require.NoError(t, os.MkdirAll(filepath.Join(root, "workflows"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(root, "atmos.yaml"), []byte(`base_path: "."
workflows:
  base_path: workflows
diagnostics:
  enabled: true
  file: '`+filepath.ToSlash(eventsPath)+`'
`), 0o644))
			require.NoError(t, os.WriteFile(filepath.Join(root, "workflows", "test.yaml"), []byte(`workflows:
  test:
    steps:
      - name: plan
        type: atmos
        command: terraform plan
`), 0o644))
			t.Setenv("ATMOS_CLI_CONFIG_PATH", root)
			t.Setenv("ATMOS_BASE_PATH", root)
			t.Setenv("ATMOS_WORKFLOW_LABELS", "team = platform,owner=platform engineering")
			cmd := &cobra.Command{Use: "workflow", RunE: workflowCmd.RunE}
			workflowParser.RegisterFlags(cmd)
			cmd.Flags().String("base-path", "", "")
			cmd.Flags().StringSlice("config", nil, "")
			cmd.Flags().StringSlice("config-path", nil, "")
			cmd.Flags().StringSlice("profile", nil, "")
			require.NoError(t, cmd.ParseFlags(append([]string{"--file=test", "--dry-run"}, tc.args...)))
			require.NoError(t, cmd.RunE(cmd, []string{"test"}))
			content, err := os.ReadFile(eventsPath)
			require.NoError(t, err)
			var starts []diagnostics.Event
			for _, line := range strings.Split(strings.TrimSpace(string(content)), "\n") {
				var event diagnostics.Event
				require.NoError(t, json.Unmarshal([]byte(line), &event))
				if event.Type == "process.start" && event.Command == "atmos" {
					starts = append(starts, event)
				}
			}
			require.Len(t, starts, 1)
			assert.Contains(t, starts[0].Args, "--labels="+tc.want)
		})
	}
}
