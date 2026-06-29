package hooks

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v2"

	errUtils "github.com/cloudposse/atmos/errors"
	runnerstep "github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
)

// errFlaky is the sentinel a flaky test handler returns while it is still
// configured to fail.
var errFlaky = errors.New("flaky step failed")

// flakyHandler fails its first (failUntil-1) executions, then succeeds. It
// records how many times Execute ran so retry behavior can be asserted.
type flakyHandler struct {
	runnerstep.BaseHandler
	attempts  *int
	failUntil int
}

func (h *flakyHandler) Validate(*schema.WorkflowStep) error { return nil }

func (h *flakyHandler) Execute(context.Context, *schema.WorkflowStep, *runnerstep.Variables) (*runnerstep.StepResult, error) {
	*h.attempts++
	if *h.attempts < h.failUntil {
		return nil, errFlaky
	}
	return runnerstep.NewStepResult("ok"), nil
}

// envCaptureHandler records the variable environment it was handed so the
// ATMOS_* seeding can be asserted.
type envCaptureHandler struct {
	runnerstep.BaseHandler
	captured *map[string]string
}

func (h *envCaptureHandler) Validate(*schema.WorkflowStep) error { return nil }

func (h *envCaptureHandler) Execute(_ context.Context, _ *schema.WorkflowStep, vars *runnerstep.Variables) (*runnerstep.StepResult, error) {
	captured := make(map[string]string, len(vars.Env))
	for k, v := range vars.Env {
		captured[k] = v
	}
	*h.captured = captured
	return runnerstep.NewStepResult("ok"), nil
}

func stepExecContext(hook *Hook) *ExecContext {
	kind, _ := GetKind(stepKindName)
	return &ExecContext{
		Hook:  hook,
		Kind:  kind,
		Event: AfterTerraformApply,
		Info:  &schema.ConfigAndStacksInfo{Stack: "test-stack", ComponentFromArg: "test-component"},
	}
}

func TestStepKindRegistered(t *testing.T) {
	kind, ok := GetKind(stepKindName)
	require.True(t, ok, "step kind must be registered")
	assert.Equal(t, stepKindName, kind.Name)
	assert.Equal(t, OnFailureWarn, kind.OnFailure)
	assert.Contains(t, ListKinds(), stepKindName)
}

func TestHookUnmarshalStepFields(t *testing.T) {
	const src = `
kind: step
type: container
events:
  - after-terraform-apply
on_failure: fail
retry:
  max_attempts: 3
with:
  action: build
  image: example:latest
  build:
    context: .
    tags:
      - example:latest
`
	var hook Hook
	require.NoError(t, yaml.Unmarshal([]byte(src), &hook))

	assert.Equal(t, stepKindName, hook.Kind)
	assert.Equal(t, "container", hook.Type)
	assert.Equal(t, OnFailureFail, hook.OnFailure)
	require.NotNil(t, hook.Retry)
	require.NotNil(t, hook.Retry.MaxAttempts)
	assert.Equal(t, 3, *hook.Retry.MaxAttempts)
	assert.Equal(t, "build", hook.With["action"])
}

func TestStepFromHookDecodesNestedConfig(t *testing.T) {
	hook := &Hook{
		Kind: stepKindName,
		Type: "say",
		Retry: &schema.RetryConfig{
			MaxAttempts: func() *int { n := 5; return &n }(),
		},
		With: map[string]any{
			"content": "deployed",
			"voice":   []any{"Alex", "Daniel"},
			"viewport": map[string]any{
				"height": 10,
				"width":  80,
			},
		},
	}

	ws, err := stepFromHook(hook)
	require.NoError(t, err)

	// Envelope wins: type + retry are set from the hook, not `with`.
	assert.Equal(t, "say", ws.Type)
	require.NotNil(t, ws.Retry)
	assert.Equal(t, 5, *ws.Retry.MaxAttempts)
	assert.Equal(t, "hook:say", ws.Name)

	// `with:` decodes into the step, including scalars, slices, and nested structs.
	assert.Equal(t, "deployed", ws.Content)
	require.Len(t, ws.Voice, 2)
	assert.Equal(t, "Alex", ws.Voice[0])
	assert.Equal(t, "Daniel", ws.Voice[1])
	require.NotNil(t, ws.Viewport)
	assert.Equal(t, 10, ws.Viewport.Height)
	assert.Equal(t, 80, ws.Viewport.Width)
}

func TestStepEngineRunLog(t *testing.T) {
	hook := &Hook{
		Kind: stepKindName,
		Type: "log",
		With: map[string]any{"content": "deployed"},
	}

	out, err := stepEngine{}.Run(stepExecContext(hook))
	require.NoError(t, err)
	require.NotNil(t, out)
	require.NotNil(t, out.Summary)
	assert.Equal(t, StatusSuccess, out.Summary.Status)
}

