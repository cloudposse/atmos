package step

import (
	"context"
	"errors"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// stepTimeoutSleepMS is how long the fake child sleeps. It is far longer than any timeout used
// below, so a step that finishes quickly was cut short by its deadline.
const stepTimeoutSleepMS = 30000

// stepTimeoutBudget bounds how long a timed-out step may take to return.
const stepTimeoutBudget = 15 * time.Second

func TestStartStepDeadline(t *testing.T) {
	tests := []struct {
		name    string
		timeout string
		wantErr error
		hasDL   bool
	}{
		{name: "no timeout", timeout: ""},
		{name: "blank timeout", timeout: "  "},
		{name: "valid duration", timeout: "2s", hasDL: true},
		{name: "not a duration", timeout: "soon", wantErr: errUtils.ErrStepTimeoutInvalid},
		{name: "zero", timeout: "0s", wantErr: errUtils.ErrStepTimeoutInvalid},
		{name: "negative", timeout: "-5s", wantErr: errUtils.ErrStepTimeoutInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deadline, err := StartStepDeadline(context.Background(), &schema.WorkflowStep{Name: "s", Timeout: tt.timeout}, nil)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			defer deadline.Stop()
			_, has := deadline.Context().Deadline()
			assert.Equal(t, tt.hasDL, has)
		})
	}
}

func TestStartStepDeadlineRendersTemplatedTimeout(t *testing.T) {
	vars := NewVariables()
	vars.SetFlag("wait", "3s")

	deadline, err := StartStepDeadline(context.Background(), &schema.WorkflowStep{Name: "s", Timeout: "{{ .flags.wait }}"}, vars)

	require.NoError(t, err)
	defer deadline.Stop()
	limit, has := deadline.Context().Deadline()
	require.True(t, has)
	assert.InDelta(t, 3*time.Second, time.Until(limit), float64(time.Second))
}

func TestStepDeadlineWrap(t *testing.T) {
	cause := errors.New("process killed")

	t.Run("nil deadline and nil error pass through", func(t *testing.T) {
		var none *StepDeadline
		assert.NoError(t, none.Wrap(nil))
		assert.Same(t, cause, none.Wrap(cause))
		none.Stop()
	})

	t.Run("a deadline overrun is marked as a step timeout", func(t *testing.T) {
		deadline, err := StartStepDeadline(context.Background(), &schema.WorkflowStep{Name: "slow", Timeout: "1ms"}, nil)
		require.NoError(t, err)
		defer deadline.Stop()
		<-deadline.Context().Done()

		wrapped := deadline.Wrap(cause)

		require.ErrorIs(t, wrapped, errUtils.ErrStepTimeout)
		require.ErrorIs(t, wrapped, cause)
		assert.Same(t, wrapped, deadline.Wrap(wrapped), "an already-marked error is not wrapped twice")
	})

	t.Run("a failure before the deadline is not a timeout", func(t *testing.T) {
		deadline, err := StartStepDeadline(context.Background(), &schema.WorkflowStep{Name: "fast", Timeout: "1h"}, nil)
		require.NoError(t, err)
		defer deadline.Stop()

		wrapped := deadline.Wrap(cause)

		assert.Same(t, cause, wrapped)
		assert.NotErrorIs(t, wrapped, errUtils.ErrStepTimeout)
	})

	t.Run("cancellation by the caller is not a timeout", func(t *testing.T) {
		parent, cancel := context.WithCancel(context.Background())
		deadline, err := StartStepDeadline(parent, &schema.WorkflowStep{Name: "canceled", Timeout: "1h"}, nil)
		require.NoError(t, err)
		defer deadline.Stop()
		cancel()

		wrapped := deadline.Wrap(context.Canceled)

		assert.NotErrorIs(t, wrapped, errUtils.ErrStepTimeout)
	})

	t.Run("a step without a timeout never reports one", func(t *testing.T) {
		deadline, err := StartStepDeadline(context.Background(), &schema.WorkflowStep{Name: "open"}, nil)
		require.NoError(t, err)
		defer deadline.Stop()

		assert.Same(t, cause, deadline.Wrap(cause))
	})
}

