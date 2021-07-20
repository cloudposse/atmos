package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"log"
)

// terraformCmd represents the base command for all terraform sub-commands
var terraformCmd = &cobra.Command{
	Use:   "terraform",
	Short: "Terraform commands",
	Long:  `This command runs terraform sub-commands`,
	// FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
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

		fmt.Println(args)
		fmt.Println("Stack: " + stack)
	},
}

func init() {
	// https://github.com/spf13/cobra/issues/739
	terraformCmd.DisableFlagParsing = false
	terraformCmd.PersistentFlags().StringP("stack", "s", "", "")
	terraformCmd.PersistentFlags().StringP("command", "c", "", "")

	err := terraformCmd.MarkPersistentFlagRequired("stack")
	if err != nil {
		log.Fatalln(err)
		return
	}

	RootCmd.AddCommand(terraformCmd)
}
