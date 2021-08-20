package exec

import (
	"fmt"
	"github.com/spf13/cobra"
	"log"
)

// ExecuteTerraform executes terraform commands
func ExecuteTerraform(cmd *cobra.Command, args []string) {
	fmt.Print("Args: ")
	fmt.Println(args)

	cmd.DisableFlagParsing = false
	err := cmd.ParseFlags(args)
	if err != nil {
		return
	}
	flags := cmd.Flags()

	stack, err := flags.GetString("stack")
	if err != nil {
		log.Fatalln(err)
		return
	}
	fmt.Println("Stack: " + stack)

	args2 := RemoveCommonFlags(args)
	fmt.Print("Args2: ")
	fmt.Println(args2)
}
