package cmd

import (
	"github.com/spf13/cobra"

	initcmd "github.com/cloudposse/atmos/cmd/init"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/ui/spinner"
)

func preflightProjectInit(root *cobra.Command, args []string) error {
	command, remaining, err := root.Find(args)
	if err == nil && isProjectInit(root, command) && !isHelpRequested(command, args) {
		return initcmd.Preflight(remaining)
	}
	return nil
}

// startInitProgress keeps a single indicator alive from startup through copying.
// Help and non-terminal output stay quiet; init stops it before prompts or results.
func startInitProgress(root *cobra.Command, args []string) func() {
	if !showInitConfigProgress(root, args) || !term.IsTTYSupportForStdout() {
		return func() {}
	}
	progress := spinner.New("Loading configuration")
	progress.Start()
	initcmd.SetStartupProgress(progress)
	return func() {
		progress.Stop()
		initcmd.SetStartupProgress(nil)
	}
}

// showInitConfigProgress limits early progress to the built-in project init command;
// help, completion, nested init commands, and other machine-readable commands stay quiet.
func showInitConfigProgress(root *cobra.Command, args []string) bool {
	command, _, err := root.Find(args)
	return err == nil && isProjectInit(root, command) && !isHelpRequested(command, args)
}

func isProjectInit(root, command *cobra.Command) bool {
	return command != nil && command.Name() == "init" && command.Parent() == root
}
