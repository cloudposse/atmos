package execution

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -destination=mock_reporter_test.go -package=execution github.com/cloudposse/atmos/pkg/component/execution Reporter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// recorder records every hook and the action in call order. Tests drive it through the
// generated MockRunner returned by runner.
type recorder struct {
	calls []string
	// failures maps a hook name (or "action") to the error it returns.
	failures map[string]error
	// output maps a hook name (or "action") to the output it produces.
	output map[string]string
	// onCall runs on every call, for cancellation tests.
	onCall func(name string)
}

// runner returns a generated Runner mock whose RunHook calls feed the recorder.
func (r *recorder) runner(t *testing.T) Runner {
	t.Helper()
	mock := NewMockRunner(gomock.NewController(t))
	mock.EXPECT().RunHook(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(r.runHook).AnyTimes()
	return mock
}

func (r *recorder) runHook(_ context.Context, kind HookKind, hook *schema.ComponentHook) (Output, error) {
	name := string(kind) + ":" + hook.Name
	r.calls = append(r.calls, name)
	if r.onCall != nil {
		r.onCall(name)
	}
	return TextOutput(r.output[hook.Name]), r.failures[hook.Name]
}

func (r *recorder) action() (Output, error) {
	r.calls = append(r.calls, "action")
	if r.onCall != nil {
		r.onCall("action")
	}
	return TextOutput(r.output["action"]), r.failures["action"]
}

func hook(name string, commands ...string) schema.ComponentHook {
	return schema.ComponentHook{Name: name, Commands: commands, Execute: []string{"tool", name}}
}

func runLifecycle(t *testing.T, config *schema.ComponentExecution, command string, r *recorder) error {
	t.Helper()
	return Run(t.Context(), config, command, r.runner(t), r.action)
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
	err := Run(ctx, config, "plan", r.runner(t), r.action)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, []string{"before_hook:b1"}, r.calls)
}

func TestRunDoesNotStartWhenAlreadyCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	r := &recorder{}
	err := Run(ctx, &schema.ComponentExecution{BeforeHooks: []schema.ComponentHook{hook("b", "plan")}}, "plan", r.runner(t), r.action)
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, r.calls)
}

func TestRunDoesNotStartActionWhenLastBeforeHookIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	r := &recorder{onCall: func(name string) {
		if name == "before_hook:last" {
			cancel()
		}
	}}
	config := &schema.ComponentExecution{BeforeHooks: []schema.ComponentHook{hook("first", "plan"), hook("last", "plan")}}
	err := Run(ctx, config, "plan", r.runner(t), r.action)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, []string{"before_hook:first", "before_hook:last"}, r.calls)
}

func TestRunWithoutHooksDoesNotStartWhenAlreadyCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	for _, config := range []*schema.ComponentExecution{nil, {}} {
		r := &recorder{}
		err := Run(ctx, config, "plan", r.runner(t), r.action)
		require.ErrorIs(t, err, context.Canceled)
		require.Empty(t, r.calls)
	}
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

// recoveryRun drives a Recoverer with a scripted sequence of failures and records the pauses.
type recoveryRun struct {
	failures []error
	texts    []string
	attempts int
	pauses   []time.Duration
	mu       sync.Mutex
}

// count reports the attempts made so far, for readers that run beside the recoverer.
func (r *recoveryRun) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.attempts
}

func (r *recoveryRun) attempt(context.Context) (string, error) {
	r.mu.Lock()
	index := r.attempts
	r.attempts++
	r.mu.Unlock()
	var text string
	if index < len(r.texts) {
		text = r.texts[index]
	}
	if index < len(r.failures) {
		return text, r.failures[index]
	}
	return text, nil
}

func (r *recoveryRun) sleep(_ context.Context, d time.Duration) error {
	r.pauses = append(r.pauses, d)
	return nil
}

var errExit = errors.New("exit status 1")

func TestRecovererWithoutPolicyReturnsTheFailure(t *testing.T) {
	var nilRecoverer *Recoverer
	for name, recoverer := range map[string]*Recoverer{"nil recoverer": nilRecoverer, "nil policy": NewRecoverer(nil), "empty policy": NewRecoverer(&schema.ComponentErrors{})} {
		t.Run(name, func(t *testing.T) {
			run := &recoveryRun{failures: []error{errExit}, texts: []string{"timeout"}}
			ignored, err := recoverer.Run(t.Context(), run.attempt)
			require.ErrorIs(t, err, errExit)
			require.False(t, ignored)
			require.Equal(t, 1, run.attempts)
		})
	}
	ignored, err := NewRecoverer(&schema.ComponentErrors{}).Run(t.Context(), (&recoveryRun{}).attempt)
	require.NoError(t, err)
	require.False(t, ignored)
}

