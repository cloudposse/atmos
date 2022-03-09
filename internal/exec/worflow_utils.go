package exec

import (
	"errors"
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
			if err := execCommand(args[0], args[1:], ".", []string{}); err != nil {
				return err
			}
		} else if commandType == "atmos" {

		} else {
			return errors.New(fmt.Sprintf("invalid workflow step type '%s'", commandType))
		}

		fmt.Println()
	}

	return nil
}
