package exec

import (
	"context"
	"strings"

	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/perf"
	stepPkg "github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/scheduler"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/workflow"
)

// CustomCommandControlContext carries the context needed to execute a `type: parallel`/
// `type: matrix` step declared inside a custom command's `steps:` list. Custom commands
// have no schema.WorkflowDefinition of their own, so one is synthesized from the fields
// pkg/workflow/control_executor.go actually reads (Env, Stack) rather than threading a
// second, parallel context type through pkg/workflow.
type CustomCommandControlContext struct {
	AtmosConfig      schema.AtmosConfiguration
	CommandName      string
	CommandEnv       map[string]string
	CommandLineStack string
	CommandIdentity  string
	BaseEnv          []string
	AuthManager      auth.AuthManager
	Executor         *stepPkg.StepExecutor
}

// ExecuteCustomCommandControlStep runs a parallel/matrix step declared in a custom
// command's steps: list. It mirrors executeWorkflowControlStep (workflow_control_adapter.go)
// exactly, reusing the same pkg/workflow.ControlCommandExecutor/ExecuteControlStep engine so
// `needs:`/concurrency/fail-mode semantics are identical between workflows and custom commands.
func ExecuteCustomCommandControlStep(ctx context.Context, control *CustomCommandControlContext, parent *schema.WorkflowStep) error {
	defer perf.Track(&control.AtmosConfig, "exec.ExecuteCustomCommandControlStep")()

	workflowDefinition := &schema.WorkflowDefinition{
		Env:   control.CommandEnv,
		Stack: control.CommandLineStack,
	}
	childExecutor := &workflow.ControlCommandExecutor{
		WorkflowDefinition:  workflowDefinition,
		BasePath:            control.AtmosConfig.BasePath,
		BaseEnv:             control.BaseEnv,
		CommandLineStack:    control.CommandLineStack,
		CommandLineIdentity: control.CommandIdentity,
		PrepareEnv: func(baseEnv []string, identity string, stepName string, workflowEnv map[string]string, stepEnv map[string]string) ([]string, error) {
			return prepareStepEnvironment(baseEnv, identity, stepName, control.AuthManager, workflowEnv, stepEnv)
		},
		RunCommand: func(request *workflow.ControlCommandRequest) error {
			return ExecuteShellCommand(
				control.AtmosConfig,
				request.Program,
				request.Args,
				request.Dir,
				nil,
				false,
				"",
				WithProcessContext(request.Context),
				WithEnvironment(request.Env),
				WithProcessStreams(request.Streams),
				WithStdoutCapture(request.Stdout),
				WithStderrCapture(request.Stderr),
			)
		},
	}
	return workflow.ExecuteControlStep(ctx, parent, childExecutor.Execute, workflow.ControlExecutionOptions{
		TemplateData: func(stepName string, matrix map[string]string) map[string]any {
			return control.Executor.Variables().TemplateData()
		},
		StoreResult: func(result *scheduler.Result) {
			storeCustomCommandControlResult(control.Executor, result)
		},
	})
}

// storeCustomCommandControlResult bridges a completed parallel/matrix child's result back
// into the custom command's own step executor, so later sequential steps can reference
// `{{ .steps.<name>.* }}`, exactly as storeWorkflowControlResult does for workflows.
func storeCustomCommandControlResult(executor *stepPkg.StepExecutor, result *scheduler.Result) {
	stepResult := stepPkg.NewStepResult("")
	if controlResult, ok := result.Value.(*workflow.ControlResult); ok && controlResult != nil {
		stepResult = stepPkg.NewStepResult(strings.TrimSpace(controlResult.Stdout)).
			WithMetadata("stdout", controlResult.Stdout).
			WithMetadata("stderr", controlResult.Stderr).
			WithMetadata("status", string(result.Status)).
			WithMetadata("canceled", controlResult.Canceled)
		if controlResult.Err != nil {
			stepResult.WithError(controlResult.Err.Error())
		}
	}
	if result.Status == scheduler.StatusSkipped {
		stepResult.WithSkipped()
	}
	executor.Variables().Set(result.NodeID, stepResult)
}