func TestRunWithStepDeadline(t *testing.T) {
	t.Run("runs fn under the deadline and marks an overrun", func(t *testing.T) {
		err := RunWithStepDeadline(context.Background(), &schema.WorkflowStep{Name: "s", Timeout: "20ms"}, nil, func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		})
		require.ErrorIs(t, err, errUtils.ErrStepTimeout)
	})

	t.Run("passes the context through without a timeout", func(t *testing.T) {
		parent := context.WithValue(context.Background(), outputSuppressedContextKey{}, struct{}{})
		err := RunWithStepDeadline(parent, &schema.WorkflowStep{Name: "s"}, nil, func(ctx context.Context) error {
			assert.True(t, OutputSuppressed(ctx))
			return nil
		})
		require.NoError(t, err)
	})

	t.Run("an invalid timeout fails before fn runs", func(t *testing.T) {
		called := false
		err := RunWithStepDeadline(context.Background(), &schema.WorkflowStep{Name: "s", Timeout: "later"}, nil, func(context.Context) error {
			called = true
			return nil
		})
		require.ErrorIs(t, err, errUtils.ErrStepTimeoutInvalid)
		assert.False(t, called)
	})
}

// boundedRetryConfig returns a retry policy with a backoff far longer than any test timeout.
func boundedRetryConfig() *schema.RetryConfig {
	attempts := 5
	delay := time.Hour
	return &schema.RetryConfig{MaxAttempts: &attempts, InitialDelay: &delay, BackoffStrategy: schema.BackoffConstant}
}

func TestRunWithStepRetryTimeoutBoundsTheWholeStep(t *testing.T) {
	cause := errors.New("attempt failed")

	t.Run("a timeout expiring during backoff ends the step without another attempt", func(t *testing.T) {
		attempts := 0
		start := time.Now()
		step := &schema.WorkflowStep{Name: "flaky", Timeout: "150ms", Retry: boundedRetryConfig()}

		err := RunWithStepRetry(context.Background(), step, nil, func(context.Context) error {
			attempts++
			return cause
		})

		require.ErrorIs(t, err, errUtils.ErrStepTimeout)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Equal(t, 1, attempts, "no attempt may run after the deadline")
		assert.Less(t, time.Since(start), stepTimeoutBudget, "backoff must not outlive the step timeout")
	})

	t.Run("an attempt cut short by the deadline is not retried", func(t *testing.T) {
		attempts := 0
		step := &schema.WorkflowStep{Name: "slow", Timeout: "100ms", Retry: boundedRetryConfig()}

		err := RunWithStepRetry(context.Background(), step, nil, func(ctx context.Context) error {
			attempts++
			<-ctx.Done()
			return ctx.Err()
		})

		require.ErrorIs(t, err, errUtils.ErrStepTimeout)
		assert.Equal(t, 1, attempts)
	})

	t.Run("one deadline is shared by every attempt", func(t *testing.T) {
		attempts := 0
		var deadlines []time.Time
		attemptsAllowed, delay := 3, time.Millisecond
		step := &schema.WorkflowStep{Name: "shared", Timeout: "1m", Retry: &schema.RetryConfig{
			MaxAttempts: &attemptsAllowed, InitialDelay: &delay, BackoffStrategy: schema.BackoffConstant,
		}}

		err := RunWithStepRetry(context.Background(), step, nil, func(ctx context.Context) error {
			attempts++
			limit, ok := ctx.Deadline()
			require.True(t, ok)
			deadlines = append(deadlines, limit)
			return cause
		})

		require.Error(t, err)
		assert.NotErrorIs(t, err, errUtils.ErrStepTimeout)
		require.ErrorIs(t, err, cause)
		require.Equal(t, 3, attempts)
		assert.Equal(t, deadlines[0], deadlines[len(deadlines)-1], "later attempts must not get a fresh deadline")
	})

	t.Run("a step without a timeout still retries until it succeeds", func(t *testing.T) {
		attempts := 0
		attemptsAllowed, delay := 5, time.Millisecond
		step := &schema.WorkflowStep{Name: "open", Retry: &schema.RetryConfig{
			MaxAttempts: &attemptsAllowed, InitialDelay: &delay, BackoffStrategy: schema.BackoffConstant,
		}}

		err := RunWithStepRetry(context.Background(), step, nil, func(context.Context) error {
			attempts++
			if attempts < 3 {
				return cause
			}
			return nil
		})

		require.NoError(t, err)
		assert.Equal(t, 3, attempts)
	})

	t.Run("an invalid timeout fails before any attempt runs", func(t *testing.T) {
		attempts := 0
		step := &schema.WorkflowStep{Name: "bad", Timeout: "later", Retry: boundedRetryConfig()}

		err := RunWithStepRetry(context.Background(), step, nil, func(context.Context) error {
			attempts++
			return nil
		})

		require.ErrorIs(t, err, errUtils.ErrStepTimeoutInvalid)
		assert.Zero(t, attempts)
	})
}

