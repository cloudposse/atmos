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

// Run evaluates and executes every hook in hooksMap that matches event, in a
// stable (name-sorted) order, skipping any hook that the skipHooks predicate
// (implementing --skip-hooks) reports true for before event/when: evaluation,
// mirroring pkg/hooks' own --skip-hooks semantics.
func Run(hooksMap map[string]hooks.Hook, event hooks.HookEvent, answers map[string]any, status string, skipHooks func(string) bool) error {
	defer perf.Track(nil, "scaffoldhooks.Run")()

	names := make([]string, 0, len(hooksMap))
	for name := range hooksMap {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		hook := hooksMap[name]

		// Event match first, then --skip-hooks, matching the check order
		// pkg/hooks.Hooks.runHookIfMatch uses -- a hook that wouldn't have
		// run for this event shouldn't log a misleading "skipped by
		// --skip-hooks" line.
		if !hook.MatchesEvent(event) {
			continue
		}

		if skipHooks != nil && skipHooks(name) {
			log.Info("Skipping scaffold hook (--skip-hooks)", "hook", name, "kind", hook.Kind)
			continue
		}

		runs, err := hook.RunsWhenE(schema.ConditionContext{
			Answers: answers,
			Event:   string(event),
			Status:  status,
			Hook:    name,
		})
		if err != nil {
			return fmt.Errorf(hookErrFormat, name, err)
		}
		if !runs {
			continue
		}

		if err := runHook(name, &hook, answers); err != nil {
			return err
		}
	}
	return nil
}

// runHook dispatches a single hook to its kind's step conversion and runs
// the resulting step(s) through the shared step executor.
func runHook(name string, hook *hooks.Hook, answers map[string]any) error {
	switch hook.Kind {
	case stepKind:
		return runStep(name, hook, answers)
	case stepsKind:
		return runSteps(name, hook, answers)
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

func runStep(name string, hook *hooks.Hook, answers map[string]any) error {
	ws, err := hooks.StepFromHook(hook)
	if err != nil {
		return fmt.Errorf(hookErrFormat, name, err)
	}
	if ws.Name == "" {
		ws.Name = "hook:" + name
	}

	executor := runnerstep.NewStepExecutorWithVars(stepVariables(answers))
	if _, err := executor.Execute(context.Background(), ws); err != nil {
		return fmt.Errorf(hookErrFormat, name, err)
	}
	return nil
}

func runSteps(name string, hook *hooks.Hook, answers map[string]any) error {
	steps, err := hooks.StepsFromHook(hook)
	if err != nil {
		return fmt.Errorf(hookErrFormat, name, err)
	}

	executor := runnerstep.NewStepExecutorWithVars(stepVariables(answers))
	for i := range steps {
		if steps[i].Name == "" {
			steps[i].Name = fmt.Sprintf("hook:%s:%d", name, i+1)
		}
		if _, err := executor.Execute(context.Background(), &steps[i]); err != nil {
			return fmt.Errorf(hookErrFormat, name, err)
		}
	}
	return nil
}

// stepVariables builds the step Variables for a scaffold hook run: OS
// environment (via NewVariables' default) plus the collected prompt answers
// exposed as {{ .Answers.<field> }} in step bodies and as the `answers` when:
// CEL variable (via Hook.RunsWhenE, not this function).
func stepVariables(answers map[string]any) *runnerstep.Variables {
	vars := runnerstep.NewVariables()
	vars.SetTemplateData(map[string]any{"Answers": answers})
	return vars
}
