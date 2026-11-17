package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomCommandFlagsReachCommandEnv(t *testing.T) {
	_ = NewTestKit(t)
	exe, err := os.Executable()
	require.NoError(t, err)
	result := filepath.Join(t.TempDir(), "env")
	command := schema.Command{
		Name: "flag-env",
		Flags: []schema.CommandFlag{
			{Name: "output", Type: "string", Default: "default-path"},
			{Name: "enabled", Type: "bool", Default: true},
			{Name: "count", Type: "int", Default: 7},
		},
		Env: []schema.CommandEnv{
			{Key: "CAPTURED_RESULT", Value: "{{ index .Flags \"output\" }}|{{ .Flags.enabled }}|{{ .flags.count }}"},
		},
		Steps: schema.Tasks{{Type: "shell", Command: fmt.Sprintf("%q", exe), Env: map[string]string{"_ATMOS_TEST_DUMP_ENV": result}}},
	}
	config := schema.AtmosConfiguration{BasePath: t.TempDir(), Commands: []schema.Command{command}}
	parent := &cobra.Command{Use: "atmos"}
	require.NoError(t, processCustomCommands(config, config.Commands, parent))
	target := findSubcommand(parent, command.Name)
	require.NotNil(t, target)
	require.NoError(t, target.PersistentFlags().Set("output", "custom path/binary"))
	target.Run(target, nil)
	content, err := os.ReadFile(result)
	require.NoError(t, err)
	assert.Equal(t, "custom path/binary|true|7", extractEnvVar(string(content), "CAPTURED_RESULT"))
}
