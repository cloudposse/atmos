package exec

import (
	"context"
	"strings"

	"github.com/cloudposse/atmos/pkg/auth"
	envpkg "github.com/cloudposse/atmos/pkg/env"
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
	// WorkingDirectory is the resolved directory of the enclosing control step (the command-level
	// `working_directory`, optionally overridden by the parent step's own). Children resolve relative
	// `working_directory` values against it and inherit it when they set none. Empty falls back to the
	// Atmos base path.
	WorkingDirectory string
}

// ExecuteCustomCommandControlStep runs a parallel/matrix step declared in a custom
// command's steps: list. It mirrors executeWorkflowControlStep (workflow_control_adapter.go)
// exactly, reusing the same pkg/workflow.ControlCommandExecutor/ExecuteControlStep engine so
// `needs:`/concurrency/fail-mode semantics are identical between workflows and custom commands.
func ExecuteCustomCommandControlStep(ctx context.Context, control *CustomCommandControlContext, parent *schema.WorkflowStep) error {
	defer perf.Track(&control.AtmosConfig, "exec.ExecuteCustomCommandControlStep")()

	childExecutor := newCustomCommandControlExecutor(control)
	return workflow.ExecuteControlStep(ctx, parent, childExecutor.Execute, workflow.ControlExecutionOptions{
		TemplateData: func(stepName string, matrix map[string]string) map[string]any {
			return control.Executor.Variables().TemplateData()
		},
		StoreResult: func(result *scheduler.Result) {
			storeCustomCommandControlResult(control.Executor, result)
		},
	})
}

// newCustomCommandControlExecutor builds the child executor for a custom command's parallel/matrix
// step. Relative child `working_directory` values resolve against the command's working directory
// (not the project root), mirroring how sequential custom-command steps resolve theirs; absolute
// values still win.
func newCustomCommandControlExecutor(control *CustomCommandControlContext) *workflow.ControlCommandExecutor {
	workflowDefinition := &schema.WorkflowDefinition{
		Env:   control.CommandEnv,
		Stack: control.CommandLineStack,
	}
	basePath := control.AtmosConfig.BasePath
	if strings.TrimSpace(control.WorkingDirectory) != "" {
		basePath = control.WorkingDirectory
		workflowDefinition.WorkingDirectory = control.WorkingDirectory
	}
	var vars *stepPkg.Variables
	if control.Executor != nil {
		vars = control.Executor.Variables()
	}
	return &workflow.ControlCommandExecutor{
		InstallTools: stepPkg.ScriptToolInstaller(&control.AtmosConfig),
		// Custom commands have no dry-run mode; children always execute.
		DryRun:                 false,
		ScriptComponent:        stepPkg.ScriptComponentRef(vars),
		ScriptFlags:            vars.ScriptFlags(),
		ScriptArguments:        vars.ScriptArguments(),
		ResolveComponent:       stepPkg.ScriptComponentResolver(vars),
		ScriptProcessOverrides: commandEnvOverrides(control),
		WorkflowDefinition:     workflowDefinition,
		BasePath:               basePath,
		ProjectRoot:            stepPkg.ScriptProjectRoot(control.AtmosConfig.BasePathAbsolute, control.AtmosConfig.BasePath),
		BaseEnv:                control.BaseEnv,
		CommandLineStack:       control.CommandLineStack,
		CommandLineIdentity:    control.CommandIdentity,
		PrepareEnv: func(baseEnv []string, identity string, stepName string, workflowEnv map[string]string, stepEnv map[string]string) ([]string, error) {
			// Custom-command parallel/matrix children have no `type: env` persistent-env
			// concept (that's a sequential-workflow-only feature) -- pass nil.
			return prepareStepEnvironment(baseEnv, identity, stepName, control.AuthManager, workflowEnv, nil, stepEnv)
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
}

// commandEnvOverrides names the command-level env keys, with their values from the command's
// process env, so embedded script children keep command env above component env.
func commandEnvOverrides(control *CustomCommandControlContext) map[string]string {
	if len(control.CommandEnv) == 0 {
		return nil
	}
	processEnv := envpkg.SliceToMap(control.BaseEnv)
	overrides := make(map[string]string, len(control.CommandEnv))
	for key := range control.CommandEnv {
		overrides[key] = processEnv[key]
	}
	return overrides
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
		if controlResult.Value != nil {
			stepResult.Value = *controlResult.Value
		}
		if controlResult.Err != nil {
			stepResult.WithError(controlResult.Err.Error())
		}
	}
	if result.Status == scheduler.StatusSkipped {
		stepResult.WithSkipped()
	}
	executor.Variables().Set(result.NodeID, stepResult)
}