func TestStepEngineSeedsAtmosEnv(t *testing.T) {
	var captured map[string]string
	runnerstep.Register(&envCaptureHandler{
		BaseHandler: runnerstep.NewBaseHandler("env-capture-test", runnerstep.CategoryCommand, false),
		captured:    &captured,
	})

	hook := &Hook{
		Kind: stepKindName,
		Type: "env-capture-test",
		Env:  map[string]string{"CUSTOM_HOOK_VAR": "from-hook"},
	}

	_, err := stepEngine{}.Run(stepExecContext(hook))
	require.NoError(t, err)
	assert.Equal(t, "test-stack", captured["ATMOS_STACK"])
	assert.Equal(t, "test-component", captured["ATMOS_COMPONENT"])
	assert.Equal(t, "from-hook", captured["CUSTOM_HOOK_VAR"])
}

func TestRunsOnStatus(t *testing.T) {
	cases := []struct {
		when   schema.Condition
		status RunStatus
		want   bool
	}{
		{schema.Condition{}, RunSuccess, true},  // default: success-only.
		{schema.Condition{}, RunFailure, false}, // default: does not run on failure.
		{schema.MustCondition(WhenSuccess), RunSuccess, true},
		{schema.MustCondition(WhenSuccess), RunFailure, false},
		{schema.MustCondition(WhenFailure), RunFailure, true},
		{schema.MustCondition(WhenFailure), RunSuccess, false},
		{schema.MustCondition(WhenAlways), RunSuccess, true},
		{schema.MustCondition(WhenAlways), RunFailure, true},
		{schema.MustCondition("bogus"), RunSuccess, false},
		{schema.MustCondition("bogus"), RunFailure, false},
		{schema.MustCondition([]any{"ci", WhenSuccess}), RunSuccess, false},
	}
	for _, c := range cases {
		hook := Hook{When: c.when}
		got := hook.RunsOnStatus(c.status)
		assert.Equalf(t, c.want, got, "status=%q", c.status)
	}

	t.Run("ci-only condition still requires success unless always is explicit", func(t *testing.T) {
		ciOnly := Hook{When: schema.MustCondition("ci")}
		assert.True(t, ciOnly.RunsWhen(RunSuccess, true))
		assert.False(t, ciOnly.RunsWhen(RunFailure, true))
		assert.False(t, ciOnly.RunsWhen(RunSuccess, false))

		ciAlways := Hook{When: schema.MustCondition([]any{"ci", WhenAlways})}
		assert.True(t, ciAlways.RunsWhen(RunSuccess, true))
		assert.True(t, ciAlways.RunsWhen(RunFailure, true))
		assert.False(t, ciAlways.RunsWhen(RunFailure, false))
	})
}

func TestWithOutcomeTemplateData(t *testing.T) {
	data := withOutcomeTemplateData(
		map[string]any{"atmos_component": "vpc"},
		Outcome{Status: RunFailure, Err: errors.New("boom"), ExitCode: 1},
	)
	assert.Equal(t, "vpc", data["atmos_component"], "existing section keys are preserved")
	assert.Equal(t, "failure", data["status"])
	assert.Equal(t, 1, data["exit_code"])
	assert.Equal(t, "boom", data["error"])

	// A nil section still yields the outcome keys (no panic).
	empty := withOutcomeTemplateData(nil, Outcome{Status: RunSuccess})
	assert.Equal(t, "success", empty["status"])
	assert.Equal(t, "", empty["error"])
	assert.Equal(t, 0, empty["exit_code"])
}

func TestResolveHookRendersOutcomeInWith(t *testing.T) {
	hooks := &Hooks{
		sections: map[string]any{
			"atmos_component": "vpc",
			"stack":           "foobar",
			"hooks": map[string]any{
				"announce": map[string]any{
					"kind": stepKindName,
					"type": "say",
					"with": map[string]any{
						"message": "{{ .atmos_component }} in {{ .stack }}: {{ .status }}",
					},
				},
			},
		},
	}

	outcome := Outcome{Status: RunFailure, Err: errors.New("apply boom"), ExitCode: 2}
	resolved, err := hooks.resolveHookForExecution(
		"announce", &Hook{Kind: stepKindName, Type: "say"},
		&schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{}, outcome,
	)
	require.NoError(t, err)
	require.NotNil(t, resolved.With)
	// Component and stack come from the section; status from the outcome.
	assert.Equal(t, "vpc in foobar: failure", resolved.With["message"])
}