func TestRecovererRetriesUntilSuccess(t *testing.T) {
	reporter := NewMockReporter(gomock.NewController(t))
	gomock.InOrder(reporter.EXPECT().Retrying("net", 1, 3, 7*time.Second), reporter.EXPECT().Retrying("net", 2, 3, 7*time.Second))
	run := &recoveryRun{failures: []error{errExit, errExit}, texts: []string{"Error: connection timeout", "Error: connection timeout"}}
	policy := &schema.ComponentErrors{Retry: []schema.ComponentRetryPolicy{{Name: "net", Patterns: []string{"timeout"}, MaxAttempts: 3, SleepInterval: 7 * time.Second}}}
	ignored, err := NewRecoverer(policy, WithReporter(reporter), WithSleep(run.sleep)).Run(t.Context(), run.attempt)
	require.NoError(t, err)
	require.False(t, ignored)
	require.Equal(t, 3, run.attempts)
	require.Equal(t, []time.Duration{7 * time.Second, 7 * time.Second}, run.pauses)
}

func TestRecovererStopsAtMaxAttempts(t *testing.T) {
	run := &recoveryRun{failures: []error{errExit, errExit, errExit, errExit}, texts: []string{"timeout", "timeout", "timeout", "timeout"}}
	policy := &schema.ComponentErrors{Retry: []schema.ComponentRetryPolicy{{Name: "net", Patterns: []string{"timeout"}, MaxAttempts: 3}}}
	_, err := NewRecoverer(policy, WithSleep(run.sleep)).Run(t.Context(), run.attempt)
	require.ErrorIs(t, err, errUtils.ErrComponentMaxAttempts)
	require.ErrorIs(t, err, errExit, "the last failure stays in the chain")
	require.Equal(t, 3, run.attempts, "max_attempts counts the first attempt")
	require.Len(t, run.pauses, 2)
}

func TestRecovererDoesNotRetryUnmatchedErrors(t *testing.T) {
	run := &recoveryRun{failures: []error{errExit}, texts: []string{"permission denied"}}
	policy := &schema.ComponentErrors{Retry: []schema.ComponentRetryPolicy{{Name: "net", Patterns: []string{"timeout"}, MaxAttempts: 3}}}
	_, err := NewRecoverer(policy, WithSleep(run.sleep)).Run(t.Context(), run.attempt)
	require.ErrorIs(t, err, errExit)
	require.Equal(t, 1, run.attempts)
	require.Empty(t, run.pauses)
}

func TestRecovererMatchesStderrOrFallsBackToError(t *testing.T) {
	policy := &schema.ComponentErrors{Ignore: []schema.ComponentIgnorePolicy{{Name: "exit", Patterns: []string{"exit status"}}}}
	for _, tc := range []struct {
		name, stderr string
		ignored      bool
	}{
		{name: "stderr takes precedence", stderr: "permission denied", ignored: false},
		{name: "empty stderr falls back", stderr: "", ignored: true},
		{name: "blank stderr falls back", stderr: " \n", ignored: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ignored, err := NewRecoverer(policy).Run(t.Context(), (&recoveryRun{failures: []error{errExit}, texts: []string{tc.stderr}}).attempt)
			require.Equal(t, tc.ignored, ignored)
			if tc.ignored {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, errExit)
			}
		})
	}
}

// streamOutput is captured output that records how it was read.
type streamOutput struct {
	text    string
	openErr error
	readErr error
	closed  int
}

func (o *streamOutput) Open() (io.Reader, error) {
	if o.openErr != nil {
		return nil, o.openErr
	}
	var reader io.Reader = strings.NewReader(o.text)
	if o.readErr != nil {
		reader = io.MultiReader(reader, &failingReader{err: o.readErr})
	}
	return &closingReader{Reader: reader, output: o}, nil
}

type closingReader struct {
	io.Reader
	output *streamOutput
}

func (r *closingReader) Close() error {
	r.output.closed++
	return nil
}

