package devcontainer

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestExecCobraPreservesCommandFlagBoundary(t *testing.T) {
	// Exercise Cobra before the parser: direct parser calls never reproduced the
	// lost separator because Cobra normally removes it before invoking RunE.
	originalRun := execCmd.RunE
	originalPre := execCmd.PreRunE
	instance := execCmd.Flags().Lookup("instance")
	originalInstance, originalChanged := instance.Value.String(), instance.Changed
	t.Cleanup(func() { require.NoError(t, instance.Value.Set(originalInstance)); instance.Changed = originalChanged })
	execCmd.PreRunE = nil
	t.Cleanup(func() { execCmd.RunE = originalRun; execCmd.PreRunE = originalPre })
	execCmd.RunE = func(cmd *cobra.Command, args []string) error {
		parsed, err := execParser.Parse(cmd.Context(), args)
		require.NoError(t, err)
		name, command, err := parseExecInvocation(args, parsed)
		require.NoError(t, err)
		require.Equal(t, "example", name)
		require.Equal(t, []string{"sh", "-c", "printf '%s' --flag"}, command)
		require.Equal(t, "test-instance", parseExecOptions(parsed).Instance)
		return nil
	}
	root := &cobra.Command{Use: "test", SilenceUsage: true, SilenceErrors: true}
	devcontainerCmd.RemoveCommand(execCmd)
	root.AddCommand(execCmd)
	t.Cleanup(func() { root.RemoveCommand(execCmd); devcontainerCmd.AddCommand(execCmd) })
	root.SetArgs([]string{"exec", "example", "--instance", "test-instance", "--", "sh", "-c", "printf '%s' --flag"})
	require.NoError(t, root.Execute())
}

func TestExecHelpBeforeInvocationValidation(t *testing.T) {
	for _, arg := range []string{"--help", "-h"} {
		t.Run(arg, func(t *testing.T) {
			cmd := &cobra.Command{Use: "exec"}
			shown := false
			cmd.SetHelpFunc(func(*cobra.Command, []string) { shown = true })
			require.NoError(t, execCmd.RunE(cmd, []string{arg}))
			require.True(t, shown)
		})
	}
}
