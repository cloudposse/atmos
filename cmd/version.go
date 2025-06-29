package cmd

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/internal/exec"
)

var (
	checkFlag     bool
	versionFormat string
)

var versionCmd = &cobra.Command{
	Use:     "version",
	Short:   "Display the version of Atmos you are running and check for updates",
	Long:    `This command shows the version of the Atmos CLI you are currently running and checks if a newer version is available. Use this command to verify your installation and ensure you are up to date.`,
	Example: "atmos version",
	Args:    cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		checkErrorAndExit(exec.NewVersionExec(&atmosConfig).Execute(checkFlag, versionFormat))
	},
}

func init() {
	versionCmd.Flags().BoolVarP(&checkFlag, "check", "c", false, "Run additional checks after displaying version info")
	versionCmd.Flags().StringVar(&versionFormat, "format", "", "Specify the output format")
	RootCmd.AddCommand(versionCmd)
}
