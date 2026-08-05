package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestRunCompletionShells(t *testing.T) {
	for _, shellName := range []string{"bash", "zsh", "fish", "powershell", "unknown"} {
		t.Run(shellName, func(t *testing.T) {
			root := &cobra.Command{Use: "atmos"}
			cmd := &cobra.Command{Use: shellName, RunE: runCompletion}
			root.AddCommand(cmd)

			require.NoError(t, runCompletion(cmd, nil))
		})
	}
}
