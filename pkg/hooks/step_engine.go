package hooks

import (
	"context"

	yaml "gopkg.in/yaml.v2"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/retry"
	runnerstep "github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
)

// stepKindName is the discriminator for hooks that delegate to the step
// registry (`kind: step`).
const stepKindName = "step"

// init registers the `step` kind, which bridges component lifecycle hooks to
// the workflow/custom-command step registry (pkg/runner/step). A hook of this
// kind names a step `type:` and carries that step's parameters under `with:`,
// making every registered step type (container, toast, log, http, …) available
// on terraform lifecycle events.
func init() {
	if err := RegisterKind(&Kind{
		Name:      stepKindName,
		Engine:    &stepEngine{},
		OnFailure: OnFailureWarn,
	}); err != nil {
		panic("failed to register built-in step kind: " + err.Error())
	}
}

// stepEngine runs a registered step type as a hook. It builds a
// schema.WorkflowStep from the hook's `type:` and `with:` block, seeds the
// step's variable environment with the standard ATMOS_* variables plus the
// hook's `env:`, then executes the step through the same StepExecutor that
// workflows and custom commands use. Wrapper-level policy (`retry`, on_failure)
// is applied here, mirroring how pkg/runner/runner.go wraps step execution.
type stepEngine struct{}

// Run implements Engine.
func (stepEngine) Run(ctx *ExecContext) (*Output, error) {
	defer perf.Track(nil, "hooks.stepEngine.Run")()

	stepType := ctx.Hook.Type
	if stepType == "" {
		return nil, errUtils.Build(errUtils.ErrInvalidConfig).
			WithExplanation("A hook with kind: step must set a step type").
			WithHint("Add a `type:` naming a registered step type (e.g. type: container)").
			Err()
	}
	if _, ok := runnerstep.Get(stepType); !ok {
		return nil, errUtils.Build(errUtils.ErrUnknownStepType).
			WithExplanationf("Hook step type %q is not a registered step type", stepType).
			WithHint("Run `atmos workflow` step types reference for the available types").
			WithContext("type", stepType).
			Err()
	}

	ws, err := stepFromHook(ctx.Hook)
	if err != nil {
		return nil, err
	}

	vars := runnerstep.NewVariables()
	for k, v := range BuildAtmosEnv(ctx, "", "") {
		vars.SetEnv(k, v)
	}
	// The hook's own env: overrides/augments the ATMOS_* defaults, matching the
	// command kind where Hook.Env is layered on top of the standard variables.
	for k, v := range ctx.Hook.Env {
		vars.SetEnv(k, v)
	}

	executor := runnerstep.NewStepExecutorWithVars(vars)

	var result *runnerstep.StepResult
	run := func() error {
		r, runErr := executor.Execute(context.Background(), ws)
		result = r
		return runErr
	}

	var runErr error
	if ws.Retry != nil {
		runErr = retry.Do(context.Background(), ws.Retry, run)
	} else {
		runErr = run()
	}

	out := stepSummary(stepType, result, runErr)
	if runErr != nil {
		return out, ApplyOnFailure(ctx, runErr)
	}
	return out, nil
}

// verifyStepHookType checks, at preflight, that a step-kind hook names a
// registered step type. It deliberately does not render or validate the
// `with:` block (which may still contain unrendered templates pre-auth); the
// engine validates the resolved step config at run time.
func verifyStepHookType(name, stepType string) error {
	if stepType == "" {
		return errUtils.Build(errUtils.ErrInvalidConfig).
			WithExplanationf("Hook %q (kind step) is missing a step type", name).
			WithHint("Add a `type:` naming a registered step type (e.g. type: container)").
			WithContext("hook", name).
			Err()
	}
	if _, ok := runnerstep.Get(stepType); !ok {
		return errUtils.Build(errUtils.ErrUnknownStepType).
			WithExplanationf("Hook %q references step type %q, which is not registered", name, stepType).
			WithContext("hook", name).
			WithContext("type", stepType).
			Err()
	}
	return nil
}

// stepFromHook builds a WorkflowStep from a step-kind hook. The `with:` block
// (already rendered by resolveHookForExecution) is round-tripped through YAML
// into the WorkflowStep — WorkflowStep is designed to unmarshal from YAML, so
// this reuses its tags and nested-struct decoding without a separate mapping.
func stepFromHook(hook *Hook) (*schema.WorkflowStep, error) {
	ws := &schema.WorkflowStep{}
	if len(hook.With) > 0 {
		data, err := yaml.Marshal(hook.With)
		if err != nil {
			return nil, errUtils.Build(errUtils.ErrInvalidConfig).
				WithCause(err).
				WithExplanation("Failed to encode the step hook `with:` block").
				Err()
		}
		if err := yaml.Unmarshal(data, ws); err != nil {
			return nil, errUtils.Build(errUtils.ErrInvalidConfig).
				WithCause(err).
				WithExplanation("Failed to decode the step hook `with:` block into a step").
				WithContext("type", hook.Type).
				Err()
		}
	}
	// The envelope owns type and retry; they always win over anything in `with:`.
	ws.Type = hook.Type
	ws.Retry = hook.Retry
	if ws.Name == "" {
		ws.Name = "hook:" + hook.Type
	}
	return ws, nil
}

// stepSummary builds a best-effort Output envelope for the step run. The step
// streams its own output to the terminal, so Body is left empty (no
// double-render); this Summary exists for run-page/PR consumers that key off
// status. Returns a non-nil Output even on error so callers can surface it.
func stepSummary(stepType string, result *runnerstep.StepResult, runErr error) *Output {
	summary := &Summary{Kind: stepKindName}
	switch {
	case runErr != nil:
		summary.Status = StatusFailure
		summary.Title = "step " + stepType + " failed"
	case result != nil && result.Skipped:
		summary.Status = StatusSuccess
		summary.Title = "step " + stepType + " skipped"
	default:
		summary.Status = StatusSuccess
		summary.Title = "step " + stepType + " ok"
	}
	log.Debug("Step hook finished", logKeyKind, stepKindName, "type", stepType, "status", summary.Status)
	return &Output{Summary: summary}
}
