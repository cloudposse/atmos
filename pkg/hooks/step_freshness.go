package hooks

import (
	"fmt"
	"os"
	"path/filepath"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/runner/freshness"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/telemetry"
)

// hookStepPolicy decides whether an individual hook step runs and records its freshness
// state afterwards. It gives `kind: step` and `kind: steps` hooks the same per-step `when:`
// evaluation, `inputs`/`artifacts`/`preconditions` facts, and persisted checksum state that
// custom commands (cmd/cmd_utils.go) and workflows (internal/exec/workflow_utils.go) apply,
// so a step declared once in the shared step library behaves the same in every caller.
type hookStepPolicy struct {
	checker  *freshness.Checker
	stateDir string
	scope    string
}

// newFreshnessChecker builds the checker hook steps use. Tests swap it for one with an
// uncached globber so artifact removal within a single process is observed.
var newFreshnessChecker = func() *freshness.Checker { return freshness.NewChecker() }

// newHookStepPolicy keys freshness state by component, stack, and hook name so two hooks
// (or the same hook on two components) never share a recorded checksum.
func newHookStepPolicy(ctx *ExecContext) hookStepPolicy {
	basePath := ""
	if ctx.AtmosConfig != nil {
		basePath = ctx.AtmosConfig.BasePath
	}
	component, stack := "", ""
	if ctx.Info != nil {
		component, stack = ctx.Info.ComponentFromArg, ctx.Info.Stack
	}
	return hookStepPolicy{
		checker:  newFreshnessChecker(),
		stateDir: freshness.StateDir(basePath),
		scope:    "hook:" + component + ":" + stack + ":" + ctx.HookName,
	}
}

// hookStepConditionContext exposes the same lifecycle facts to a step's `when:` that the hook
// envelope's own `when:` sees, plus the step name. Only an explicit step `when:` consults the
// operation outcome: the envelope already decided whether the hook runs at all, so an implicit
// condition (the `checksum.changed` that inputs/artifacts imply) must not re-apply the
// envelope's default `when: success` and silently skip steps of a `when: failure` hook.
func hookStepConditionContext(ctx *ExecContext, step *schema.WorkflowStep) schema.ConditionContext {
	status := schema.ConditionPredicateSuccess
	if !step.When.IsZero() && ctx.Outcome.Status != "" {
		status = string(ctx.Outcome.Status)
	}
	cc := schema.ConditionContext{
		CI:     telemetry.IsCI(),
		Status: status,
		Hook:   ctx.HookName,
		Event:  string(ctx.Event),
		Step:   step.Name,
	}
	if ctx.Hook != nil {
		cc.Env = ctx.Hook.Env
	}
	if ctx.Info != nil {
		cc.Stack = ctx.Info.Stack
		cc.Component = ctx.Info.ComponentFromArg
	}
	return cc
}

// hookStepFreshnessDir resolves the directory freshness globs are relative to: the step's
// working directory as already defaulted to the component directory, or the process CWD when
// the step left a dot-prefixed path for exec to resolve.
func hookStepFreshnessDir(step *schema.WorkflowStep) string {
	dir := step.WorkingDirectory
	if dir == "" {
		dir = "."
	}
	if filepath.IsAbs(dir) {
		return dir
	}
	cwd, err := os.Getwd()
	if err != nil {
		return dir
	}
	return filepath.Join(cwd, dir)
}

// hookStepFreshnessName keys persisted state for unnamed steps by position.
func hookStepFreshnessName(step *schema.WorkflowStep, index int) string {
	if step.Name != "" {
		return step.Name
	}
	return fmt.Sprintf("step-%d", index)
}

// declaresFreshness reports whether the step opts into freshness facts at all.
func declaresFreshness(step *schema.WorkflowStep) bool {
	return step.Inputs != nil || step.Artifacts != nil || step.Preconditions != nil
}

// shouldRun evaluates the step's effective `when:` (the declared one, or the implicit
// `checksum.changed` that inputs/artifacts imply) against lifecycle and freshness facts.
func (p hookStepPolicy) shouldRun(ctx *ExecContext, step *schema.WorkflowStep, index int) (bool, error) {
	defer perf.Track(nil, "hooks.hookStepPolicy.shouldRun")()

	// A step with neither a `when:` nor freshness declarations always runs, as it always has.
	if step.When.IsZero() && !declaresFreshness(step) {
		return true, nil
	}
	declared := freshness.StepDeclarations{Inputs: step.Inputs, Artifacts: step.Artifacts, Preconditions: step.Preconditions}
	effective := freshness.EffectiveWhen(step.When, declared)
	cc := hookStepConditionContext(ctx, step)
	if declaresFreshness(step) {
		id := freshness.StepIdentity{BaseDir: hookStepFreshnessDir(step), StateDir: p.stateDir, Scope: p.scope, StepName: hookStepFreshnessName(step, index)}
		facts, err := p.checker.Compute(effective, declared, id)
		if err != nil {
			return false, err
		}
		cc.ChecksumChanged = facts.ChecksumChanged
		cc.TimestampChanged = facts.TimestampChanged
		cc.PreconditionsSuccess = facts.PreconditionsSuccess
		cc.Sources = facts.Sources
		cc.Artifacts = facts.Artifacts
	}
	return effective.EvaluateWithImplicitSuccessE(cc)
}

// recordSuccess persists the sources checksum after a successful step so the next run can
// skip it. Only steps that declared inputs or artifacts have state to record; a recording
// failure is logged rather than failing an otherwise-successful step, as in workflows.
func (p hookStepPolicy) recordSuccess(step *schema.WorkflowStep, index int) {
	if step.Inputs == nil && step.Artifacts == nil {
		return
	}
	if err := p.checker.RecordSuccess(step.Inputs, hookStepFreshnessDir(step), p.stateDir, p.scope, hookStepFreshnessName(step, index)); err != nil {
		log.Debug("Failed to record freshness state for hook step", "hook", p.scope, "step", step.Name, "error", err)
	}
}
