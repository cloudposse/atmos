package workflow

import (
	"context"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
	stepPkg "github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/scheduler"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// init registers the control-step bridge into the step registry so parallel and
// matrix steps run through the registry (from custom commands and lifecycle
// hooks), not only through the legacy `atmos workflow` executor. The bridge lives
// here — not in pkg/runner/step — because the step package cannot import
// pkg/workflow without an import cycle; the dependency direction is reversed, the
// same seam the emulator step uses (see pkg/runner/step/emulator.go).
func init() {
	stepPkg.RegisterControlRunner(controlBridge{})
}

// controlBridge implements stepPkg.ControlRunner by driving ExecuteControlStep
// with a plain (no-auth) command executor. Child steps are limited to the types
// ControlCommandExecutor supports (shell/atmos/sleep); the `atmos workflow`
// executor supplies its own auth- and output-aware executor for full-fidelity
// runs, but this plain executor is enough to make parallel/matrix usable outside
// a workflow (the credentials, stack, and identity a workflow injects are simply
// absent).
type controlBridge struct{}

// RunControl fans a parallel/matrix step out to its children.
func (controlBridge) RunControl(ctx context.Context, step *schema.WorkflowStep, vars *stepPkg.Variables) (*stepPkg.StepResult, error) {
	defer perf.Track(nil, "workflow.controlBridge.RunControl")()

	childExecutor := &ControlCommandExecutor{
		BaseEnv:     vars.EnvSlice(),
		RunCommand:  plainControlRunCommand,
		ShellRunner: interpreterShellRunner,
	}
	err := ExecuteControlStep(ctx, step, childExecutor.Execute, ControlExecutionOptions{
		TemplateData: func(stepName string, matrix map[string]string) map[string]any {
			return vars.TemplateData()
		},
		StoreResult: func(result *scheduler.Result) {
			storeControlBridgeResult(vars, result)
		},
	})
	if err != nil {
		return nil, err
	}
	return stepPkg.NewStepResult(""), nil
}

// storeControlBridgeResult writes a child's result back into the step variables
// so downstream steps can reference it, mirroring the legacy adapter's
// storeWorkflowControlResult but targeting the caller-supplied Variables.
func storeControlBridgeResult(vars *stepPkg.Variables, result *scheduler.Result) {
	stepResult := stepPkg.NewStepResult("")
	if controlResult, ok := result.Value.(*ControlResult); ok && controlResult != nil {
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
	vars.Set(result.NodeID, stepResult)
}

// plainControlRunCommand executes a control child command without auth/identity
// wiring. It streams to the control engine's output writers and also captures to
// the request buffers so the aggregator can render summaries. An `atmos` child
// resolves to the running binary so it works regardless of PATH.
func plainControlRunCommand(request *ControlCommandRequest) error {
	program := request.Program
	if program == schema.TaskTypeAtmos {
		if exe, err := os.Executable(); err == nil {
			program = exe
		}
	}

	cmd := exec.CommandContext(request.Context, program, request.Args...)
	cmd.Dir = request.Dir
	if len(request.Env) > 0 {
		cmd.Env = request.Env
	} else {
		cmd.Env = os.Environ()
	}
	cmd.Stdin = request.Streams.Stdin
	cmd.Stdout = controlWriter(request.Streams.Stdout, request.Stdout)
	cmd.Stderr = controlWriter(request.Streams.Stderr, request.Stderr)
	return cmd.Run()
}

// interpreterShellRunner runs a control shell child through the in-process
// mvdan/sh interpreter, so parallel/matrix shell children dispatched via the
// registry get the same cross-platform, cancellable semantics as every other
// registry shell step. The writers already tee display + capture; display masks
// at the data layer, so no extra MaskWriter is applied here.
func interpreterShellRunner(ctx context.Context, req *ControlShellRequest) error {
	return u.ShellRunnerWithWriters(&u.ShellRunnerSpec{
		Context: ctx,
		Command: req.Command,
		Dir:     req.Dir,
		Env:     req.Env,
		Stdout:  req.Stdout,
		Stderr:  req.Stderr,
	})
}

// controlWriter tees the live stream and the capture buffer when both are set.
func controlWriter(stream io.Writer, capture io.Writer) io.Writer {
	switch {
	case stream != nil && capture != nil:
		return io.MultiWriter(stream, capture)
	case capture != nil:
		return capture
	default:
		return stream
	}
}
