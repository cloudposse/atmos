package exec

import (
	"fmt"
	"github.com/fatih/color"
)

// processHelp processes help commands
func processHelp(componentType string, command string) error {
	if len(command) == 0 {
		fmt.Println(fmt.Sprintf("'atmos' supports all native '%s' commands.", componentType))
		fmt.Println(fmt.Sprintf("In addition, 'component' and 'stack' are required in order to generate variables for the component in the stack."))
		color.Cyan(fmt.Sprintf("atmos %s <command> <component> -s <stack> [options]", componentType))
		color.Cyan(fmt.Sprintf("atmos %s <command> <component> --stack <stack> [options]", componentType))

		err := execCommand(componentType, []string{"--help"}, "", nil)
		if err != nil {
			return err
		}
	} else {
		fmt.Println(fmt.Sprintf("'atmos' supports native '%s %s' command with all the options, arguments and flags.", componentType, command))
		fmt.Println(fmt.Sprintf("In addition, 'component' and 'stack' are required in order to generate variables for the component in the stack."))
		color.Cyan(fmt.Sprintf("atmos %s %s <component> -s <stack> [options]", componentType, command))
		color.Cyan(fmt.Sprintf("atmos %s %s <component> --stack <stack> [options]", componentType, command))

		err := execCommand(componentType, []string{command, "--help"}, "", nil)
		if err != nil {
			return err
		}
	}

	fmt.Println()
	return nil
}
