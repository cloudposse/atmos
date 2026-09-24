package scaffoldhooks

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/condition"
	"github.com/cloudposse/atmos/pkg/hooks"
	runnerstep "github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
)

// captureHandler records the resolved content of every step it executes, so
// tests can assert both that a step ran and what template data it saw.
type captureHandler struct {
	runnerstep.BaseHandler
	calls *[]string
}

func (h *captureHandler) Validate(*schema.WorkflowStep) error { return nil }

func (h *captureHandler) Execute(_ context.Context, step *schema.WorkflowStep, vars *runnerstep.Variables) (*runnerstep.StepResult, error) {
	resolved, err := vars.Resolve(step.Content)
	if err != nil {
		return nil, err
	}
	*h.calls = append(*h.calls, resolved)
	return runnerstep.NewStepResult(resolved), nil
}

// registerCapture registers a uniquely-named "capture" step type for a
// single test and returns the calls slice it appends to.
func registerCapture(t *testing.T) *[]string {
	t.Helper()
	calls := &[]string{}
	runnerstep.Register(&captureHandler{
		BaseHandler: runnerstep.NewBaseHandler(t.Name(), runnerstep.CategoryOutput, false),
		calls:       calls,
	})
	return calls
}

func TestRun_MatchesEventAndWhen(t *testing.T) {
	calls := registerCapture(t)

	hooksMap := map[string]hooks.Hook{
		"post-only": {
			Kind:   "step",
			Events: []string{"after.scaffold.generate"},
			Type:   t.Name(),
			With:   map[string]any{"content": "post ran"},
		},
		"pre-only": {
			Kind:   "step",
			Events: []string{"before.scaffold.generate"},
			Type:   t.Name(),
			With:   map[string]any{"content": "pre ran"},
		},
	}

	err := Run(RunInput{
		HooksMap:   hooksMap,
		Event:      hooks.AfterScaffoldGenerate,
		Answers:    map[string]any{},
		Status:     "success",
		SkipHooks:  nil,
		TargetPath: "/target",
	})
	require.NoError(t, err)

	assert.Equal(t, []string{"post ran"}, *calls)
}

func TestRun_WhenGatesExecution(t *testing.T) {
	calls := registerCapture(t)

	hooksMap := map[string]hooks.Hook{
		"conditional": {
			Kind: "step",
			Type: t.Name(),
			When: mustCondition(t, "'dev' in answers.environments"),
			With: map[string]any{"content": "ran"},
		},
	}

	// Answer doesn't include "dev": hook must not run.
	err := Run(RunInput{
		HooksMap:   hooksMap,
		Event:      hooks.AfterScaffoldGenerate,
		Answers:    map[string]any{"environments": []string{"staging"}},
		Status:     "success",
		SkipHooks:  nil,
		TargetPath: "/target",
	})
	require.NoError(t, err)
	assert.Empty(t, *calls)

	// Answer includes "dev": hook must run.
	err = Run(RunInput{
		HooksMap:   hooksMap,
		Event:      hooks.AfterScaffoldGenerate,
		Answers:    map[string]any{"environments": []string{"dev"}},
		Status:     "success",
		SkipHooks:  nil,
		TargetPath: "/target",
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"ran"}, *calls)
}

func TestRun_SkipHooksBypassesNamedHook(t *testing.T) {
	calls := registerCapture(t)

	hooksMap := map[string]hooks.Hook{
		"git-add": {
			Kind: "step",
			Type: t.Name(),
			With: map[string]any{"content": "git-add ran"},
		},
		"other": {
			Kind: "step",
			Type: t.Name(),
			With: map[string]any{"content": "other ran"},
		},
	}

	skip := func(name string) bool { return name == "git-add" }
	err := Run(RunInput{
		HooksMap:   hooksMap,
		Event:      hooks.AfterScaffoldGenerate,
		Answers:    map[string]any{},
		Status:     "success",
		SkipHooks:  skip,
		TargetPath: "/target",
	})
	require.NoError(t, err)

	assert.ElementsMatch(t, []string{"other ran"}, *calls)
}

func TestRun_SkipHooksAllBypassesEverything(t *testing.T) {
	calls := registerCapture(t)

	hooksMap := map[string]hooks.Hook{
		"a": {Kind: "step", Type: t.Name(), With: map[string]any{"content": "a"}},
		"b": {Kind: "step", Type: t.Name(), With: map[string]any{"content": "b"}},
	}

	skipAll := func(string) bool { return true }
	err := Run(RunInput{
		HooksMap:   hooksMap,
		Event:      hooks.AfterScaffoldGenerate,
		Answers:    map[string]any{},
		Status:     "success",
		SkipHooks:  skipAll,
		TargetPath: "/target",
	})
	require.NoError(t, err)

	assert.Empty(t, *calls)
}

