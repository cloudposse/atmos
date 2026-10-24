package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	e "github.com/cloudposse/atmos/internal/exec"
	stepPkg "github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestCustomCommandAtmosStepStack(t *testing.T) {
	for _, tc := range []struct {
		name, command, stack string
		want                 []string
	}{
		{"step stack", "describe component noop", "dev", []string{"describe", "component", "noop", "-s", "dev"}},
		{"templated stack", "describe component noop", "{{ .Flags.target }}", []string{"describe", "component", "noop", "-s", "dev"}},
		{"explicit attached short stack", "describe component noop -sprod", "dev", []string{"describe", "component", "noop", "-sprod"}},
		{"explicit short stack", "describe component noop -s prod", "dev", []string{"describe", "component", "noop", "-s", "prod"}},
		{"explicit long stack", "describe component noop --stack prod", "dev", []string{"describe", "component", "noop", "--stack", "prod"}},
		{"explicit equals stack", "describe component noop --stack=prod", "dev", []string{"describe", "component", "noop", "--stack=prod"}},
		{"explicit short equals stack", "describe component noop -s=prod", "dev", []string{"describe", "component", "noop", "-s=prod"}},
		{"attached short passthrough", "terraform test noop -- -sother", "dev", []string{"terraform", "test", "noop", "-s", "dev", "--", "-sother"}},
		{"passthrough", "terraform test noop -- -s other", "dev", []string{"terraform", "test", "noop", "-s", "dev", "--", "-s", "other"}},
		{"no stack", "version", "", []string{"version"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_ = NewTestKit(t)
			dir := t.TempDir()
			output := filepath.Join(dir, "args.json")
			command := schema.Command{
				Name:  "test-step-stack",
				Flags: []schema.CommandFlag{{Name: "target", Default: "dev"}},
				Steps: schema.Tasks{{
					Type: "atmos", Command: tc.command, Stack: tc.stack,
					Env: map[string]string{"_ATMOS_TEST_DUMP_ARGS": output},
				}},
			}
			config := schema.AtmosConfiguration{BasePath: dir, Commands: []schema.Command{command}}
			parent := &cobra.Command{Use: "atmos"}
			require.NoError(t, processCustomCommands(config, config.Commands, parent))
			custom := findSubcommand(parent, command.Name)
			require.NotNil(t, custom)
			custom.Run(custom, nil)
			data, err := os.ReadFile(output)
			require.NoError(t, err)
			var args []string
			require.NoError(t, json.Unmarshal(data, &args))
			assert.Equal(t, tc.want, args)
		})
	}
}

// Workflows retain their existing step-level stack behavior alongside custom commands.
func TestWorkflowAtmosStepStack(t *testing.T) {
	_ = NewTestKit(t)
	dir := t.TempDir()
	executable, err := os.Executable()
	require.NoError(t, err)
	binaryName := "atmos"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	require.NoError(t, os.Link(executable, filepath.Join(dir, binaryName)))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	output := filepath.Join(dir, "args.json")
	config := schema.AtmosConfiguration{BasePath: dir}
	workflow := &schema.WorkflowDefinition{
		Stack: "prod",
		Steps: []schema.WorkflowStep{{
			Name: "describe", Type: "atmos", Command: "describe component noop", Stack: "dev",
			Env: map[string]string{"_ATMOS_TEST_DUMP_ARGS": output},
		}},
	}
	require.NoError(t, e.ExecuteWorkflow(config, "step-stack", "", workflow, false, "", "", ""))
	data, err := os.ReadFile(output)
	require.NoError(t, err)
	var args []string
	require.NoError(t, json.Unmarshal(data, &args))
	assert.Equal(t, []string{"describe", "component", "noop", "-s", "dev"}, args)
}

func TestCustomCommandAtmosArgsInvalidStackTemplate(t *testing.T) {
	_, err := customCommandAtmosArgs("describe component noop", &schema.Task{Name: "describe", Stack: "{{ .Flags.target"}, stepPkg.NewVariables())
	require.Error(t, err)
	assert.ErrorContains(t, err, "stack")
	assert.ErrorContains(t, err, "describe")
}
