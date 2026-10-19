// Package scaffoldhooks runs a scaffold template's declarative hooks: block
// around atmos scaffold generate / atmos init. It reuses the exact hook
// schema and when: vocabulary that stack-level lifecycle hooks use
// (pkg/hooks.Hook), driving step-backed hooks (kind: step, kind: steps)
// through the same shared step-execution engine (pkg/runner/step) that
// workflows and custom commands use -- not a bespoke execution engine.
package scaffoldhooks

import (
	"context"
	"fmt"
	"sort"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/hooks"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	runnerstep "github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Kind discriminators supported by scaffold hooks. Only the step-backed
// kinds are supported today: command/store/git kinds assume stack/component
// ExecContext data that scaffold generation doesn't have.
const (
	stepKind  = "step"
	stepsKind = "steps"

	// Format string wrapping an error from a specific named hook.
	hookErrFormat = "scaffold hook %q: %w"
)

// RunInput bundles scaffoldhooks.Run's inputs into a single argument, mirroring how
// pkg/hooks.ExecContext bundles a stack-level lifecycle hook run's inputs.
type RunInput struct {
	// HooksMap is the scaffold's decoded spec.hooks block.
	HooksMap map[string]hooks.Hook
	// Event is the scaffold lifecycle event being fired (before/after.scaffold.generate).
	Event hooks.HookEvent
	// Answers is the collected prompt answers, exposed to hooks as {{ .Answers.<field> }} and as
	// the `answers` when: CEL variable.
	Answers map[string]any
	// Status is the run outcome ("success"/"failure"), exposed as the `status` when: CEL variable
	// and to step-backed hooks as ATMOS_HOOK_STATUS (with ATMOS_HOOK_EXIT_CODE).
	Status string
	// Err is the generation error behind a "failure" Status, exposed to step-backed hooks as
	// ATMOS_HOOK_ERROR. Nil on success.
	Err error
	// SkipHooks implements --skip-hooks; nil means no hook is skipped.
	SkipHooks func(string) bool
	// TargetPath is the scaffold's target/output directory. An unset or bare-relative
	// working_directory on a step-backed hook defaults to it (see
	// hooks.ApplyDefaultWorkingDirectory), and it is exposed to hook templates as
	// {{ .TargetPath }}.
	TargetPath string
}

// Run evaluates and executes every hook in in.HooksMap that matches in.Event, in a stable
// (name-sorted) order, skipping any hook that in.SkipHooks (implementing --skip-hooks) reports
// true for before event/when: evaluation, mirroring pkg/hooks' own --skip-hooks semantics.
func Run(in RunInput) error {
	defer perf.Track(nil, "scaffoldhooks.Run")()

	names := make([]string, 0, len(in.HooksMap))
	for name := range in.HooksMap {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		hook := in.HooksMap[name]

		// Event match first, then --skip-hooks, matching the check order
		// pkg/hooks.Hooks.runHookIfMatch uses -- a hook that wouldn't have
		// run for this event shouldn't log a misleading "skipped by
		// --skip-hooks" line.
		if !hook.MatchesEvent(in.Event) {
			continue
		}

		if in.SkipHooks != nil && in.SkipHooks(name) {
			log.Info("Skipping scaffold hook (--skip-hooks)", "hook", name, "kind", hook.Kind)
			continue
		}

		runs, err := hook.RunsWhenE(schema.ConditionContext{
			Answers: in.Answers,
			Event:   string(in.Event),
			Status:  in.Status,
			Hook:    name,
		})
		if err != nil {
			return fmt.Errorf(hookErrFormat, name, err)
		}
		if !runs {
			continue
		}

		if err := runHook(name, &hook, &in); err != nil {
			return err
		}
	}
	return nil
}

// runHook dispatches a single hook to its kind's step conversion and runs
// the resulting step(s) through the shared step executor.
func runHook(name string, hook *hooks.Hook, in *RunInput) error {
	switch hook.Kind {
	case stepKind:
		return runStep(name, hook, in)
	case stepsKind:
		return runSteps(name, hook, in)
	default:
		return errUtils.Build(errUtils.ErrScaffoldHookKindUnsupported).
			WithExplanationf("Scaffold hook %q uses kind %q, which is not yet supported", name, hook.Kind).
			WithHint("Use `kind: step` or `kind: steps` for scaffold hooks").
			WithContext("hook", name).
			WithContext("kind", hook.Kind).
			WithExitCode(2).
			Err()
	}
}

// runStep builds and executes the single step a kind: step hook decodes to, anchoring its
// working_directory at targetPath before running it through the shared step executor.
func runStep(name string, hook *hooks.Hook, in *RunInput) error {
	// A step that sets no name of its own is labelled with the hook's name (hook:<name>), not its
	// step type, so run output identifies which hook ran.
	ws, err := hooks.StepFromHookNamed(name, hook)
	if err != nil {
		return fmt.Errorf(hookErrFormat, name, err)
	}
	hooks.ApplyDefaultWorkingDirectory(ws, in.TargetPath)

	executor := runnerstep.NewStepExecutorWithVars(stepVariables(in))
	if _, err := executor.Execute(context.Background(), ws); err != nil {
		return fmt.Errorf(hookErrFormat, name, err)
	}
	return nil
}

// runSteps builds and executes, in order, the step list a kind: steps hook decodes to, anchoring
// each step's working_directory at targetPath before running it through the shared step executor.
func runSteps(name string, hook *hooks.Hook, in *RunInput) error {
	steps, err := hooks.StepsFromHook(hook)
	if err != nil {
		return fmt.Errorf(hookErrFormat, name, err)
	}

	executor := runnerstep.NewStepExecutorWithVars(stepVariables(in))
	for i := range steps {
		if steps[i].Name == "" {
			steps[i].Name = fmt.Sprintf("hook:%s:%d", name, i+1)
		}
		hooks.ApplyDefaultWorkingDirectory(&steps[i], in.TargetPath)
		if _, err := executor.Execute(context.Background(), &steps[i]); err != nil {
			return fmt.Errorf(hookErrFormat, name, err)
		}
	}
	return nil
}

// stepVariables builds the step Variables for a scaffold hook run: OS
// environment (via NewVariables' default) plus the collected prompt answers
// exposed as {{ .Answers.<field> }} in step bodies and as the `answers` when:
// CEL variable (via Hook.RunsWhenE, not this function). The targetPath
// parameter is likewise exposed as {{ .TargetPath }}, and anchors any other
// relative step field (e.g. a file/workdir step's source/destination/path)
// that resolves through pkg/runner/step's componentWorkingDir mechanism. The run outcome is
// exported as ATMOS_HOOK_STATUS, ATMOS_HOOK_EXIT_CODE, and ATMOS_HOOK_ERROR through the same
// builder (hooks.OutcomeEnv) that stack lifecycle hooks use.
func stepVariables(in *RunInput) *runnerstep.Variables {
	vars := runnerstep.NewVariables()
	vars.SetTemplateData(map[string]any{"Answers": in.Answers, "TargetPath": in.TargetPath})
	vars.SetComponentWorkingDirectory(in.TargetPath)
	for k, v := range hooks.OutcomeEnv(outcomeFor(in)) {
		vars.SetEnv(k, v)
	}
	return vars
}

// outcomeFor converts a RunInput's status and error into the hooks.Outcome the shared env builder
// consumes. A failure outcome without a recorded exit code reports 1.
func outcomeFor(in *RunInput) hooks.Outcome {
	status := hooks.RunStatus(in.Status)
	outcome := hooks.Outcome{Status: status, Err: in.Err}
	if status == hooks.RunFailure {
		outcome.ExitCode = 1
	}
	return outcome
}
