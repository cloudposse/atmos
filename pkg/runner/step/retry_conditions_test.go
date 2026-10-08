package step

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/automation"
	"github.com/cloudposse/atmos/pkg/schema"
)

func retryConfig(conditions ...string) *schema.RetryConfig {
	attempts := 3
	return &schema.RetryConfig{MaxAttempts: &attempts, Conditions: conditions}
}

func TestRetryPredicate(t *testing.T) {
	failure := errors.New("exit status 1")
	withOutput := func(stdout, stderr string) error {
		return &StepOutputError{Err: failure, Stdout: stdout, Stderr: stderr}
	}
	for _, tc := range []struct {
		name       string
		conditions []string
		err        error
		want       bool
	}{
		{"no conditions retries any error", nil, failure, true},
		{"no conditions never retries cancellation", nil, context.Canceled, false},
		{"no conditions never retries a deadline", nil, context.DeadlineExceeded, false},
		{"no conditions never retries a workflow exit", nil, errUtils.ErrWorkflowExit, false},
		{"no conditions never retries a user abort", nil, errUtils.ErrUserAborted, false},
		{"a condition matches the error text", []string{"status 1"}, failure, true},
		{"a condition matches stderr", []string{"Bad Gateway"}, withOutput("", "502 Bad Gateway"), true},
		{"a condition matches stdout", []string{"try again"}, withOutput("please try again", ""), true},
		{"slash delimiters are stripped", []string{"/Bad Gateway/"}, withOutput("", "502 Bad Gateway"), true},
		{"any of several conditions", []string{"nope", "gateway"}, withOutput("", "bad gateway"), true},
		{"a condition that matches nothing stops the retries", []string{"never-matches"}, withOutput("out", "err"), false},
		{"a condition that matches nothing stops the retries for a plain error", []string{"never-matches"}, failure, false},
		{"conditions do not override cancellation", []string{"cancel"}, context.Canceled, false},
		{"output on a wrapped error is found", []string{"flaky"}, errors.Join(errors.New("outer"), withOutput("flaky", "")), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			predicate, err := RetryPredicate(retryConfig(tc.conditions...))
			require.NoError(t, err)
			assert.Equal(t, tc.want, predicate(tc.err))
		})
	}

	t.Run("a nil config retries what the default retries", func(t *testing.T) {
		predicate, err := RetryPredicate(nil)
		require.NoError(t, err)
		assert.True(t, predicate(failure))
	})

	t.Run("an invalid pattern is a configuration error", func(t *testing.T) {
		_, err := RetryPredicate(retryConfig("(unclosed"))
		require.ErrorIs(t, err, errUtils.ErrInvalidConfig)
	})
}

func TestWithStepOutput(t *testing.T) {
	failure := errors.New("exit status 4")

	t.Run("output is attached and the message is unchanged", func(t *testing.T) {
		result := NewStepResult("").WithMetadata("stdout", "out text").WithMetadata("stderr", "err text")
		wrapped := WithStepOutput(failure, result)
		var carried *StepOutputError
		require.ErrorAs(t, wrapped, &carried)
		assert.Equal(t, "out text", carried.Stdout)
		assert.Equal(t, "err text", carried.Stderr)
		assert.Equal(t, failure.Error(), wrapped.Error())
		require.ErrorIs(t, wrapped, failure)
	})

	t.Run("a result without metadata falls back to its value and error", func(t *testing.T) {
		wrapped := WithStepOutput(failure, &StepResult{Value: "value text", Error: "error text"})
		var carried *StepOutputError
		require.ErrorAs(t, wrapped, &carried)
		assert.Equal(t, "value text", carried.Stdout)
		assert.Equal(t, "error text", carried.Stderr)
	})

	t.Run("nothing to attach leaves the error alone", func(t *testing.T) {
		assert.Same(t, failure, WithStepOutput(failure, nil))
		assert.Same(t, failure, WithStepOutput(failure, &StepResult{}))
		assert.NoError(t, WithStepOutput(nil, NewStepResult("x")))
	})

	t.Run("output is attached once", func(t *testing.T) {
		first := WithStepOutput(failure, NewStepResult("first"))
		assert.Same(t, first, WithStepOutput(first, NewStepResult("second")))
	})
}

// shellRetryCall runs a shell command that appends one line to counter on every attempt, prints
// the given text on the given stream, and exits with the given status.
func shellRetryCall(t *testing.T, counter, stream, text string, status int, retry map[string]any) *automation.StepCall {
	t.Helper()
	redirect := ""
	if stream == "stderr" {
		redirect = " >&2"
	}
	command := `echo attempt >> "$WP5_COUNTER"; echo "` + text + `"` + redirect + `; exit ` + strconv.Itoa(status)
	return &automation.StepCall{Type: "shell", WorkingDirectory: filepath.Dir(counter), Configuration: map[string]any{
		"command": command, "output": "none", "retry": retry,
		"env": map[string]string{"WP5_COUNTER": counter},
	}}
}

