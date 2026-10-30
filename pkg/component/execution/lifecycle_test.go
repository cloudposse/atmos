package execution

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// recorder implements Runner and records every hook and the action in call order.
type recorder struct {
	calls []string
	// failures maps a hook name (or "action") to the error it returns.
	failures map[string]error
	// output maps a hook name (or "action") to the output it produces.
	output map[string]string
	// onCall runs on every call, for cancellation tests.
	onCall func(name string)
}

func (r *recorder) RunHook(_ context.Context, kind HookKind, hook *schema.ComponentHook) (string, error) {
	name := string(kind) + ":" + hook.Name
	r.calls = append(r.calls, name)
	if r.onCall != nil {
		r.onCall(name)
	}
	return r.output[hook.Name], r.failures[hook.Name]
}

func (r *recorder) action() (string, error) {
	r.calls = append(r.calls, "action")
	if r.onCall != nil {
		r.onCall("action")
	}
	return r.output["action"], r.failures["action"]
}

func hook(name string, commands ...string) schema.ComponentHook {
	return schema.ComponentHook{Name: name, Commands: commands, Execute: []string{"tool", name}}
}

func runLifecycle(t *testing.T, config *schema.ComponentExecution, command string, r *recorder) error {
	t.Helper()
	return Run(t.Context(), config, command, r, r.action)
}

func TestRunOrdersBeforeActionAfter(t *testing.T) {
	r := &recorder{}
	config := &schema.ComponentExecution{
		BeforeHooks: []schema.ComponentHook{hook("b1", "plan"), hook("b2", "plan"), hook("skip", "apply")},
		AfterHooks:  []schema.ComponentHook{hook("a1", "plan"), hook("a2", "plan")},
		ErrorHooks:  []schema.ComponentHook{{Name: "e1", Commands: []string{"plan"}, OnErrors: []string{".*"}, Execute: []string{"tool"}}},
	}
	require.NoError(t, runLifecycle(t, config, "plan", r))
	require.Equal(t, []string{"before_hook:b1", "before_hook:b2", "action", "after_hook:a1", "after_hook:a2"}, r.calls)
}

func TestRunWithoutExecutionJustRunsAction(t *testing.T) {
	for _, config := range []*schema.ComponentExecution{nil, {}} {
		r := &recorder{}
		require.NoError(t, runLifecycle(t, config, "plan", r))
		require.Equal(t, []string{"action"}, r.calls)
	}
}

func TestRunDoesNotFireHooksForOtherCommands(t *testing.T) {
	r := &recorder{failures: map[string]error{"action": errors.New("boom")}}
	config := &schema.ComponentExecution{
		BeforeHooks: []schema.ComponentHook{hook("b", "init")},
		AfterHooks:  []schema.ComponentHook{{Name: "a", Commands: []string{"init"}, Execute: []string{"tool"}, RunOnError: true}},
		ErrorHooks:  []schema.ComponentHook{{Name: "e", Commands: []string{"init"}, OnErrors: []string{".*"}, Execute: []string{"tool"}}},
	}
	err := runLifecycle(t, config, "plan", r)
	require.ErrorContains(t, err, "boom")
	require.Equal(t, []string{"action"}, r.calls)
}

func TestRunBeforeHookFailureAbortsActionAndKeepsExitCode(t *testing.T) {
	r := &recorder{failures: map[string]error{"b1": errUtils.ExitCodeError{Code: 23}}}
	config := &schema.ComponentExecution{
		BeforeHooks: []schema.ComponentHook{hook("b1", "apply"), hook("b2", "apply")},
		AfterHooks:  []schema.ComponentHook{hook("a", "apply")},
	}
	err := runLifecycle(t, config, "apply", r)
	require.ErrorIs(t, err, errUtils.ErrComponentHookFailed)
	var exit errUtils.ExitCodeError
	require.ErrorAs(t, err, &exit)
	require.Equal(t, 23, exit.Code)
	require.Equal(t, []string{"before_hook:b1"}, r.calls)
}

func TestRunRunOnErrorHooksStillRunAfterBeforeHookFailure(t *testing.T) {
	r := &recorder{failures: map[string]error{"b1": errors.New("first")}}
	always := hook("b2", "plan")
	always.RunOnError = true
	config := &schema.ComponentExecution{
		BeforeHooks: []schema.ComponentHook{hook("b1", "plan"), hook("b-skipped", "plan"), always},
		AfterHooks:  []schema.ComponentHook{hook("a-skipped", "plan"), {Name: "a-always", Commands: []string{"plan"}, Execute: []string{"tool"}, RunOnError: true}},
	}
	err := runLifecycle(t, config, "plan", r)
	require.Error(t, err)
	require.Equal(t, []string{"before_hook:b1", "before_hook:b2", "after_hook:a-always"}, r.calls)
}

