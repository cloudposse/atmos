package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"log"
	"strings"
)

// terraformCmd represents the base command for all terraform sub-commands
var terraformCmd = &cobra.Command{
	Use:   "terraform",
	Short: "Terraform commands",
	Long:  `This command runs terraform sub-commands`,
	Run: func(cmd *cobra.Command, args []string) {
		flags := cmd.Flags()

		fmt.Println("Flags: ")

		flags.Visit(func(flag *pflag.Flag) {
			fmt.Println(flag.Name + ": " + flag.Value.String())
		})

		stack, err := flags.GetString("stack")
		if err != nil {
			log.Fatalln(err)
			return
		}

		fmt.Println("Arguments: " + strings.Join(args, ","))
		fmt.Println("Stack: " + stack)
	},
}

func init() {
	terraformCmd.PersistentFlags().StringP("stack", "s", "", "")
	terraformCmd.PersistentFlags().StringP("command", "c", "", "")

	err := terraformCmd.MarkPersistentFlagRequired("stack")
	if err != nil {
		log.Fatalln(err)
		return
	}

	RootCmd.AddCommand(terraformCmd)
}
