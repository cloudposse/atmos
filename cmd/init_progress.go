package cmd

import (
	"os"

	"github.com/spf13/cobra"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/spinner"
)

// loadStartupConfig shows progress for project initialization before configuration
// is available. The spinner stops before formatter initialization or error output.
func loadStartupConfig(root *cobra.Command, info *schema.ConfigAndStacksInfo) (schema.AtmosConfiguration, error) {
	if showInitConfigProgress(root, os.Args[1:]) {
		progress := spinner.New("Loading configuration")
		progress.Start()
		defer progress.Stop()
	}
	return cfg.InitCliConfig(*info, false)
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