func TestRunAfterHooksFollowRunOnError(t *testing.T) {
	for _, test := range []struct {
		name       string
		runOnError bool
		want       []string
	}{
		{"skipped after failure", false, []string{"action"}},
		{"runs after failure when requested", true, []string{"action", "after_hook:a"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := &recorder{failures: map[string]error{"action": errors.New("boom")}}
			after := hook("a", "plan")
			after.RunOnError = test.runOnError
			err := runLifecycle(t, &schema.ComponentExecution{AfterHooks: []schema.ComponentHook{after}}, "plan", r)
			require.ErrorContains(t, err, "boom")
			require.Equal(t, test.want, r.calls)
		})
	}
}

func TestRunFailedAfterHookSuppressesLaterOrdinaryAfterHooks(t *testing.T) {
	r := &recorder{failures: map[string]error{"a1": errors.New("after failed")}}
	config := &schema.ComponentExecution{AfterHooks: []schema.ComponentHook{hook("a1", "plan"), hook("a2", "plan")}}
	err := runLifecycle(t, config, "plan", r)
	require.ErrorIs(t, err, errUtils.ErrComponentHookFailed)
	require.Equal(t, []string{"action", "after_hook:a1"}, r.calls)
}

func TestRunErrorHooks(t *testing.T) {
	for _, test := range []struct {
		name         string
		command      string
		patterns     []string
		failures     map[string]error
		output       map[string]string
		withBeforeOn bool
		want         []string
	}{
		{name: "matches action error text", command: "apply", patterns: []string{"timeout"}, failures: map[string]error{"action": errors.New("request timeout")}, want: []string{"action", "error_hook:e"}},
		{name: "matches action output", command: "apply", patterns: []string{"AccessDenied"}, failures: map[string]error{"action": errors.New("exit status 1")}, output: map[string]string{"action": "Error: AccessDenied"}, want: []string{"action", "error_hook:e"}},
		{name: "any of several patterns", command: "apply", patterns: []string{"nope", "^exit"}, failures: map[string]error{"action": errors.New("exit status 1")}, want: []string{"action", "error_hook:e"}},
		{name: "no match", command: "apply", patterns: []string{"timeout"}, failures: map[string]error{"action": errors.New("other")}, want: []string{"action"}},
		{name: "no failure", command: "apply", patterns: []string{".*"}, want: []string{"action"}},
		{name: "other command", command: "plan", patterns: []string{".*"}, failures: map[string]error{"action": errors.New("boom")}, want: []string{"action"}},
		{name: "before hook failure", command: "apply", patterns: []string{"setup broke"}, failures: map[string]error{"b": errors.New("setup broke")}, withBeforeOn: true, want: []string{"before_hook:b", "error_hook:e"}},
		{name: "no patterns never fires", command: "apply", failures: map[string]error{"action": errors.New("boom")}, want: []string{"action"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := &recorder{failures: test.failures, output: test.output}
			config := &schema.ComponentExecution{
				ErrorHooks: []schema.ComponentHook{{Name: "e", Commands: []string{"apply"}, OnErrors: test.patterns, Execute: []string{"tool"}}},
			}
			if test.withBeforeOn {
				config.BeforeHooks = []schema.ComponentHook{hook("b", "apply")}
			}
			_ = runLifecycle(t, config, test.command, r)
			require.Equal(t, test.want, r.calls)
		})
	}
}

func TestRunErrorHooksSeeAfterHookFailures(t *testing.T) {
	r := &recorder{failures: map[string]error{"a": errors.New("cleanup exploded")}}
	config := &schema.ComponentExecution{
		AfterHooks: []schema.ComponentHook{hook("a", "plan")},
		ErrorHooks: []schema.ComponentHook{{Name: "e", Commands: []string{"plan"}, OnErrors: []string{"cleanup exploded"}, Execute: []string{"tool"}}},
	}
	require.Error(t, runLifecycle(t, config, "plan", r))
	require.Equal(t, []string{"action", "after_hook:a", "error_hook:e"}, r.calls)
}

