package helmfile

import (
	"os"

	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// handleHelpRequest checks if the user requested help and displays it.
// This uses cobra's built-in help mechanism rather than calling os.Exit directly.
func handleHelpRequest(cmd *cobra.Command, args []string) {
	defer perf.Track(nil, "helmfile.handleHelpRequest")()

	for _, arg := range args {
		if arg == "-h" || arg == "--help" {
			cmd.SetArgs([]string{"--help"})
			_ = cmd.Execute()
			return
		}
	}
}

// enableHeatmapIfRequested enables heatmap tracking if the --heatmap flag is present.
func enableHeatmapIfRequested() {
	defer perf.Track(nil, "helmfile.enableHeatmapIfRequested")()

	for _, arg := range os.Args {
		if arg == "--heatmap" {
			perf.EnableTracking(true)
			return
		}
	}
}

// getConfigAndStacksInfo processes command line arguments and returns configuration info.
func getConfigAndStacksInfo(commandName string, cmd *cobra.Command, args []string) (schema.ConfigAndStacksInfo, error) {
	defer perf.Track(nil, "helmfile.getConfigAndStacksInfo")()

	return e.ProcessCommandLineArgs(commandName, cmd, args, nil)
}

// addStackCompletion adds stack completion to a command.
func addStackCompletion(cmd *cobra.Command) {
	defer perf.Track(nil, "helmfile.addStackCompletion")()

	cmd.PersistentFlags().StringP("stack", "s", "", "Specify the stack name")
	_ = cmd.RegisterFlagCompletionFunc("stack", stackFlagCompletion)
}

// stackFlagCompletion provides shell completion for the --stack flag.
func stackFlagCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	defer perf.Track(nil, "helmfile.stackFlagCompletion")()

	// Return empty completion - the actual completion logic would need to be implemented.
	return nil, cobra.ShellCompDirectiveNoFileComp
}
