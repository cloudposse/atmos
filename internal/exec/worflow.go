package exec

import (
	"errors"
	"fmt"
	c "github.com/cloudposse/atmos/pkg/config"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"path"
)

// ExecuteWorkflow executes a workflow
func ExecuteWorkflow(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return errors.New("invalid arguments. The command requires one argument `workflow name` and one flag `file name`")
	}

	err := c.InitConfig()
	if err != nil {
		return err
	}

	flags := cmd.Flags()

	workflowFile, err := flags.GetString("file")
	if err != nil {
		return err
	}

	workflow := args[0]

	var workflowPath string
	if u.IsPathAbsolute(workflowFile) {
		workflowPath = workflowFile
	} else {
		workflowPath = path.Join(c.Config.Workflows.BasePath, workflowFile)
	}

	if !u.FileExists(workflowPath) {
		return errors.New(fmt.Sprintf("File '%s' does not exist", workflowPath))
	}

	color.Cyan("Executing the workflow '%s' from the file '%s'\n", workflow, workflowPath)

	fmt.Println()
	return nil
}
