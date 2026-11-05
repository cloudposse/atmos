package recovery

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/policy/admission"
	"github.com/cloudposse/atmos/pkg/schema"
)

func retryRunner(t *testing.T, rule, extra string) *Runner {
	t.Helper()
	dir := t.TempDir()
	text := "apiVersion: atmos/v1\nkind: Policy\nmetadata: {name: recovery}\nspec:\n  policies:\n    transient:\n      on: custom.run.error\n      action: retry\n      rule: " + rule + "\n      with: {max_attempts: 3}\n" + extra
	require.NoError(t, os.WriteFile(filepath.Join(dir, "retry.yaml"), []byte(text), 0o600))
	t.Setenv("ATMOS_POLICY_DIR", dir)
	finish, err := admission.BeginInvocation()
	require.NoError(t, err)
	t.Cleanup(finish)
	runner, err := New(&schema.ConfigAndStacksInfo{ComponentFromArg: "app", Stack: "dev"}, "custom", "run")
	require.NoError(t, err)
	require.NotNil(t, runner)
	return runner
}

func TestRecoveryAttempts(t *testing.T) {
	failure := errors.New("transient")
	for _, tc := range []struct {
		name, rule         string
		succeeds, attempts int
	}{
		{"event recovery", "input.error.message == 'transient'", 2, 2},
		{"budget exhausted", "input.error.message == 'transient'", 0, 3},
		{"unmatched error", "input.output.contains('other')", 0, 1},
		{"success", "true", 1, 1},
		{"attempt facts", "input.attempt < 2 && input.stage == 'poll'", 0, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner := retryRunner(t, tc.rule, "")
			attempts := 0
			err := runner.Run(context.Background(), "poll", nil, func() Result {
				attempts++
				if attempts == tc.succeeds {
					return Result{}
				}
				return Result{Err: failure}
			})
			assert.Equal(t, tc.attempts, attempts)
			if tc.succeeds > 0 {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, failure)
			}
		})
	}
}

func TestRecoveryDenialAndEvaluationFailure(t *testing.T) {
	for _, tc := range []struct {
		name, rule, extra string
		want              error
	}{
		{"deny", "true", "    block:\n      on: custom.run.error\n      action: deny\n      rule: 'true'\n", errUtils.ErrPolicyDenied},
		{"evaluation", "input.missing.value", "", errUtils.ErrPolicyEvaluation},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner := retryRunner(t, tc.rule, tc.extra)
			attempts := 0
			err := runner.Run(context.Background(), "call", nil, func() Result { attempts++; return Result{Err: errors.New("failure")} })
			require.ErrorIs(t, err, tc.want)
			assert.Equal(t, 1, attempts)
		})
	}
}

func TestRecoveryNeverRetriesCancellationOrPolicyFailure(t *testing.T) {
	runner := retryRunner(t, "true", "")
	for _, failure := range []error{context.Canceled, context.DeadlineExceeded, errUtils.ErrPolicyDenied, errUtils.ErrPolicyRequirements} {
		attempts := 0
		err := runner.Run(context.Background(), "call", nil, func() Result { attempts++; return Result{Err: fmt.Errorf("wrapped: %w", failure)} })
		require.ErrorIs(t, err, failure)
		assert.Equal(t, 1, attempts)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := runner.Run(ctx, "call", nil, func() Result { t.Fatal("canceled operation executed"); return Result{} })
	require.ErrorIs(t, err, context.Canceled)
}

func TestRecoveryUsesOneBudgetAndLegacyFallback(t *testing.T) {
	for _, matched := range []bool{false, true} {
		t.Run(fmt.Sprint(matched), func(t *testing.T) {
			runner := retryRunner(t, fmt.Sprint(matched), "")
			maxAttempts := 2
			fallback := &schema.RetryConfig{MaxAttempts: &maxAttempts, Conditions: []string{"transient"}}
			attempts := 0
			err := runner.Run(context.Background(), "call", fallback, func() Result { attempts++; return Result{Err: errors.New("failed"), Output: "transient"} })
			require.Error(t, err)
			want := 2
			if matched {
				want = 3
			}
			assert.Equal(t, want, attempts)
		})
	}
}

func TestRecoveryInvalidLegacyConfigFailsBeforeExecution(t *testing.T) {
	runner := retryRunner(t, "true", "")
	err := runner.Run(context.Background(), "call", &schema.RetryConfig{Conditions: []string{"["}}, func() Result {
		t.Fatal("invalid retry configuration must fail before execution")
		return Result{}
	})
	require.ErrorIs(t, err, errUtils.ErrInvalidConfig)
}

func TestRecoveryConcurrentIndependentCalls(t *testing.T) {
	runner := retryRunner(t, "true", "")
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			attempts := 0
			value, err := Call(context.Background(), runner, "poll", func() (int, error) {
				attempts++
				if attempts < 3 {
					return 0, errors.New("transient")
				}
				return 42, nil
			})
			assert.NoError(t, err)
			assert.Equal(t, 42, value)
			assert.Equal(t, 3, attempts)
		})
	}
	wg.Wait()
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type trackedBody struct {
	io.Reader
	closed bool
}

