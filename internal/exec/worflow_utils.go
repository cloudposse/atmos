package exec

import (
	"errors"
	"fmt"
	c "github.com/cloudposse/atmos/pkg/config"
	"github.com/fatih/color"
	"strings"
)

func executeWorkflowSteps(workflowDefinition c.WorkflowDefinition, dryRun bool) error {
	var steps = workflowDefinition.Steps

	for _, step := range steps {
		var command = strings.TrimSpace(step.Command)
		var commandType = strings.TrimSpace(step.Type)

		color.HiCyan(fmt.Sprintf("Executing workflow step: %s", command))

		if commandType == "" {
			commandType = "atmos"
		}

		if commandType == "shell" {
			args := strings.Fields(command)
			if err := execCommand(args[0], args[1:], ".", []string{}, dryRun); err != nil {
				return err
			}
		} else if commandType == "atmos" {
			args := strings.Fields(command)

			var workflowStack = strings.TrimSpace(workflowDefinition.Stack)
			var stepStack = strings.TrimSpace(step.Stack)
			var finalStack = ""

			// The workflow `stack` attribute overrides the stack in the `command` (if specified)
			// The step `stack` attribute overrides the stack in the `command` and the workflow `stack` attribute
			if workflowStack != "" {
				finalStack = workflowStack
			}
			if stepStack != "" {
				finalStack = stepStack
			}

			if finalStack != "" {
				args = append(args, []string{"-s", finalStack}...)
				color.HiCyan(fmt.Sprintf("Stack: %s", finalStack))
			}

			if err := execCommand("atmos", args, ".", []string{}, dryRun); err != nil {
				return err
			}
		} else {
			return errors.New(fmt.Sprintf("invalid workflow step type '%s'. Supported types are 'atmos' and 'shell'", commandType))
		}

		fmt.Println()
	}

	return nil
}
