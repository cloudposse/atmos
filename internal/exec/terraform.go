package exec

import (
	"fmt"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

// ExecuteTerraform executes terraform commands
func ExecuteTerraform(cmd *cobra.Command, args []string) error {
	if len(args) < 3 {
		return errors.New("invalid number of arguments and flags")
	}

	cmd.DisableFlagParsing = false

	err := cmd.ParseFlags(args)
	if err != nil {
		return err
	}
	flags := cmd.Flags()

	stack, err := flags.GetString("stack")
	if err != nil {
		return err
	}
	fmt.Println("Stack: " + stack)

	additionalArgsAndFlags := removeCommonArgsAndFlags(args)
	fmt.Print("Args2: ")
	fmt.Println(additionalArgsAndFlags)

	err = execCommand("terraform", additionalArgsAndFlags)
	if err != nil {
		return err
	}

	return nil
}
