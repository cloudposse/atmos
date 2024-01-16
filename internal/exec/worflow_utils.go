package exec

import (
	"fmt"
	"strings"

	"github.com/samber/lo"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ExecuteWorkflow executes an Atmos workflow
func ExecuteWorkflow(
	cliConfig schema.CliConfiguration,
	workflow string,
	workflowPath string,
	workflowDefinition *schema.WorkflowDefinition,
	dryRun bool,
	commandLineStack string,
	fromStep string,
) error {
	var steps = workflowDefinition.Steps

	if len(steps) == 0 {
		return fmt.Errorf("workflow '%s' does not have any steps defined", workflow)
	}

	logFunc := u.LogDebug
	if dryRun {
		logFunc = u.LogInfo
	}

	// Check if the steps have the `name` attribute.
	// If not, generate a friendly name consisting of a prefix of `step` and followed by the index of the
	// step (the index starts with 1, so the first generated step name would be `step1`)
	for index, step := range steps {
		if step.Name == "" {
			// When iterating through a slice with a range loop, if elements need to be changed,
			// changing the returned value from the range is not changing the original slice element.
			// That return value is a copy of the element.
			// So doing changes to it will not affect the original elements.
			// We need to access the element with the index returned from the range iterator and change it there.
			// https://medium.com/@nsspathirana/common-mistakes-with-go-slices-95f2e9b362a9
			steps[index].Name = fmt.Sprintf("step%d", index+1)
		}
	}

	logFunc(cliConfig, fmt.Sprintf("\nExecuting the workflow '%s' from '%s'\n", workflow, workflowPath))

	if cliConfig.Logs.Level == u.LogLevelTrace || cliConfig.Logs.Level == u.LogLevelDebug {
		err := u.PrintAsYAML(workflowDefinition)
		if err != nil {
			return err
		}
	}

	// If `--from-step` is specified, skip all the previous steps
	if fromStep != "" {
		steps = lo.DropWhile[schema.WorkflowStep](steps, func(step schema.WorkflowStep) bool {
			return step.Name != fromStep
		})

		if len(steps) == 0 {
			return fmt.Errorf("invalid '--from-step' flag. Workflow '%s' does not have a step with the name '%s'", workflow, fromStep)
		}
	}

	for stepIdx, step := range steps {
		var command = strings.TrimSpace(step.Command)
		var commandType = strings.TrimSpace(step.Type)

		logFunc(cliConfig, fmt.Sprintf("Executing workflow step: %s", command))

		if commandType == "" {
			commandType = "atmos"
		}

		if commandType == "shell" {
			commandName := fmt.Sprintf("%s-step-%d", workflow, stepIdx)
			if err := ExecuteShell(cliConfig, command, commandName, ".", []string{}, dryRun); err != nil {
				return err
			}
		} else if commandType == "atmos" {
			args := strings.Fields(command)

			var workflowStack = strings.TrimSpace(workflowDefinition.Stack)
			var stepStack = strings.TrimSpace(step.Stack)
			var finalStack = ""

			// The workflow `stack` attribute overrides the stack in the `command` (if specified)
			// The step `stack` attribute overrides the stack in the `command` and the workflow `stack` attribute
			// The stack defined on the command line (`atmos workflow <name> -f <file> -s <stack>`) has the highest priority,
			// it overrides all other stacks attributes
			if workflowStack != "" {
				finalStack = workflowStack
			}
			if stepStack != "" {
				finalStack = stepStack
			}
			if commandLineStack != "" {
				finalStack = commandLineStack
			}

			if finalStack != "" {
				args = append(args, []string{"-s", finalStack}...)
				logFunc(cliConfig, fmt.Sprintf("Stack: %s", finalStack))
			}

			if err := ExecuteShellCommand(cliConfig, "atmos", args, ".", []string{}, dryRun, ""); err != nil {
				return err
			}
		} else {
			return fmt.Errorf("invalid workflow step type '%s'. Supported types are 'atmos' and 'shell'", commandType)
		}
	}

	return nil
}

// ExecuteDescribeWorkflows executes `atmos describe workflows` command
func ExecuteDescribeWorkflows(
	cliConfig schema.CliConfiguration,
) (schema.DescribeWorkflowsInfo, error) {
	return nil, nil
}