func TestRun_AnswersReachStepTemplateData(t *testing.T) {
	calls := registerCapture(t)

	hooksMap := map[string]hooks.Hook{
		"templated": {
			Kind: "step",
			Type: t.Name(),
			With: map[string]any{"content": "environments: {{ .Answers.environments }}"},
		},
	}

	err := Run(RunInput{
		HooksMap:   hooksMap,
		Event:      hooks.AfterScaffoldGenerate,
		Answers:    map[string]any{"environments": []string{"dev", "staging"}},
		Status:     "success",
		SkipHooks:  nil,
		TargetPath: "/target",
	})
	require.NoError(t, err)

	require.Len(t, *calls, 1)
	assert.Equal(t, "environments: [dev staging]", (*calls)[0])
}

func TestRun_TargetPathReachesStepTemplateData(t *testing.T) {
	calls := registerCapture(t)

	hooksMap := map[string]hooks.Hook{
		"templated": {
			Kind: "step",
			Type: t.Name(),
			With: map[string]any{"content": "target: {{ .TargetPath }}"},
		},
	}

	err := Run(RunInput{
		HooksMap:   hooksMap,
		Event:      hooks.AfterScaffoldGenerate,
		Answers:    map[string]any{},
		Status:     "success",
		SkipHooks:  nil,
		TargetPath: "/scaffold/out",
	})
	require.NoError(t, err)

	require.Len(t, *calls, 1)
	assert.Equal(t, "target: /scaffold/out", (*calls)[0])
}

func TestRun_StepsKind(t *testing.T) {
	calls := registerCapture(t)

	hooksMap := map[string]hooks.Hook{
		"multi": {
			Kind: "steps",
			With: []any{
				map[string]any{"type": t.Name(), "content": "first"},
				map[string]any{"type": t.Name(), "content": "second"},
			},
		},
	}

	err := Run(RunInput{
		HooksMap:   hooksMap,
		Event:      hooks.AfterScaffoldGenerate,
		Answers:    map[string]any{},
		Status:     "success",
		SkipHooks:  nil,
		TargetPath: "/target",
	})
	require.NoError(t, err)

	assert.Equal(t, []string{"first", "second"}, *calls)
}

func TestRun_DocumentationFixtureUsesEventsConditionsAndOrderedSteps(t *testing.T) {
	calls := registerCapture(t)
	answers := map[string]any{"component_name": "vpc", "enable_monitoring": true}

	hooksMap := map[string]hooks.Hook{
		"prepare": {
			Kind:   "step",
			Events: []string{"before.scaffold.generate"},
			Type:   t.Name(),
			With:   map[string]any{"content": "prepare {{ .Answers.component_name }}"},
		},
		"format": {
			Kind:   "step",
			Events: []string{"after.scaffold.generate"},
			Type:   t.Name(),
			When:   mustCondition(t, "answers.enable_monitoring == true"),
			With:   map[string]any{"content": "format {{ .Answers.component_name }}"},
		},
		"verify": {
			Kind:   "steps",
			Events: []string{"after.scaffold.generate"},
			With: []any{
				map[string]any{"type": t.Name(), "content": "validate {{ .Answers.component_name }}"},
				map[string]any{"type": t.Name(), "content": "finish {{ .Answers.component_name }}"},
			},
		},
	}

	require.NoError(t, Run(RunInput{
		HooksMap:   hooksMap,
		Event:      hooks.BeforeScaffoldGenerate,
		Answers:    answers,
		Status:     "success",
		SkipHooks:  nil,
		TargetPath: "/target",
	}))
	require.NoError(t, Run(RunInput{
		HooksMap:   hooksMap,
		Event:      hooks.AfterScaffoldGenerate,
		Answers:    answers,
		Status:     "success",
		SkipHooks:  nil,
		TargetPath: "/target",
	}))
	assert.Equal(t, []string{"prepare vpc", "format vpc", "validate vpc", "finish vpc"}, *calls)
}

func TestRun_UnsupportedKindReturnsError(t *testing.T) {
	hooksMap := map[string]hooks.Hook{
		"legacy": {Kind: "command", Command: "echo hi"},
	}

	err := Run(RunInput{
		HooksMap:   hooksMap,
		Event:      hooks.AfterScaffoldGenerate,
		Answers:    map[string]any{},
		Status:     "success",
		SkipHooks:  nil,
		TargetPath: "/target",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrScaffoldHookKindUnsupported)
}

// mustCondition parses a bare CEL when: expression for test fixtures.
func mustCondition(t *testing.T, expr string) schema.Condition {
	t.Helper()
	c, err := condition.New("!cel " + expr)
	require.NoError(t, err)
	return c
}