func attemptCount(t *testing.T, counter string) int {
	t.Helper()
	data, err := os.ReadFile(counter)
	require.NoError(t, err)
	return strings.Count(string(data), "attempt")
}

func stepRetryMap(attempts int, conditions ...string) map[string]any {
	retry := map[string]any{"max_attempts": attempts, "initial_delay": "1ms", "backoff_strategy": "constant"}
	if len(conditions) > 0 {
		retry["conditions"] = conditions
	}
	return retry
}

// retry.conditions applies to every step type, not only http.
func TestDirectStepCallRetryConditions(t *testing.T) {
	initShellTestIO(t)

	t.Run("a condition that never matches does not retry", func(t *testing.T) {
		counter := filepath.Join(t.TempDir(), "count")
		_, err := NewAutomationLibrary(NewVariables(), nil).Run(t.Context(), shellRetryCall(t, counter, "stderr", "flaky", 1, stepRetryMap(3, "never-matches")))
		require.Error(t, err)
		assert.Equal(t, 1, attemptCount(t, counter))
		assert.NotContains(t, err.Error(), "max attempts", "a failure no condition matches ends the loop without a retry-exhaustion error")
	})

	t.Run("a condition that matches stderr retries to the attempt limit", func(t *testing.T) {
		counter := filepath.Join(t.TempDir(), "count")
		_, err := NewAutomationLibrary(NewVariables(), nil).Run(t.Context(), shellRetryCall(t, counter, "stderr", "flaky failure", 1, stepRetryMap(3, "flaky")))
		require.Error(t, err)
		assert.Equal(t, 3, attemptCount(t, counter))
		assert.Contains(t, err.Error(), "max attempts (3)")
	})

	t.Run("a condition that matches stdout retries", func(t *testing.T) {
		counter := filepath.Join(t.TempDir(), "count")
		_, err := NewAutomationLibrary(NewVariables(), nil).Run(t.Context(), shellRetryCall(t, counter, "stdout", "try again later", 1, stepRetryMap(2, "/try again/")))
		require.Error(t, err)
		assert.Equal(t, 2, attemptCount(t, counter))
	})

	t.Run("a condition that matches the error text retries", func(t *testing.T) {
		counter := filepath.Join(t.TempDir(), "count")
		_, err := NewAutomationLibrary(NewVariables(), nil).Run(t.Context(), shellRetryCall(t, counter, "stdout", "x", 7, stepRetryMap(2, "code 7")))
		require.Error(t, err)
		assert.Equal(t, 2, attemptCount(t, counter))
	})

	t.Run("without conditions every failure retries", func(t *testing.T) {
		counter := filepath.Join(t.TempDir(), "count")
		_, err := NewAutomationLibrary(NewVariables(), nil).Run(t.Context(), shellRetryCall(t, counter, "stderr", "anything", 1, stepRetryMap(3)))
		require.Error(t, err)
		assert.Equal(t, 3, attemptCount(t, counter))
	})

	t.Run("an invalid pattern fails before the step runs", func(t *testing.T) {
		counter := filepath.Join(t.TempDir(), "count")
		_, err := NewAutomationLibrary(NewVariables(), nil).Run(t.Context(), shellRetryCall(t, counter, "stderr", "x", 1, stepRetryMap(3, "(unclosed")))
		require.ErrorIs(t, err, errUtils.ErrInvalidConfig)
		assert.NoFileExists(t, counter)
	})
}

// RunWithStepRetry serves the hook path, where only the step's error is visible to the loop.
func TestRunWithStepRetryHonorsConditions(t *testing.T) {
	for _, tc := range []struct {
		name         string
		conditions   []string
		wantAttempts int
	}{
		{"matching condition retries", []string{"transient"}, 3},
		{"other condition stops at once", []string{"never-matches"}, 1},
		{"no condition retries", nil, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			attempts := 0
			step := &schema.WorkflowStep{Name: "flaky", Type: "shell", Retry: retryConfig(tc.conditions...)}
			err := RunWithStepRetry(t.Context(), step, nil, func(context.Context) error {
				attempts++
				return WithStepOutput(errors.New("exit status 1"), NewStepResult("").WithMetadata("stderr", "transient failure"))
			})
			require.Error(t, err)
			assert.Equal(t, tc.wantAttempts, attempts)
		})
	}

	t.Run("an invalid pattern is reported before the first attempt", func(t *testing.T) {
		attempts := 0
		step := &schema.WorkflowStep{Name: "bad", Type: "shell", Retry: retryConfig("(unclosed")}
		err := RunWithStepRetry(t.Context(), step, nil, func(context.Context) error { attempts++; return nil })
		require.ErrorIs(t, err, errUtils.ErrInvalidConfig)
		assert.Zero(t, attempts)
	})

	t.Run("no retry config runs once", func(t *testing.T) {
		attempts := 0
		err := RunWithStepRetry(t.Context(), &schema.WorkflowStep{Name: "once"}, nil, func(context.Context) error { attempts++; return errors.New("boom") })
		require.Error(t, err)
		assert.Equal(t, 1, attempts)
	})
}
