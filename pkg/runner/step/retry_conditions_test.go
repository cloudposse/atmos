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
