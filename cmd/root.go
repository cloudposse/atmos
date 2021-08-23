package cmd

import (
	c "atmos/internal/config"
	"github.com/spf13/cobra"
	"log"
)

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:   "atmos",
	Short: "Universal Tool for DevOps and Cloud Automation",
	Long:  `'atmos'' is universal tool for DevOps and cloud automation used for provisioning, managing and orchestrating workflows across various toolchains`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the RootCmd.
func Execute() error {
	return RootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)
}

func initConfig() {
	err := c.InitConfig()
	if err != nil {
		log.Fatalln(err)
	}
}

// https://blog.knoldus.com/create-kubectl-like-cli-with-go-and-cobra/
// https://pkg.go.dev/github.com/c-bata/go-prompt
// https://pkg.go.dev/github.com/spf13/cobra
// https://scene-si.org/2017/04/20/managing-configuration-with-viper/
