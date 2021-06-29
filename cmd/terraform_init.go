package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"log"
)

// terraformInitCmd represents the card command
var terraformInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Run terraform init",
	Long:  ``,

	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("'terraform init' called")
	},
}

func init() {
	terraformInitCmd.PersistentFlags().StringP("stack", "s", "", "")
	terraformInitCmd.PersistentFlags().StringP("command", "c", "", "")

	err := terraformInitCmd.MarkPersistentFlagRequired("stack")
	if err != nil {
		log.Fatalln(err)
		return
	}

	terraformCmd.AddCommand(terraformInitCmd)
}