func TestStepTimeoutBoundsRetries(t *testing.T) {
	tests := []struct {
		stepType string
		want     bool
	}{
		{schema.TaskTypeShell, true},
		{schema.TaskTypeScript, true},
		{schema.TaskTypeAtmos, true},
		{tflintStepType, true},
		{" shell ", true},
		{"sleep", false},
		{"http", false},
		{"spin", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.stepType, func(t *testing.T) {
			assert.Equal(t, tt.want, StepTimeoutBoundsRetries(tt.stepType))
		})
	}
}

// sleepingChildEnv makes the test binary, started as a child, sleep stepTimeoutSleepMS.
func sleepingChildEnv(t *testing.T) map[string]string {
	t.Helper()

	exe, err := os.Executable()
	require.NoError(t, err)
	return map[string]string{
		"FAKE_EXE":              exe,
		"_ATMOS_STEP_FAKE":      "sleep",
		atmosStepFakeSleepMSEnv: strconv.Itoa(stepTimeoutSleepMS),
	}
}

func TestScriptHandlerEnforcesTimeout(t *testing.T) {
	initShellTestIO(t)
	handler, ok := Get(schema.TaskTypeScript)
	require.True(t, ok)

	step := &schema.WorkflowStep{
		Name: "slow", Type: schema.TaskTypeScript, Interpreter: "starlark", Output: "none",
		Timeout: "300ms",
		Env:     sleepingChildEnv(t),
		Script:  "exec.run([env[\"FAKE_EXE\"]])\nprint(\"finished\")\n",
	}

	start := time.Now()
	result, err := handler.Execute(context.Background(), step, NewVariables())

	require.ErrorIs(t, err, errUtils.ErrStepTimeout)
	assert.Less(t, time.Since(start), stepTimeoutBudget, "the timeout must cancel the interpreter and its subprocess")
	require.NotNil(t, result)
	assert.NotContains(t, result.Value, "finished")
}

func TestScriptHandlerTimeoutNotReachedStillSucceeds(t *testing.T) {
	initShellTestIO(t)
	handler, ok := Get(schema.TaskTypeScript)
	require.True(t, ok)

	step := &schema.WorkflowStep{
		Name: "quick", Type: schema.TaskTypeScript, Interpreter: "starlark", Output: "none",
		Timeout: "1m",
		Script:  "output = \"done\"\n",
	}

	result, err := handler.Execute(context.Background(), step, NewVariables())

	require.NoError(t, err)
	assert.Equal(t, "done", result.Value)
}

func TestScriptHandlerRejectsInvalidTimeout(t *testing.T) {
	initShellTestIO(t)
	handler, ok := Get(schema.TaskTypeScript)
	require.True(t, ok)

	_, err := handler.Execute(context.Background(), &schema.WorkflowStep{
		Name: "bad", Type: schema.TaskTypeScript, Interpreter: "starlark", Output: "none",
		Timeout: "eventually", Script: "output = \"x\"\n",
	}, NewVariables())

	require.ErrorIs(t, err, errUtils.ErrStepTimeoutInvalid)
}

func TestShellHandlerEnforcesTimeout(t *testing.T) {
	initShellTestIO(t)
	handler, ok := Get(schema.TaskTypeShell)
	require.True(t, ok)

	step := &schema.WorkflowStep{
		Name: "slow", Type: schema.TaskTypeShell, Output: "none",
		Timeout: "300ms",
		Env:     sleepingChildEnv(t),
		Command: `"$FAKE_EXE"`,
	}

	start := time.Now()
	_, err := handler.Execute(context.Background(), step, NewVariables())

	require.ErrorIs(t, err, errUtils.ErrStepTimeout)
	assert.Less(t, time.Since(start), stepTimeoutBudget)
}

func TestAtmosHandlerEnforcesTimeout(t *testing.T) {
	initShellTestIO(t)
	handler, ok := Get(schema.TaskTypeAtmos)
	require.True(t, ok)

	env := sleepingChildEnv(t)
	delete(env, "FAKE_EXE")
	step := &schema.WorkflowStep{
		Name: "slow", Type: schema.TaskTypeAtmos, Output: "none",
		Timeout: "300ms",
		Env:     env,
		Command: "version",
	}

	start := time.Now()
	_, err := handler.Execute(context.Background(), step, NewVariables())

	require.ErrorIs(t, err, errUtils.ErrStepTimeout)
	assert.Less(t, time.Since(start), stepTimeoutBudget)
}
