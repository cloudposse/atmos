package execbatch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// TestBatchConcurrencyAndPlacement checks bounded overlap and isolation of the copied document.
func TestBatchConcurrencyAndPlacement(t *testing.T) {
	t.Parallel()
	for _, limit := range []int{1, 2, 4} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			started := make(chan string, 4)
			release := make(chan struct{})
			var active, peak atomic.Int32
			execute := func(ctx context.Context, command, _ string) (any, error) {
				n := active.Add(1)
				defer active.Add(-1)
				for old := peak.Load(); n > old && !peak.CompareAndSwap(old, n); old = peak.Load() {
				}
				started <- command
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-release:
					return command, nil
				}
			}
			input := map[string]any{"a": "!exec first", "nested": map[string]any{"list": []any{"!exec second", "!exec third", "!exec fourth"}}, "literal": "unchanged"}
			type outcome struct {
				data map[string]any
				err  error
			}
			done := make(chan outcome, 1)
			go func() { data, err := Evaluate(ctx, input, limit, execute); done <- outcome{data, err} }()
			for i := 0; i < limit; i++ {
				select {
				case <-started:
				case <-ctx.Done():
					t.Fatal("workers did not overlap")
				}
			}
			assert.Equal(t, int32(limit), active.Load())
			close(release)
			got := <-done
			require.NoError(t, got.err)
			assert.LessOrEqual(t, peak.Load(), int32(limit))
			assert.Equal(t, "first", got.data["a"].(*Value).Data)
			list := got.data["nested"].(map[string]any)["list"].([]any)
			assert.Equal(t, []any{&Value{Data: "second"}, &Value{Data: "third"}, &Value{Data: "fourth"}}, list)
			assert.Equal(t, "unchanged", got.data["literal"])
			assert.Equal(t, "!exec first", input["a"])
			got.data["nested"].(map[string]any)["other"] = "output-only"
			assert.NotContains(t, input["nested"], "other")
			input["nested"].(map[string]any)["source"] = "input-only"
			assert.NotContains(t, got.data["nested"], "source")
		})
	}
}

// TestBatchCancellationAndFailure verifies failures and cancellation prevent queued execution.
func TestBatchCancellationAndFailure(t *testing.T) {
	t.Parallel()
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			started := make(chan struct{})
			var calls atomic.Int32
			execute := Executor(func(ctx context.Context, _, _ string) (any, error) {
				calls.Add(1)
				close(started)
				if fail {
					return nil, errUtils.ErrProcessStartFailed
				}
				<-ctx.Done()
				return nil, ctx.Err()
			})
			done := make(chan error, 1)
			go func() {
				_, err := Evaluate(ctx, map[string]any{"a": "!exec first", "b": "!exec second"}, 1, execute)
				done <- err
			}()
			<-started
			if !fail {
				cancel()
			}
			select {
			case err := <-done:
				if fail {
					require.ErrorIs(t, err, errUtils.ErrProcessStartFailed)
					assert.Contains(t, err.Error(), `$["a"]`)
				} else {
					require.ErrorIs(t, err, context.Canceled)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("cancellation did not stop the batch")
			}
			assert.Equal(t, int32(1), calls.Load())
		})
	}
}

// TestBatchValidationAndLiteralValues checks invalid commands and preserves non-exec values.
func TestBatchValidationAndLiteralValues(t *testing.T) {
	t.Parallel()
	execute := func(context.Context, string, string) (any, error) { t.Fatal("unexpected execution"); return nil, nil }
	_, err := Evaluate(t.Context(), nil, 0, execute)
	require.ErrorIs(t, err, errUtils.ErrInvalidConfig)
	_, err = Evaluate(t.Context(), map[string]any{"bad": "!exec"}, 2, execute)
	require.ErrorIs(t, err, errUtils.ErrInvalidConfig)
	input := map[string]any{"string": "!execute not-an-exec", "empty": []any{}, "value": 42}
	got, err := Evaluate(t.Context(), input, 2, execute)
	require.NoError(t, err)
	assert.Equal(t, input, got)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = Evaluate(ctx, input, 2, execute)
	require.True(t, errors.Is(err, context.Canceled))
}

// TestShellOutputDecoding checks shell result decoding and preserves process exit errors.
func TestShellOutputDecoding(t *testing.T) {
	// printf is an interpreter builtin, so these tests need no platform shell binary.
	execute, err := ShellExecutor()
	require.NoError(t, err)
	for _, tc := range []struct {
		command string
		want    any
	}{
		{`printf 'hello\n\n'`, "hello"},
		{`printf '{"n":2}\n'`, map[string]any{"n": float64(2)}},
		{`printf '[1,2]'`, []any{float64(1), float64(2)}},
		{`printf '!exec must-not-run'`, "!exec must-not-run"},
		{`printf ''`, ""},
	} {
		got, err := execute(t.Context(), tc.command, "test")
		require.NoError(t, err)
		assert.Equal(t, tc.want, got)
	}
	_, err = execute(t.Context(), "exit 7", "bad")
	require.Error(t, err)
	var exitErr errUtils.ExitCodeError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 7, exitErr.Code)
}

// TestBatchPolicyBeforeCommands verifies denied commands never run or leak their arguments.
func TestBatchPolicyBeforeCommands(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ATMOS_POLICY_DIR", dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "deny.yaml"), []byte("apiVersion: atmos/v1\nkind: Policy\nmetadata: {name: safety}\nspec:\n  policies:\n    exec:\n      on: function\n      action: deny\n      rule: input.name == 'exec'\n"), 0o600))
	_, err := Evaluate(t.Context(), map[string]any{"value": "!exec secret-argument"}, 4,
		func(context.Context, string, string) (any, error) {
			t.Fatal("denied command must not run")
			return nil, nil
		})
	require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
	assert.NotContains(t, err.Error(), "secret-argument")
}
