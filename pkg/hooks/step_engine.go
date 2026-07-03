package hooks

import (
	"context"
	"fmt"

	// Use yaml.v3 (not v2) so that WorkflowStep.UnmarshalYAML fires when decoding the
	// hook `with:` block — that custom unmarshaler owns the polymorphic `output`
	// and the container `action:`/`with:` keys, which yaml.v2 would silently skip.
	yaml "gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/retry"
	runnerstep "github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
)

// stepKindName is the discriminator for hooks that delegate to one step in the
// step registry (`kind: step`).
const stepKindName = "step"

// stepsKindName is the discriminator for hooks that delegate to an ordered list
// of registered steps (`kind: steps`).
const stepsKindName = "steps"

// init registers hook kinds that bridge component lifecycle hooks to the
// workflow/custom-command step registry (pkg/runner/step). `kind: step` runs
// one step; `kind: steps` runs an ordered list from `with:`.
func init() {
	if err := RegisterKind(&Kind{
		Name:      stepKindName,
		Engine:    &stepEngine{},
		OnFailure: OnFailureWarn,
	}); err != nil {
		panic("failed to register built-in step kind: " + err.Error())
	}
	if err := RegisterKind(&Kind{
		Name:      stepsKindName,
		Engine:    &stepsEngine{},
		OnFailure: OnFailureWarn,
	}); err != nil {
		panic("failed to register built-in steps kind: " + err.Error())
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

	executor := runnerstep.NewStepExecutorWithVars(stepVariables(ctx))

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
	if hook.With != nil {
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

// stepsEngine runs an ordered list of registered step types as a single hook.
// The hook envelope owns lifecycle concerns (`events`, `when`, `on_failure`,
// `retry`, `env`); `with:` is the ordered step payload.
type stepsEngine struct{}

// Run implements Engine.
func (stepsEngine) Run(ctx *ExecContext) (*Output, error) {
	defer perf.Track(nil, "hooks.stepsEngine.Run")()

	steps, err := stepsFromHook(ctx.Hook)
	if err != nil {
		return nil, err
	}

	var lastResult *runnerstep.StepResult
	run := func() error {
		lastResult = nil
		executor := runnerstep.NewStepExecutorWithVars(stepVariables(ctx))
		for i := range steps {
			if steps[i].Name == "" {
				steps[i].Name = fmt.Sprintf("hook:steps:%d", i+1)
			}
			result, runErr := executor.Execute(context.Background(), &steps[i])
			lastResult = result
			if runErr != nil {
				return runErr
			}
		}
		return nil
	}

	var runErr error
	if ctx.Hook.Retry != nil {
		runErr = retry.Do(context.Background(), ctx.Hook.Retry, run)
	} else {
		runErr = run()
	}

	out := stepsSummary(lastResult, runErr)
	if runErr != nil {
		return out, ApplyOnFailure(ctx, runErr)
	}
	return out, nil
}

func stepsFromHook(hook *Hook) ([]schema.WorkflowStep, error) {
	if hook.With == nil {
		return nil, errUtils.Build(errUtils.ErrInvalidConfig).
			WithExplanation("A hook with kind: steps must set `with:` to an ordered list of steps").
			WithHint("Add `with:` as a YAML list, e.g. `- type: emulator` followed by `- type: atmos`").
			Err()
	}

	data, err := yaml.Marshal(hook.With)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrInvalidConfig).
			WithCause(err).
			WithExplanation("Failed to encode the steps hook `with:` block").
			Err()
	}

	var steps []schema.WorkflowStep
	if err := yaml.Unmarshal(data, &steps); err != nil {
		return nil, errUtils.Build(errUtils.ErrInvalidConfig).
			WithCause(err).
			WithExplanation("Failed to decode the steps hook `with:` block into an ordered step list").
			Err()
	}
	if len(steps) == 0 {
		return nil, errUtils.Build(errUtils.ErrInvalidConfig).
			WithExplanation("A hook with kind: steps must include at least one step in `with:`").
			Err()
	}
	return steps, nil
}

func verifyStepsHookTypes(name string, hook *Hook) error {
	steps, err := stepsFromHook(hook)
	if err != nil {
		return err
	}
	for i := range steps {
		stepType := steps[i].Type
		if stepType == "" {
			stepType = "shell"
		}
		if _, ok := runnerstep.Get(stepType); !ok {
			return errUtils.Build(errUtils.ErrUnknownStepType).
				WithExplanationf("Hook %q step %d references step type %q, which is not registered", name, i+1, stepType).
				WithContext("hook", name).
				WithContext("type", stepType).
				Err()
		}
	}
	return nil
}

func stepVariables(ctx *ExecContext) *runnerstep.Variables {
	vars := runnerstep.NewVariables()
	for k, v := range BuildAtmosEnv(ctx, "", "") {
		vars.SetEnv(k, v)
	}
	// The hook's own env: overrides/augments the ATMOS_* defaults, matching the
	// command kind where Hook.Env is layered on top of the standard variables.
	for k, v := range ctx.Hook.Env {
		vars.SetEnv(k, v)
	}
	return vars
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

func stepsSummary(result *runnerstep.StepResult, runErr error) *Output {
	summary := &Summary{Kind: stepsKindName}
	switch {
	case runErr != nil:
		summary.Status = StatusFailure
		summary.Title = "steps failed"
	case result != nil && result.Skipped:
		summary.Status = StatusSuccess
		summary.Title = "steps skipped"
	default:
		summary.Status = StatusSuccess
		summary.Title = "steps ok"
	}
	log.Debug("Steps hook finished", logKeyKind, stepsKindName, "status", summary.Status)
	return &Output{Summary: summary}
}
