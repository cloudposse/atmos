package exec

import (
	"errors"
	"fmt"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// ExecuteWorkflow executes a workflow
func ExecuteWorkflow(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return errors.New("invalid arguments. The command requires one argument `workflow name` and one flag `file name`")
	}

	flags := cmd.Flags()

	workflowFile, err := flags.GetString("file")
	if err != nil {
		return err
	}

	workflow := args[0]

	color.Cyan("Executing the workflow '%s' from the file '%s'\n", workflow, workflowFile)

	fmt.Println()
	return nil
}