func TestResolveHookRendersSayApplyOutcome(t *testing.T) {
	hooks := &Hooks{
		sections: map[string]any{
			"atmos_component": "hello-world",
			"stack":           "test",
			"hooks": map[string]any{
				"announce-apply": map[string]any{
					"kind": stepKindName,
					"type": "say",
					"with": map[string]any{
						"content": `{{ if eq .status "success" -}}
Terraform apply for {{ .atmos_component }} in {{ .stack }} was successful.
{{- else -}}
Terraform apply for {{ .atmos_component }} in {{ .stack }} was not successful.
{{- if .error }} {{ .error }}{{ end -}}
{{- end }}`,
					},
				},
			},
		},
	}

	success, err := hooks.resolveHookForExecution(
		"announce-apply", &Hook{Kind: stepKindName, Type: "say"},
		&schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{}, Outcome{Status: RunSuccess},
	)
	require.NoError(t, err)
	assert.Equal(t, "Terraform apply for hello-world in test was successful.", success.With["content"])

	failure, err := hooks.resolveHookForExecution(
		"announce-apply", &Hook{Kind: stepKindName, Type: "say"},
		&schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{}, Outcome{Status: RunFailure, Err: errors.New("apply boom"), ExitCode: 1},
	)
	require.NoError(t, err)
	assert.Equal(t, "Terraform apply for hello-world in test was not successful. apply boom", failure.With["content"])
}

func TestStepEngineSeedsOutcomeEnv(t *testing.T) {
	var captured map[string]string
	runnerstep.Register(&envCaptureHandler{
		BaseHandler: runnerstep.NewBaseHandler("env-outcome-test", runnerstep.CategoryCommand, false),
		captured:    &captured,
	})

	ctx := stepExecContext(&Hook{Kind: stepKindName, Type: "env-outcome-test"})
	ctx.Outcome = Outcome{Status: RunFailure, Err: errors.New("apply boom"), ExitCode: 2}

	_, err := stepEngine{}.Run(ctx)
	require.NoError(t, err)
	assert.Equal(t, "failure", captured["ATMOS_HOOK_STATUS"])
	assert.Equal(t, "2", captured["ATMOS_HOOK_EXIT_CODE"])
	assert.Equal(t, "apply boom", captured["ATMOS_HOOK_ERROR"])
}

func TestStepEngineMissingType(t *testing.T) {
	_, err := stepEngine{}.Run(stepExecContext(&Hook{Kind: stepKindName}))
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidConfig)
}

func TestStepEngineUnknownType(t *testing.T) {
	hook := &Hook{Kind: stepKindName, Type: "does-not-exist"}
	_, err := stepEngine{}.Run(stepExecContext(hook))
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrUnknownStepType)
}

func TestStepEngineOnFailure(t *testing.T) {
	// A `log` step with no content fails Validate deterministically and
	// cross-platform — no external binary needed.
	newHook := func(onFailure string) *Hook {
		return &Hook{Kind: stepKindName, Type: "log", OnFailure: onFailure, With: map[string]any{}}
	}

	t.Run("fail propagates", func(t *testing.T) {
		_, err := stepEngine{}.Run(stepExecContext(newHook(OnFailureFail)))
		require.Error(t, err)
	})
	t.Run("warn swallows", func(t *testing.T) {
		out, err := stepEngine{}.Run(stepExecContext(newHook(OnFailureWarn)))
		require.NoError(t, err)
		require.NotNil(t, out.Summary)
		assert.Equal(t, StatusFailure, out.Summary.Status)
	})
	t.Run("ignore swallows", func(t *testing.T) {
		_, err := stepEngine{}.Run(stepExecContext(newHook(OnFailureIgnore)))
		require.NoError(t, err)
	})
	t.Run("default (empty) warns and swallows", func(t *testing.T) {
		_, err := stepEngine{}.Run(stepExecContext(newHook("")))
		require.NoError(t, err)
	})
}

func TestStepEngineRetry(t *testing.T) {
	maxAttempts := func(n int) *schema.RetryConfig {
		return &schema.RetryConfig{MaxAttempts: &n}
	}

	t.Run("retries up to max_attempts", func(t *testing.T) {
		attempts := 0
		runnerstep.Register(&flakyHandler{
			BaseHandler: runnerstep.NewBaseHandler("flaky-retry-test", runnerstep.CategoryCommand, false),
			attempts:    &attempts,
			failUntil:   3, // fail attempts 1 and 2, succeed on 3.
		})
		hook := &Hook{Kind: stepKindName, Type: "flaky-retry-test", Retry: maxAttempts(3)}
		_, err := stepEngine{}.Run(stepExecContext(hook))
		require.NoError(t, err)
		assert.Equal(t, 3, attempts)
	})

	t.Run("no retry when retry absent", func(t *testing.T) {
		attempts := 0
		runnerstep.Register(&flakyHandler{
			BaseHandler: runnerstep.NewBaseHandler("flaky-once-test", runnerstep.CategoryCommand, false),
			attempts:    &attempts,
			failUntil:   3, // would fail without retry.
		})
		hook := &Hook{Kind: stepKindName, Type: "flaky-once-test", OnFailure: OnFailureFail}
		_, err := stepEngine{}.Run(stepExecContext(hook))
		require.Error(t, err)
		assert.Equal(t, 1, attempts, "must run exactly once without a retry block")
	})
}
