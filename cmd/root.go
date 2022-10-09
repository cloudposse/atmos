package cmd

import (
	c "github.com/cloudposse/atmos/pkg/config"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
)

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:   "atmos",
	Short: "Universal Tool for DevOps and Cloud Automation",
	Long:  `'atmos'' is a universal tool for DevOps and cloud automation used for provisioning, managing and orchestrating workflows across various toolchains`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the RootCmd.
func Execute() error {
	return RootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)

	// InitConfig finds and merges CLI configurations in the following order:
	// system dir, home dir, current dir, ENV vars, command-line arguments
	// Here we need the custom commands from the config
	Config, err := c.InitConfig(c.ConfigAndStacksInfo{})
	if err != nil {
		u.PrintErrorToStdErrorAndExit(err)
	}

	// Add custom commands from the CLI config
	err = processCustomCommands(Config.Commands, RootCmd, true)
	if err != nil {
		u.PrintErrorToStdErrorAndExit(err)
	}
}

func initConfig() {
}

// https://www.sobyte.net/post/2021-12/create-cli-app-with-cobra/
// https://github.com/spf13/cobra/blob/master/user_guide.md
// https://blog.knoldus.com/create-kubectl-like-cli-with-go-and-cobra/
// https://pkg.go.dev/github.com/c-bata/go-prompt
// https://pkg.go.dev/github.com/spf13/cobra
// https://scene-si.org/2017/04/20/managing-configuration-with-viper/
