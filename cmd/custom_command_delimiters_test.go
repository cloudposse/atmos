package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time guards for the schema fields this test sets.
var (
	_ = schema.AtmosConfiguration{Edition: ""}
	_ = schema.Command{Name: "", Steps: nil}
)

// TestCustomCommandIntegration_StepTemplatesHonorConfiguredDelimiters runs a custom command whose
// matrix child renders .matrix.* values, with custom delimiters configured. An unpinned project
// renders [[ ]] and leaves {{ }} alone; a project pinned before the transition keeps rendering {{ }}.
func TestCustomCommandIntegration_StepTemplatesHonorConfiguredDelimiters(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping integration test in short mode")
	}

	testDir := "../tests/fixtures/scenarios/atmos-auth-mock"
	t.Setenv("ATMOS_CLI_CONFIG_PATH", testDir)
	t.Setenv("ATMOS_BASE_PATH", testDir)

	tests := []struct {
		name        string
		pin         string
		squareForms bool
	}{
		{name: "unpinned renders the configured delimiters", pin: "", squareForms: true},
		{name: "pinned on the transition date renders the configured delimiters", pin: "2026-10-19", squareForms: true},
		{name: "pinned before the transition keeps rendering the default delimiters", pin: "2026-10-18"},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = NewTestKit(t)

			atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
			require.NoError(t, err)
			atmosConfig.Edition = tt.pin
			atmosConfig.Templates.Settings.Delimiters = []string{"[[", "]]"}

			rowsFile := filepath.Join(t.TempDir(), "rows.txt")
			command := customCommandMatrixWriteHelperCommand(t, rowsFile)
			if tt.squareForms {
				command = strings.NewReplacer("{{", "[[", "}}", "]]").Replace(command)
			}
			showSummary := false
			name := "test-delimiters-" + string(rune('a'+i))
			atmosConfig.Commands = []schema.Command{{
				Name: name,
				Steps: schema.Tasks{{
					Name:   "plans",
					Type:   schema.TaskTypeMatrix,
					Matrix: map[string][]string{"component": {"vpc", "eks"}, "stack": {"dev"}},
					ParallelOutput: &schema.ParallelOutputConfig{
						Mode:        "none",
						ShowSummary: &showSummary,
					},
					Steps: []schema.WorkflowStep{{Name: "plan", Type: "shell", Command: command}},
				}},
			}}

			require.NoError(t, processCustomCommands(atmosConfig, atmosConfig.Commands, RootCmd))
			var customCmd *cobra.Command
			for _, c := range RootCmd.Commands() {
				if c.Name() == name {
					customCmd = c
					break
				}
			}
			require.NotNil(t, customCmd, "Custom command should be registered")

			customCmd.Run(customCmd, []string{})

			content, err := os.ReadFile(rowsFile)
			require.NoError(t, err, "matrix rows must execute")
			assert.ElementsMatch(t, []string{"component=vpc,stack=dev", "component=eks,stack=dev"}, splitNonEmptyLines(string(content)))
		})
	}
}