type failingReader struct{ err error }

func (r *failingReader) Read([]byte) (int, error) { return 0, r.err }

// TestErrorHooksStreamCapturedOutput verifies error hook matching reads captured output as a
// stream, closes every reader, keeps matching across failure boundaries, and reports capture
// failures instead of silently skipping the hook.
func TestErrorHooksStreamCapturedOutput(t *testing.T) {
	failed := errors.New("exit status 1")
	readFailure := errors.New("capture read failed")
	large := strings.Repeat("x", 4<<20) + "\nError: AccessDenied\n"
	for _, test := range []struct {
		name    string
		output  *streamOutput
		pattern string
		hookRan bool
		wantErr error
	}{
		{name: "match at end of large output", output: &streamOutput{text: large}, pattern: "AccessDenied", hookRan: true},
		{name: "match spans error and output", output: &streamOutput{text: "Error: Throttled"}, pattern: "(?s)exit status 1.*Throttled", hookRan: true},
		{name: "anchored pattern on the whole text", output: &streamOutput{text: "tail"}, pattern: "^exit status 1\ntail\n$", hookRan: true},
		{name: "no match", output: &streamOutput{text: large}, pattern: "Throttled"},
		{name: "capture cannot be opened", output: &streamOutput{openErr: readFailure}, pattern: ".*", wantErr: readFailure},
		{name: "capture read fails", output: &streamOutput{text: "partial", readErr: readFailure}, pattern: "never", wantErr: readFailure},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := &recorder{}
			config := &schema.ComponentExecution{ErrorHooks: []schema.ComponentHook{{Name: "e", Commands: []string{"plan"}, OnErrors: []string{test.pattern}, Execute: []string{"tool"}}}}
			err := Run(t.Context(), config, "plan", r.runner(t), func() (Output, error) {
				r.calls = append(r.calls, "action")
				return test.output, failed
			})
			require.ErrorIs(t, err, failed, "the action failure stays visible")
			if test.wantErr != nil {
				require.ErrorIs(t, err, test.wantErr)
			}
			require.Equal(t, test.hookRan, slices.Contains(r.calls, "error_hook:e"), r.calls)
			if test.output.openErr == nil {
				require.Equal(t, 1, test.output.closed, "every opened reader is closed")
			}
		})
	}
}

func TestRecovererIgnorePolicyWinsOverRetryAndWritesSignals(t *testing.T) {
	signalsFile := filepath.Join(t.TempDir(), SignalsFileName)
	reporter := NewMockReporter(gomock.NewController(t))
	reporter.EXPECT().Ignored("exists", "fine to reuse")
	reporter.EXPECT().SignalsWritten(signalsFile)
	run := &recoveryRun{failures: []error{errExit}, texts: []string{"Error: bucket already exists"}}
	policy := &schema.ComponentErrors{
		Retry: []schema.ComponentRetryPolicy{{Name: "any", Patterns: []string{".*"}, MaxAttempts: 5}},
		Ignore: []schema.ComponentIgnorePolicy{
			{Name: "unrelated", Patterns: []string{"never"}},
			{Name: "exists", Patterns: []string{"already exists"}, Message: "fine to reuse", Signals: map[string]any{"alert": false, "note": "kept"}},
		},
	}
	ignored, err := NewRecoverer(policy, WithReporter(reporter), WithSignalsFile(signalsFile), WithSleep(run.sleep)).Run(t.Context(), run.attempt)
	require.NoError(t, err)
	require.True(t, ignored)
	require.Equal(t, 1, run.attempts)
	data, err := os.ReadFile(signalsFile)
	require.NoError(t, err)
	var written map[string]any
	require.NoError(t, json.Unmarshal(data, &written))
	require.Equal(t, map[string]any{"alert": false, "note": "kept"}, written)
}

func TestRecovererSignalsDoNotFollowSymlink(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "target")
	require.NoError(t, os.WriteFile(target, []byte("keep"), 0o600))
	signals := filepath.Join(directory, SignalsFileName)
	if err := os.Symlink(target, signals); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	policy := &schema.ComponentErrors{Ignore: []schema.ComponentIgnorePolicy{{Name: "ignore", Patterns: []string{"alpha"}, Signals: map[string]any{"x": 1}}}}
	reporter := NewMockReporter(gomock.NewController(t))
	reporter.EXPECT().Ignored("ignore", "")
	reporter.EXPECT().SignalsWritten(signals)
	ignored, err := NewRecoverer(policy, WithReporter(reporter), WithSignalsFile(signals)).Run(t.Context(), (&recoveryRun{failures: []error{errExit}, texts: []string{"alpha"}}).attempt)
	require.NoError(t, err)
	require.True(t, ignored)
	data, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "keep", string(data))
}

