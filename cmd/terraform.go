package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
)

// terraformCmd represents the base command for all terraform sub-commands
var terraformCmd = &cobra.Command{
	Use:   "terraform",
	Short: "Terraform commands",
	Long:  `This command runs terraform sub-commands`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("'atmos terraform' called")
	},
}

func init() {
	RootCmd.AddCommand(terraformCmd)
}

// https://blog.knoldus.com/create-kubectl-like-cli-with-go-and-cobra/
// https://pkg.go.dev/github.com/c-bata/go-prompt
// https://pkg.go.dev/github.com/spf13/cobra
// https://scene-si.org/2017/04/20/managing-configuration-with-viper/
