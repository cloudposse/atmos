package cmd

import (
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
	//err := c.InitConfig()
	//if err != nil {
	//	u.PrintErrorToStdErrorAndExit(err)
	//}
	//
	//var testCmd = &cobra.Command{
	//	Use:   "terraform",
	//	Short: "Print test",
	//	Long:  `This command prints test`,
	//	Run: func(cmd *cobra.Command, args []string) {
	//		fmt.Println("test-01")
	//	},
	//}
	//
	//var testCmd2 = &cobra.Command{
	//	Use:   "test",
	//	Short: "Print test",
	//	Long:  `This command prints test`,
	//	Run: func(cmd *cobra.Command, args []string) {
	//		fmt.Println("this is terraform test command")
	//	},
	//}
	//
	//testCmd.AddCommand(testCmd2)
	//RootCmd.AddCommand(testCmd)
}

func initConfig() {
}

// https://www.sobyte.net/post/2021-12/create-cli-app-with-cobra/
// https://github.com/spf13/cobra/blob/master/user_guide.md
// https://blog.knoldus.com/create-kubectl-like-cli-with-go-and-cobra/
// https://pkg.go.dev/github.com/c-bata/go-prompt
// https://pkg.go.dev/github.com/spf13/cobra
// https://scene-si.org/2017/04/20/managing-configuration-with-viper/
