package recovery

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestRecoveryConstruction(t *testing.T) {
	runner, err := New(&schema.ConfigAndStacksInfo{DryRun: true}, "invalid", "*")
	require.NoError(t, err)
	require.Nil(t, runner)
	for _, command := range []string{"*", ""} {
		runner, err = New(nil, "custom", command)
		require.ErrorIs(t, err, errUtils.ErrPolicyInputInvalid)
		require.Nil(t, runner)
	}
	retryRunner(t, "true", "")
	runner, err = New(nil, "custom", "run")
	require.NoError(t, err)
	require.NotNil(t, runner)
	runner, err = New(nil, "custom", "unselected")
	require.NoError(t, err)
	require.Nil(t, runner)
	runner, err = New(&schema.ConfigAndStacksInfo{ComponentSection: map[string]any{"metadata": "invalid"}}, "custom", "run")
	require.Error(t, err)
	require.Nil(t, runner)
}

func TestRecoveryRejectsInvalidFallbackBeforeCalling(t *testing.T) {
	attempts := 0
	maxAttempts := 0
	var runner *Runner
	err := runner.Run(t.Context(), "call", &schema.RetryConfig{MaxAttempts: &maxAttempts, Conditions: []string{"transient"}}, func() Result {
		attempts++
		return Result{}
	})
	require.ErrorIs(t, err, errUtils.ErrInvalidConfig)
	require.Zero(t, attempts)
}

func TestRecoveryFallbackStopsWhenErrorChanges(t *testing.T) {
	var runner *Runner
	maxAttempts := 3
	for _, match := range []bool{false, true} {
		calls := 0
		terminal := errors.New("permanent")
		err := runner.Run(t.Context(), "call", &schema.RetryConfig{MaxAttempts: &maxAttempts, Conditions: []string{"transient"}}, func() Result {
			calls++
			if match && calls == 1 {
				return Result{Err: errors.New("network"), Output: "transient"}
			}
			return Result{Err: terminal, Output: "permanent"}
		})
		require.ErrorIs(t, err, terminal)
		expected := 1
		if match {
			expected = 2
		}
		require.Equal(t, expected, calls)
	}
	result, err := Call(t.Context(), nil, "direct", func() (string, error) { return "response", nil })
	require.NoError(t, err)
	require.Equal(t, "response", result)
}

func TestRecoveryElapsedBudgetIncludesFirstAttempt(t *testing.T) {
	maxAttempts, limit := 3, time.Second
	config := &schema.RetryConfig{MaxAttempts: &maxAttempts, MaxElapsedTime: &limit, Conditions: []string{"transient"}}
	failure := Result{Err: errors.New("transient"), Output: "transient"}
	budget, _, err := prepareBudget(nil, config, failure, time.Second/4)
	require.NoError(t, err)
	require.Equal(t, 3*time.Second/4, *budget.MaxElapsedTime)
	require.Equal(t, time.Second, *config.MaxElapsedTime, "one invocation must not consume another invocation's budget")
	budget, _, err = prepareBudget(nil, config, failure, time.Second)
	require.NoError(t, err)
	require.Nil(t, budget)
}

func TestRecoveryLateDenialStopsRetries(t *testing.T) {
	runner := retryRunner(t, "true", "    block:\n      on: custom.run.error\n      action: deny\n      rule: input.attempt == 2\n")
	calls := 0
	err := runner.Run(t.Context(), "call", nil, func() Result { calls++; return Result{Err: errors.New("transient")} })
	require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
	require.Equal(t, 2, calls)
}

func TestRecoveryLateCancellationStopsRetries(t *testing.T) {
	runner := retryRunner(t, "true", "")
	calls := 0
	err := runner.Run(t.Context(), "call", nil, func() Result {
		calls++
		if calls == 2 {
			return Result{Err: context.Canceled}
		}
		return Result{Err: errors.New("transient")}
	})
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 2, calls)
}

func TestRecoveryTransportForwardsWithoutPolicy(t *testing.T) {
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://cluster.invalid", nil)
	require.NoError(t, err)
	expected := &http.Response{StatusCode: http.StatusServiceUnavailable, Body: http.NoBody}
	transport := &Transport{Base: roundTripFunc(func(actual *http.Request) (*http.Response, error) {
		require.Same(t, request, actual)
		return expected, nil
	})}
	response, err := transport.RoundTrip(request)
	require.NoError(t, err)
	require.Same(t, expected, response)
	require.NoError(t, response.Body.Close())
}

func TestRecoveryTransportRetriesNetworkFailure(t *testing.T) {
	runner := retryRunner(t, "true", "")
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://cluster.invalid", nil)
	require.NoError(t, err)
	calls := 0
	transport := &Transport{Runner: runner, Base: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("connection reset")
		}
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
	})}
	response, err := transport.RoundTrip(request)
	require.NoError(t, err)
	require.Equal(t, 2, calls)
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.NoError(t, response.Body.Close())
}

func TestRecoveryTransportBodyReplayFailure(t *testing.T) {
	runner := retryRunner(t, "true", "")
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://cluster.invalid", strings.NewReader("request"))
	require.NoError(t, err)
	failure := errors.New("cannot reopen body")
	request.GetBody = func() (io.ReadCloser, error) { return nil, failure }
	body := &trackedBody{Reader: strings.NewReader("unavailable")}
	calls := 0
	transport := &Transport{Runner: runner, Base: roundTripFunc(func(actual *http.Request) (*http.Response, error) {
		calls++
		require.NoError(t, actual.Body.Close())
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: body}, nil
	})}
	response, err := transport.RoundTrip(request)
	if response != nil {
		require.NoError(t, response.Body.Close())
	}
	require.ErrorIs(t, err, failure)
	require.Nil(t, response)
	require.Equal(t, 1, calls, "an unreplayable request must never reach the transport again")
	require.True(t, body.closed)
}