func TestRecovererIgnoreWithoutSignalsOrFileWritesNothing(t *testing.T) {
	directory := t.TempDir()
	policy := &schema.ComponentErrors{Ignore: []schema.ComponentIgnorePolicy{{Name: "a", Patterns: []string{"alpha"}, Signals: map[string]any{"x": 1}}, {Name: "b", Patterns: []string{"beta"}}}}
	reporter := NewMockReporter(gomock.NewController(t))
	reporter.EXPECT().Ignored("a", "")
	ignored, err := NewRecoverer(policy, WithReporter(reporter)).Run(t.Context(), (&recoveryRun{failures: []error{errExit}, texts: []string{"alpha"}}).attempt)
	require.NoError(t, err)
	require.True(t, ignored)

	// A signals file that cannot be written emits a warning and never undoes the ignore.
	var output bytes.Buffer
	log.Default().SetOutput(&output)
	t.Cleanup(func() { log.Default().SetOutput(os.Stderr) })
	blocked := filepath.Join(directory, "missing", SignalsFileName)
	reporter = NewMockReporter(gomock.NewController(t))
	reporter.EXPECT().Ignored("a", "")
	ignored, err = NewRecoverer(policy, WithReporter(reporter), WithSignalsFile(blocked)).Run(t.Context(), (&recoveryRun{failures: []error{errExit}, texts: []string{"alpha"}}).attempt)
	require.NoError(t, err)
	require.True(t, ignored)
	require.NoFileExists(t, blocked)
	require.Contains(t, output.String(), "Failed to write error signals")
	require.Contains(t, output.String(), blocked)
	require.Contains(t, output.String(), "error")

	reporter = NewMockReporter(gomock.NewController(t))
	reporter.EXPECT().Ignored("b", "")
	ignored, err = NewRecoverer(policy, WithReporter(reporter), WithSignalsFile(filepath.Join(directory, SignalsFileName))).Run(t.Context(), (&recoveryRun{failures: []error{errExit}, texts: []string{"beta"}}).attempt)
	require.NoError(t, err)
	require.True(t, ignored)
	require.NoFileExists(t, filepath.Join(directory, SignalsFileName), "a policy without signals writes no file")
}

func TestRecovererNegativePatternVetoesAPolicy(t *testing.T) {
	policy := &schema.ComponentErrors{Retry: []schema.ComponentRetryPolicy{{Name: "net", Patterns: []string{"timeout", "!fatal"}, MaxAttempts: 2}}}
	run := &recoveryRun{failures: []error{errExit}, texts: []string{"timeout and fatal"}}
	_, err := NewRecoverer(policy, WithSleep(run.sleep)).Run(t.Context(), run.attempt)
	require.ErrorIs(t, err, errExit)
	require.Equal(t, 1, run.attempts)

	run = &recoveryRun{failures: []error{errExit}, texts: []string{"timeout only"}}
	_, err = NewRecoverer(policy, WithSleep(run.sleep)).Run(t.Context(), run.attempt)
	require.NoError(t, err)
	require.Equal(t, 2, run.attempts)
}

func TestRecovererMatchesCleanedText(t *testing.T) {
	require.Equal(t, `Error: Failed to load state: dial tcp: i/o timeout`, cleanErrorText("\x1b[31mError:\x1b[0m Failed to load state:\n\tdial tcp:   i/o timeout\n"))
	require.Equal(t, "a b", cleanErrorText("a\n\n\t<b>"))
	policy := &schema.ComponentErrors{Retry: []schema.ComponentRetryPolicy{{Name: "state", Patterns: []string{`Failed to load state: dial tcp: i/o timeout`}, MaxAttempts: 2}}}
	run := &recoveryRun{failures: []error{errExit}, texts: []string{"\x1b[31m│ Error:\x1b[0m Failed to load state:\n│ dial tcp: i/o timeout"}}
	_, err := NewRecoverer(policy, WithSleep(run.sleep)).Run(t.Context(), run.attempt)
	require.NoError(t, err)
	require.Equal(t, 2, run.attempts)
}

