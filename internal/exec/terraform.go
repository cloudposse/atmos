package exec

import (
	"fmt"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

// ExecuteTerraform executes terraform commands
func ExecuteTerraform(cmd *cobra.Command, args []string) error {
	if len(args) < 3 {
		return errors.New("invalid number of arguments")
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
	terraformSubCommand := args[0]
	allArgsAndFlags := append([]string{terraformSubCommand}, additionalArgsAndFlags...)

	err = execCommand("terraform", allArgsAndFlags)
	if err != nil {
		return err
	}

	return nil
}
