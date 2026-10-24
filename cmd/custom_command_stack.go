package cmd

import (
	"strings"

	stepPkg "github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
	workflowPkg "github.com/cloudposse/atmos/pkg/workflow"
)

// customCommandAtmosArgs adds the step's stack unless command already selects one.
func customCommandAtmosArgs(command string, step *schema.Task, vars *stepPkg.Variables) ([]string, error) {
	args := strings.Fields(command)
	for _, arg := range args {
		if arg == "--" {
			break
		}
		if arg == "-s" || arg == "--stack" || strings.HasPrefix(arg, "-s=") || strings.HasPrefix(arg, "--stack=") {
			return args, nil
		}
	}
	workflowStep := step.ToWorkflowStep()
	stack, err := vars.ResolveStepField(&workflowStep, "stack", step.Stack)
	if err != nil {
		return nil, vars.TemplateFieldError(&workflowStep, "stack", err)
	}
	return workflowPkg.AppendAtmosStepFlags(args, workflowPkg.AtmosStepFlags{Stack: strings.TrimSpace(stack)}), nil
}