func TestRecovererMatchesTheErrorMessageToo(t *testing.T) {
	policy := &schema.ComponentErrors{Ignore: []schema.ComponentIgnorePolicy{{Name: "hook", Patterns: []string{"hook failed"}}}}
	run := &recoveryRun{failures: []error{fmt.Errorf("%w: hook failed", errUtils.ErrComponentHookFailed)}}
	ignored, err := NewRecoverer(policy).Run(t.Context(), run.attempt)
	require.NoError(t, err)
	require.True(t, ignored)
}

func TestRecovererStopsWhenCanceledDuringTheSleep(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	policy := &schema.ComponentErrors{Retry: []schema.ComponentRetryPolicy{{Name: "net", Patterns: []string{"timeout"}, MaxAttempts: 5, SleepInterval: time.Hour}}}
	run := &recoveryRun{failures: []error{errExit, errExit}, texts: []string{"timeout", "timeout"}}
	sleeping := make(chan struct{})
	sleep := func(ctx context.Context, d time.Duration) error {
		close(sleeping)
		return sleepContext(ctx, d)
	}
	done := make(chan error, 1)
	go func() {
		_, err := NewRecoverer(policy, WithSleep(sleep)).Run(ctx, run.attempt)
		done <- err
	}()
	// The first attempt fails and the run sleeps for an hour; cancellation must end it at once.
	select {
	case <-sleeping:
	case <-time.After(5 * time.Second):
		t.Fatal("the recoverer never started sleeping")
	}
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
	require.Equal(t, 1, run.count())
}

func TestRecovererDoesNotRecoverAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	policy := &schema.ComponentErrors{Ignore: []schema.ComponentIgnorePolicy{{Name: "all", Patterns: []string{".*"}}}}
	ignored, err := NewRecoverer(policy).Run(ctx, func(context.Context) (string, error) {
		cancel()
		return "boom", errExit
	})
	require.ErrorIs(t, err, errExit)
	require.False(t, ignored)
}

func TestSleepContext(t *testing.T) {
	require.NoError(t, sleepContext(t.Context(), 0))
	require.NoError(t, sleepContext(t.Context(), time.Millisecond))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, sleepContext(ctx, 0), context.Canceled)
	require.ErrorIs(t, sleepContext(ctx, time.Hour), context.Canceled)
}

func TestExcludeMatches(t *testing.T) {
	for _, test := range []struct {
		actions []string
		command string
		want    bool
	}{
		{[]string{"plan"}, "plan", true},
		{[]string{"PLAN"}, "plan", true},
		{[]string{"plan"}, "apply", false},
		{[]string{"all"}, "output", true},
		{[]string{"all_except_output"}, "output", false},
		{[]string{"all_except_output"}, "destroy", true},
		{nil, "plan", false},
	} {
		require.Equal(t, test.want, ExcludeMatches(test.actions, test.command), "%v %s", test.actions, test.command)
	}
}

func TestLifecycleHookSelection(t *testing.T) {
	hooks := []schema.ComponentHook{
		{Name: "alias", Commands: []string{"plan", CommandReadConfigAlias}},
		{Name: "canonical", Commands: []string{CommandReadConfig}},
		{Name: "module", Commands: []string{CommandInitFromModule}},
	}
	require.True(t, ListsCommand(hooks, CommandInitFromModule))
	require.False(t, ListsCommand(hooks[:2], CommandInitFromModule))
	require.False(t, ListsCommand(nil, CommandReadConfig))
	selected := HooksFor(hooks, CommandReadConfig, CommandReadConfigAlias)
	require.Len(t, selected, 2)
	require.Equal(t, "alias", selected[0].Name)
	require.Equal(t, []string{CommandReadConfig}, selected[0].Commands, "the alias is rewritten to the canonical command")
	require.Equal(t, []string{"plan", CommandReadConfigAlias}, hooks[0].Commands, "the input is not modified")
	require.Empty(t, HooksFor(hooks, "other"))
}

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
			err := Run(ctx, config, "plan", r.runner(t), func() (Output, error) {
				r.calls = append(r.calls, "action")
				cancel()
				return nil, test.actionErr
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
	err := Run(ctx, config, "plan", r.runner(t), r.action)
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
	err := Run(ctx, config, "plan", r.runner(t), r.action)
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
