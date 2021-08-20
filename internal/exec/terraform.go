package exec

import (
	"fmt"
	"github.com/spf13/cobra"
	"log"
)

// ExecuteTerraform executes terraform commands
func ExecuteTerraform(cmd *cobra.Command, args []string) error {
	cmd.DisableFlagParsing = false

	err := cmd.ParseFlags(args)
	if err != nil {
		return err
	}
	flags := cmd.Flags()

	stack, err := flags.GetString("stack")
	if err != nil {
		log.Fatalln(err)
		return err
	}
	fmt.Println("Stack: " + stack)

	commandArgsAndFlags := removeCommonFlags(args)
	fmt.Print("Args2: ")
	fmt.Println(commandArgsAndFlags)

	err = execCommand("terraform", commandArgsAndFlags)
	if err != nil {
		return err
	}

	return nil
}