func TestRunInvalidErrorPatternIsReportedAndOthersStillRun(t *testing.T) {
	r := &recorder{failures: map[string]error{"action": errors.New("boom")}}
	config := &schema.ComponentExecution{ErrorHooks: []schema.ComponentHook{
		{Name: "bad", Commands: []string{"plan"}, OnErrors: []string{"("}, Execute: []string{"tool"}},
		{Name: "good", Commands: []string{"plan"}, OnErrors: []string{"boom"}, Execute: []string{"tool"}},
	}}
	err := runLifecycle(t, config, "plan", r)
	require.ErrorContains(t, err, `error hook "bad"`)
	require.Equal(t, []string{"action", "error_hook:good"}, r.calls)
}

func TestRunErrorHookFailureIsJoinedWithOriginalError(t *testing.T) {
	r := &recorder{failures: map[string]error{"action": errors.New("boom"), "e": errUtils.ExitCodeError{Code: 9}}}
	config := &schema.ComponentExecution{ErrorHooks: []schema.ComponentHook{{Name: "e", Commands: []string{"plan"}, OnErrors: []string{"boom"}, Execute: []string{"tool"}}}}
	err := runLifecycle(t, config, "plan", r)
	require.ErrorContains(t, err, "boom")
	require.ErrorIs(t, err, errUtils.ErrComponentHookFailed)
}

func TestRunPassesFailedOutputToErrorHooksOnlyThroughMatching(t *testing.T) {
	r := &recorder{failures: map[string]error{"b": errors.New("x")}, output: map[string]string{"b": "hook said NEEDLE"}}
	config := &schema.ComponentExecution{
		BeforeHooks: []schema.ComponentHook{hook("b", "plan")},
		ErrorHooks:  []schema.ComponentHook{{Name: "e", Commands: []string{"plan"}, OnErrors: []string{"NEEDLE"}, Execute: []string{"tool"}}},
	}
	_ = runLifecycle(t, config, "plan", r)
	require.Equal(t, []string{"before_hook:b", "error_hook:e"}, r.calls)
}

func TestRunStopsWhenCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	r := &recorder{onCall: func(name string) {
		if name == "before_hook:b1" {
			cancel()
		}
	}}
	config := &schema.ComponentExecution{
		BeforeHooks: []schema.ComponentHook{hook("b1", "plan"), hook("b2", "plan")},
		AfterHooks:  []schema.ComponentHook{{Name: "a", Commands: []string{"plan"}, Execute: []string{"tool"}, RunOnError: true}},
		ErrorHooks:  []schema.ComponentHook{{Name: "e", Commands: []string{"plan"}, OnErrors: []string{".*"}, Execute: []string{"tool"}}},
	}
	err := Run(ctx, config, "plan", r, r.action)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, []string{"before_hook:b1"}, r.calls)
}

func TestRunDoesNotStartWhenAlreadyCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	r := &recorder{}
	err := Run(ctx, &schema.ComponentExecution{BeforeHooks: []schema.ComponentHook{hook("b", "plan")}}, "plan", r, r.action)
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, r.calls)
}

func TestRunErrorMessageNamesTheHook(t *testing.T) {
	r := &recorder{failures: map[string]error{"lint": errors.New("exit status 3")}}
	err := runLifecycle(t, &schema.ComponentExecution{BeforeHooks: []schema.ComponentHook{hook("lint", "plan")}}, "plan", r)
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "lint"))
	require.True(t, strings.Contains(err.Error(), "before_hook"))
}

func TestValidate(t *testing.T) {
	require.NoError(t, Validate(nil))
	require.NoError(t, Validate(&schema.ComponentExecution{
		BeforeHooks: []schema.ComponentHook{hook("ok", "plan")},
		ErrorHooks:  []schema.ComponentHook{{Name: "e", Commands: []string{"plan"}, OnErrors: []string{"^x$"}, Execute: []string{"tool"}}},
	}))
	for _, test := range []struct {
		name   string
		config *schema.ComponentExecution
	}{
		{"empty execute", &schema.ComponentExecution{BeforeHooks: []schema.ComponentHook{{Name: "h", Commands: []string{"plan"}}}}},
		{"blank executable", &schema.ComponentExecution{AfterHooks: []schema.ComponentHook{{Name: "h", Commands: []string{"plan"}, Execute: []string{""}}}}},
		{"bad pattern", &schema.ComponentExecution{ErrorHooks: []schema.ComponentHook{{Name: "h", Commands: []string{"plan"}, Execute: []string{"t"}, OnErrors: []string{"("}}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.ErrorIs(t, Validate(test.config), errUtils.ErrPluginProtocol)
		})
	}
}
