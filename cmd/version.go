package cmd

import (
	"fmt"

	tuiUtils "github.com/cloudposse/atmos/internal/tui/utils"
	"github.com/spf13/cobra"

	u "github.com/cloudposse/atmos/pkg/utils"
)

var Version = "0.0.1"

var versionCmd = &cobra.Command{
	Use:     "version",
	Short:   "Print the CLI version",
	Long:    `This command prints the CLI version`,
	Example: "atmos version",
	Run: func(cmd *cobra.Command, args []string) {
		// Print a styled Atmos logo to the terminal
		fmt.Println()
		err := tuiUtils.PrintAtmosLogo("ATMOS")
		if err != nil {
			u.LogErrorAndExit(err)
		}
		u.PrintMessage(Version)
		fmt.Println()
	},
}

func init() {
	RootCmd.AddCommand(versionCmd)
}
