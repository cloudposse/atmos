package flags

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/telemetry"
)

func TestPromptSelectionBindsToViper(t *testing.T) {
	originalInteractive := viper.GetBool("interactive")
	t.Cleanup(func() { viper.Set("interactive", originalInteractive) })
	preserved := telemetry.PreserveCIEnvVars()
	t.Cleanup(func() { telemetry.RestoreCIEnvVars(preserved) })
	t.Setenv("ATMOS_FORCE_TTY", "true")
	viper.Set("interactive", true)
	require.True(t, isInteractive())
	t.Cleanup(SetFormRunnerForTest(func(f *huh.Form) error {
		return f.WithAccessible(true).WithInput(strings.NewReader("1\n")).WithOutput(io.Discard).Run()
	}))

	for _, key := range []string{"stack", "deploy.stack"} {
		t.Run(key, func(t *testing.T) {
			parser := NewStandardFlagParser(
				WithStringFlag("stack", "s", "", "Stack name"),
				WithViperKey("stack", key),
				WithCompletionPrompt("stack", "Choose stack", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
					return []string{"production"}, cobra.ShellCompDirectiveNoFileComp
				}),
			)
			cmd := &cobra.Command{Use: "test"}
			parser.RegisterFlags(cmd)
			v := viper.New()
			require.NoError(t, parser.BindToViper(v))
			result, err := parser.Parse(context.Background(), nil)
			require.NoError(t, err)
			assert.Equal(t, "production", result.Flags["stack"])
			assert.Equal(t, "production", cmd.Flag("stack").Value.String())
			assert.True(t, cmd.Flag("stack").Changed)
			assert.Equal(t, "production", v.GetString(key))
		})
	}
}

func TestPromptInvalidFlagValueDoesNotBindToViper(t *testing.T) {
	originalInteractive := viper.GetBool("interactive")
	t.Cleanup(func() { viper.Set("interactive", originalInteractive) })
	preserved := telemetry.PreserveCIEnvVars()
	t.Cleanup(func() { telemetry.RestoreCIEnvVars(preserved) })
	t.Setenv("ATMOS_FORCE_TTY", "true")
	viper.Set("interactive", true)
	require.True(t, isInteractive())
	t.Cleanup(SetFormRunnerForTest(func(f *huh.Form) error {
		return f.WithAccessible(true).WithInput(strings.NewReader("1\n")).WithOutput(io.Discard).Run()
	}))
	parser := NewStandardFlagParser(
		WithIntFlag("count", "", 7, "Count"),
		WithViperKey("count", "deploy.count"),
		WithCompletionPrompt("count", "Choose count", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
			return []string{"invalid"}, cobra.ShellCompDirectiveNoFileComp
		}),
	)
	cmd := &cobra.Command{Use: "test"}
	parser.RegisterFlags(cmd)
	v := viper.New()
	require.NoError(t, parser.BindToViper(v))
	result := &ParsedConfig{Flags: map[string]any{}}
	require.NoError(t, parser.promptForSingleMissingFlag("count", result, cmd.Flags()))
	assert.False(t, cmd.Flag("count").Changed)
	// A later flag change must not leak through a binding from the failed write-back.
	require.NoError(t, cmd.Flags().Set("count", "9"))
	assert.Equal(t, 7, v.GetInt("deploy.count"))
}