func (b *trackedBody) Close() error { b.closed = true; return nil }

func TestRecoveryTransportCancellationAndStreaming(t *testing.T) {
	runner := retryRunner(t, "true", "")
	t.Run("canceled before execution closes body", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://cluster.invalid", strings.NewReader("body"))
		require.NoError(t, err)
		body := &trackedBody{Reader: request.Body}
		request.Body = body
		transport := &Transport{Runner: runner, Base: roundTripFunc(func(*http.Request) (*http.Response, error) {
			t.Fatal("canceled request executed")
			return nil, nil
		})}
		response, err := transport.RoundTrip(request) //nolint:bodyclose // Failure must return a nil response.
		require.ErrorIs(t, err, context.Canceled)
		require.Nil(t, response)
		require.True(t, body.closed)
	})
	t.Run("success remains a response if cancellation races completion", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://cluster.invalid", nil)
		require.NoError(t, err)
		transport := &Transport{Runner: runner, Base: roundTripFunc(func(*http.Request) (*http.Response, error) {
			cancel()
			return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
		})}
		response, err := transport.RoundTrip(request)
		require.NoError(t, err)
		require.NotNil(t, response)
		require.NoError(t, response.Body.Close())
	})
	t.Run("streaming bodies execute once", func(t *testing.T) {
		request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://cluster.invalid", io.NopCloser(strings.NewReader("stream")))
		require.NoError(t, err)
		calls := 0
		transport := &Transport{Runner: runner, Base: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			calls++
			require.NoError(t, request.Body.Close())
			return nil, errors.New("transient")
		})}
		response, err := transport.RoundTrip(request) //nolint:bodyclose // Failure must return a nil response.
		require.Error(t, err)
		require.Nil(t, response)
		require.Equal(t, 1, calls)
	})
}

func TestRecoveryMatchesComponentMetadata(t *testing.T) {
	retryRunner(t, "true", "      match: {tags: [protected]}\n")
	for _, tagged := range []bool{true, false} {
		info := &schema.ConfigAndStacksInfo{ComponentFromArg: "app", StackFromArg: "dev"}
		if tagged {
			info.ComponentSection = map[string]any{"metadata": map[string]any{"tags": []any{"protected"}}}
		}
		runner, err := New(info, "custom", "run")
		require.NoError(t, err)
		attempts := 0
		err = Do(context.Background(), runner, "call", func() error { attempts++; return errors.New("transient") })
		require.Error(t, err)
		want := 1
		if tagged {
			want = 3
		}
		require.Equal(t, want, attempts)
	}
}

func TestRecoveryTransportPreservesFinalResponse(t *testing.T) {
	runner := retryRunner(t, "input.error.code == '503'", "")
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		attempts++
		body, err := io.ReadAll(request.Body)
		assert.NoError(t, err)
		assert.Equal(t, "request", string(body))
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, "temporarily unavailable")
	}))
	t.Cleanup(server.Close)
	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL, strings.NewReader("request"))
	require.NoError(t, err)
	response, err := (&Transport{Runner: runner}).RoundTrip(request)
	require.NoError(t, err)
	t.Cleanup(func() { _ = response.Body.Close() })
	assert.Equal(t, 3, attempts)
	assert.Equal(t, http.StatusServiceUnavailable, response.StatusCode)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	assert.Equal(t, "temporarily unavailable", string(body))
}
