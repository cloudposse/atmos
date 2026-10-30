package execution

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func errorHook(name string) schema.ComponentHook {
	return schema.ComponentHook{Name: name, Commands: []string{"plan"}, OnErrors: []string{".*"}, Execute: []string{"tool", name}}
}

func TestRunCancellationDuringActionIsReported(t *testing.T) {
	failure := errors.New("exit status 1")
	for _, test := range []struct {
		name        string
		actionErr   error
		wantFailure bool
	}{
		{"successful action", nil, false},
		{"failed action", failure, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			r := &recorder{}
			config := &schema.ComponentExecution{
				BeforeHooks: []schema.ComponentHook{hook("b", "plan")},
				AfterHooks:  []schema.ComponentHook{{Name: "a", Commands: []string{"plan"}, Execute: []string{"tool"}, RunOnError: true}},
				ErrorHooks:  []schema.ComponentHook{errorHook("e")},
			}
			err := Run(ctx, config, "plan", r, func() (string, error) {
				r.calls = append(r.calls, "action")
				cancel()
				return "", test.actionErr
			})
			require.ErrorIs(t, err, context.Canceled)
			require.Equal(t, test.wantFailure, errors.Is(err, failure), "the action's own failure must stay visible")
			require.Equal(t, []string{"before_hook:b", "action"}, r.calls, "no later hook may run after cancellation")
		})
	}
}

func TestRunCancellationDuringAfterHookSkipsErrorHooks(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	failure := errors.New("exit status 2")
	r := &recorder{failures: map[string]error{"action": failure}, onCall: func(name string) {
		if name == "after_hook:a" {
			cancel()
		}
	}}
	config := &schema.ComponentExecution{
		AfterHooks: []schema.ComponentHook{{Name: "a", Commands: []string{"plan"}, Execute: []string{"tool"}, RunOnError: true}},
		ErrorHooks: []schema.ComponentHook{errorHook("e")},
	}
	err := Run(ctx, config, "plan", r, r.action)
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, err, failure)
	require.Equal(t, []string{"action", "after_hook:a"}, r.calls)
}

func TestRunCancellationBetweenErrorHooksStopsRemainingHooks(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	failure := errors.New("exit status 3")
	r := &recorder{failures: map[string]error{"action": failure}, onCall: func(name string) {
		if name == "error_hook:e1" {
			cancel()
		}
	}}
	config := &schema.ComponentExecution{ErrorHooks: []schema.ComponentHook{errorHook("e1"), errorHook("e2")}}
	err := Run(ctx, config, "plan", r, r.action)
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, err, failure)
	require.Equal(t, []string{"action", "error_hook:e1"}, r.calls, "the second error hook must not start once canceled")
}

func TestJoinFailuresKeepsLoneErrorIntact(t *testing.T) {
	lone := errors.New("only")
	require.Same(t, lone, joinFailures([]failure{{err: lone}}, nil))
	require.Same(t, lone, joinFailures([]failure{{err: lone}}, lone), "an already-recorded cause is not duplicated")
	require.NoError(t, joinFailures(nil, nil))
	other := errors.New("other")
	joined := joinFailures([]failure{{err: lone}}, other)
	require.ErrorIs(t, joined, lone)
	require.ErrorIs(t, joined, other)
}
