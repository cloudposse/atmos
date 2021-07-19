package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
)

var Version = "0.0.1"
var BuildTime = ""

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "version command",
	Long:  `This command shows the CLI version and the build time`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Version:\t", Version)
		fmt.Println("Build time:\t", BuildTime)
	},
}

func init() {
	RootCmd.AddCommand(versionCmd)
}

// https://www.digitalocean.com/community/tutorials/using-ldflags-to-set-version-information-for-go-applications
// https://blog.kowalczyk.info/article/vEja/embedding-build-number-in-go-executable.html
// https://polyverse.com/blog/how-to-embed-versioning-information-in-go-applications-f76e2579b572/
// https://medium.com/geekculture/golang-app-build-version-in-containers-3d4833a55094
