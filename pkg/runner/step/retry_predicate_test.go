package step

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
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

func TestRetryWithConditions(t *testing.T) {
	failure := errors.New("transient failure")
	for _, tc := range []struct {
		name         string
		config       *schema.RetryConfig
		failure      error
		wantError    error
		wantAttempts int
	}{
		{"no configuration runs once", nil, failure, failure, 1},
		{"invalid condition never runs", retryConfig("(unclosed"), failure, errUtils.ErrInvalidConfig, 0},
		{"matching failure retries until success", retryConfig("transient"), failure, nil, 2},
		{"nonmatching failure stops", retryConfig("unrelated"), failure, failure, 1},
		{"cancellation stops despite a matching condition", retryConfig("canceled"), context.Canceled, context.Canceled, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			attempts := 0
			err := RetryWithConditions(t.Context(), tc.config, func() error {
				attempts++
				if attempts == 1 {
					return tc.failure
				}
				return nil
			})
			if tc.wantError == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tc.wantError)
			}
			assert.Equal(t, tc.wantAttempts, attempts)
		})
	}
}
