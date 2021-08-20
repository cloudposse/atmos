package exec

import (
	"fmt"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"log"
)

// ExecuteTerraform executes terraform commands
func ExecuteTerraform(cmd *cobra.Command, args []string) {
	fmt.Print("Args: ")
	fmt.Println(args)
	fmt.Println()

	cmd.DisableFlagParsing = false
	err := cmd.ParseFlags(args)
	if err != nil {
		return
	}
	flags := cmd.Flags()

	fmt.Println("Flags: ")
	flags.Visit(func(flag *pflag.Flag) {
		fmt.Println(flag.Name + ": " + flag.Value.String())
	})

	_, args2, err := cmd.Traverse(args)
	if err != nil {
		return
	}

	fmt.Print("Args2: ")
	fmt.Println(args2)
	fmt.Println()

	stack, err := flags.GetString("stack")
	if err != nil {
		log.Fatalln(err)
		return
	}
	fmt.Println("Stack: " + stack)

}
