package cmd

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/spf13/cobra"

	u "github.com/cloudposse/atmos/pkg/utils"
)

const (
	atmosDocsURL = "https://atmos.tools"
)

// docsCmd opens the Atmos docs
var docsCmd = &cobra.Command{
	Use:                "docs",
	Short:              "Open the Atmos docs",
	Long:               `This command opens the Atmos docs`,
	Example:            "atmos docs",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run: func(cmd *cobra.Command, args []string) {
		var err error
		os := runtime.GOOS

		switch os {
		case "linux":
			err = exec.Command("xdg-open", atmosDocsURL).Start()
		case "windows":
			err = exec.Command("rundll32", "url.dll,FileProtocolHandler", atmosDocsURL).Start()
		case "darwin":
			err = exec.Command("open", atmosDocsURL).Start()
		default:
			err = fmt.Errorf("unsupported platform: %s", os)
		}

		if err != nil {
			u.LogErrorAndExit(err)
		}
	},
}

func init() {
	RootCmd.AddCommand(docsCmd)
}
