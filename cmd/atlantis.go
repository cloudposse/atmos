package cmd

import (
	"github.com/cloudposse/atmos/internal/exec"
	"github.com/spf13/cobra"
)

// atlantisCmd executes Atlantis commands
var atlantisCmd = &cobra.Command{
	Use:                "atlantis",
	Short:              "Execute 'atlantis' commands",
	Long:               `This command executes Atlantis integration commands`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run: func(cmd *cobra.Command, args []string) {
		exec.ExecuteAtlantis(cmd, args)
	},
}

func init() {
	RootCmd.AddCommand(atlantisCmd)
}
