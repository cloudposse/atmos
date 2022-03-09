package exec

import (
	"fmt"
	c "github.com/cloudposse/atmos/pkg/config"
	"github.com/fatih/color"
	"strings"
)

func executeWorkflowSteps(workflowDefinition c.WorkflowDefinition) error {
	var steps = workflowDefinition.Steps

	for _, step := range steps {
		var command = step.Command
		var commandType = step.Type

		color.Cyan(fmt.Sprintf("Executing workflow step: %s", command))

		if commandType == "shell" {
			args := strings.Fields(command)
			err := execCommand(args[0], args[1:], ".", []string{})
			if err != nil {
				return err
			}
		}

		fmt.Println()
	}

	return nil
}
