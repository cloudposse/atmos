package workflow

import (
	"fmt"
	"os"

	"github.com/fatih/color"

	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func ExecuteWorkflowUI(cliConfig schema.CliConfiguration) (string, string, string, error) {
	_, _, allWorkflows, err := exec.ExecuteDescribeWorkflows(cliConfig)
	if err != nil {
		return "", "", "", err
	}

	// Start the UI
	app, err := Execute(allWorkflows)
	fmt.Println()
	if err != nil {
		return "", "", "", err
	}

	selectedWorkflowFile := app.GetSelectedWorkflowFile()
	selectedWorkflow := app.GetSelectedWorkflow()
	selectedWorkflowStep := app.GetSelectedWorkflowStep()

	// If the user quit the UI, exit
	if app.ExitStatusQuit() || selectedWorkflowFile == "" || selectedWorkflow == "" {
		return "", "", "", nil
	}

	fmt.Println()
	u.PrintMessageInColor(fmt.Sprintf(
		"Executing command:\n"+os.Args[0]+" workflow %s --file %s --from-step \"%s\"\n", selectedWorkflow, selectedWorkflowFile, selectedWorkflowStep),
		color.New(color.FgCyan),
	)
	fmt.Println()

	return selectedWorkflowFile, selectedWorkflow, selectedWorkflowStep, nil
}
