package exec

import (
	"context"
	"strings"

	"github.com/cloudposse/atmos/pkg/auth"
	stepPkg "github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/scheduler"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/workflow"
)

type workflowControlContext struct {
	atmosConfig         schema.AtmosConfiguration
	workflowDefinition  *schema.WorkflowDefinition
	dryRun              bool
	commandLineStack    string
	commandLineTags     []string
	commandLineLabels   string
	commandLineIdentity string
	baseEnv             []string
	persistentEnv       map[string]string
	authManager         auth.AuthManager
}

func executeWorkflowControlStep(ctx context.Context, control *workflowControlContext, parent *schema.WorkflowStep) error {
	vars := workflowControlVariables(control)
	childExecutor := &workflow.ControlCommandExecutor{
		InstallTools:        stepPkg.ScriptToolInstaller(&control.atmosConfig),
		DryRun:              control.dryRun,
		ScriptComponent:     stepPkg.ScriptComponentRef(vars),
		ResolveComponent:    stepPkg.ScriptComponentResolver(vars),
		WorkflowDefinition:  control.workflowDefinition,
		BasePath:            control.atmosConfig.BasePath,
		ProjectRoot:         stepPkg.ScriptProjectRoot(control.atmosConfig.BasePathAbsolute, control.atmosConfig.BasePath),
		BaseEnv:             control.baseEnv,
		CommandLineStack:    control.commandLineStack,
		CommandLineTags:     control.commandLineTags,
		CommandLineLabels:   control.commandLineLabels,
		CommandLineIdentity: control.commandLineIdentity,
		PrepareEnv: func(baseEnv []string, identity string, stepName string, workflowEnv map[string]string, stepEnv map[string]string) ([]string, error) {
			return prepareStepEnvironment(baseEnv, identity, stepName, control.authManager, workflowEnv, control.persistentEnv, stepEnv)
		},
		RunCommand: func(request *workflow.ControlCommandRequest) error {
			return ExecuteShellCommand(
				control.atmosConfig,
				request.Program,
				request.Args,
				request.Dir,
				nil,
				control.dryRun || request.DryRun,
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
		TemplateData: workflowControlTemplateData,
		StoreResult:  storeWorkflowControlResult,
	})
}

// workflowControlVariables returns the workflow's shared step variables with component
// resolution installed, so embedded script children can use `components.get` even when no
// earlier extended step configured the executor.
func workflowControlVariables(control *workflowControlContext) *stepPkg.Variables {
	if stepExecutorState == nil {
		stepExecutorState = stepPkg.NewStepExecutor()
	}
	vars := stepExecutorState.Variables()
	if vars.AtmosConfig == nil {
		vars.SetAtmosConfig(&control.atmosConfig)
	}
	vars.SetScriptComponentInfoResolver(ScriptComponentInfoResolver(vars.AtmosConfig, control.authManager))
	return vars
}

func workflowControlTemplateData(stepName string, matrix map[string]string) map[string]any {
	if stepExecutorState == nil {
		stepExecutorState = stepPkg.NewStepExecutor()
	}
	return stepExecutorState.Variables().TemplateData()
}

func storeWorkflowControlResult(result *scheduler.Result) {
	if stepExecutorState == nil {
		stepExecutorState = stepPkg.NewStepExecutor()
	}
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
	stepExecutorState.Variables().Set(result.NodeID, stepResult)
}
