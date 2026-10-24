package starlark

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/script"
)

func TestProcessCheckFalse(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		result  process.Result
		wantErr bool
	}{
		{"success", process.Result{Started: true}, false},
		{"normal exit", process.Result{Started: true, ExitCode: 7, Err: errUtils.ErrProcessWaitFailed}, false},
		{"exit without cause", process.Result{Started: true, ExitCode: 7}, false},
		{"launch failure", process.Result{ExitCode: -1, Err: errUtils.ErrProcessStartFailed}, true},
		{"canceled", process.Result{Started: true, ExitCode: 1, Canceled: true}, true},
		{"signal", process.Result{Started: true, ExitCode: 137, Signaled: true}, true},
		{"io failure", process.Result{Started: true, ExitCode: 1, Err: io.ErrClosedPipe}, true},
		{"joined cancellation", process.Result{Started: true, ExitCode: 1, Err: errors.Join(errUtils.ErrProcessWaitFailed, context.Canceled)}, true},
		{"joined deadline", process.Result{Started: true, ExitCode: 1, Err: errors.Join(errUtils.ErrProcessWaitFailed, context.DeadlineExceeded)}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runner := NewMockRunner(gomock.NewController(t))
			runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, spec process.TaskSpec) process.Result {
				_, _ = io.WriteString(spec.Streams.Stdout, "response")
				_, _ = io.WriteString(spec.Streams.Stderr, "diagnostic")
				return tc.result
			})
			result, err := runSource(t, `r = exec.run(["tool"], check=False)
output = {"code": r.exit_code, "stdout": r.stdout, "stderr": r.stderr}`, WithProcessRunner(runner))
			if tc.wantErr {
				require.ErrorIs(t, err, errUtils.ErrStarlarkProcessFailed)
				return
			}
			require.NoError(t, err)
			assert.Contains(t, result.Value, `"stdout":"response"`)
			assert.Contains(t, result.Value, `"stderr":"diagnostic"`)
			if tc.result.ExitCode == 7 {
				assert.Contains(t, result.Value, `"code":7`)
			}
		})
	}
}

func TestProcessCaptureWithoutStreaming(t *testing.T) {
	t.Parallel()
	for _, output := range []string{"capture", "stream"} {
		t.Run(output, func(t *testing.T) {
			t.Parallel()
			runner := NewMockRunner(gomock.NewController(t))
			runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, spec process.TaskSpec) process.Result {
				_, _ = io.WriteString(spec.Streams.Stdout, "private response")
				_, _ = io.WriteString(spec.Streams.Stderr, "private diagnostic")
				return process.Result{Started: true}
			})
			var stdout, stderr bytes.Buffer
			result, err := New(WithProcessRunner(runner)).Execute(context.Background(), script.Spec{
				Stdout: &stdout, Stderr: &stderr,
				Source: `r = exec.run(["tool"], output="` + output + `")
output = [r.stdout, r.stderr]`,
			})
			require.NoError(t, err)
			assert.JSONEq(t, `["private response","private diagnostic"]`, result.Value)
			if output == "capture" {
				assert.Empty(t, stdout.String())
				assert.Empty(t, stderr.String())
			} else {
				assert.Equal(t, "private response", stdout.String())
				assert.Equal(t, "private diagnostic", stderr.String())
			}
		})
	}
}

func TestProcessOptionValidation(t *testing.T) {
	t.Parallel()
	for _, source := range []string{`exec.run(["tool"], check=1)`, `exec.run(["tool"], output="quiet")`, `exec.run(["tool"], stream=False)`} {
		_, err := runSource(t, source, WithProcessRunner(NewMockRunner(gomock.NewController(t))))
		require.ErrorIs(t, err, errUtils.ErrStarlark, source)
	}
}

// output="viewport" shows the process output in the host's live viewport while the full output is
// still captured for the result. Without a host viewport it streams like output="stream".
func TestProcessViewportOutput(t *testing.T) {
	t.Parallel()
	type viewportCall struct{ title string }
	for _, tc := range []struct {
		name, source string
		options      []Option
	}{
		{"exec.run", `r = exec.run(["tool", "arg"], output = "viewport")
output = r.stdout`, nil},
		{"atmos.run", `r = atmos.run(["version"], output = "viewport")
output = r.stdout`, []Option{WithAtmosExecutable(func() (string, error) { return "tool", nil })}},
	} {
		t.Run(tc.name+" uses the host viewport", func(t *testing.T) {
			t.Parallel()
			runner := NewMockRunner(gomock.NewController(t))
			var streamed bytes.Buffer
			runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, spec process.TaskSpec) process.Result {
				_, _ = io.WriteString(spec.Streams.Stdout, "child output")
				return process.Result{Started: true}
			})
			var calls []viewportCall
			var stdout, stderr bytes.Buffer
			result, err := New(append([]Option{WithProcessRunner(runner), WithAtmosCommands(testAtmosCatalog())}, tc.options...)...).Execute(t.Context(), script.Spec{
				Name: "test.star", Source: tc.source, Stdout: &stdout, Stderr: &stderr,
				Viewport: func(title string, run func(stdout, stderr io.Writer) error) error {
					calls = append(calls, viewportCall{title: title})
					return run(&streamed, io.Discard)
				},
			})
			require.NoError(t, err)
			require.Len(t, calls, 1)
			assert.Contains(t, calls[0].title, "tool")
			assert.Equal(t, "child output", streamed.String(), "the viewport receives the live output")
			assert.Empty(t, stdout.String(), "the session stream is not written twice")
			assert.Equal(t, "child output", result.Value, "the full output is still captured")
		})
		t.Run(tc.name+" streams without a host viewport", func(t *testing.T) {
			t.Parallel()
			runner := NewMockRunner(gomock.NewController(t))
			runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, spec process.TaskSpec) process.Result {
				_, _ = io.WriteString(spec.Streams.Stdout, "child output")
				return process.Result{Started: true}
			})
			var stdout bytes.Buffer
			result, err := New(append([]Option{WithProcessRunner(runner), WithAtmosCommands(testAtmosCatalog())}, tc.options...)...).Execute(t.Context(), script.Spec{
				Name: "test.star", Source: tc.source, Stdout: &stdout,
			})
			require.NoError(t, err)
			assert.Equal(t, "child output", stdout.String())
			assert.Equal(t, "child output", result.Value)
		})
	}

	t.Run("a failing process still fails the script", func(t *testing.T) {
		t.Parallel()
		runner := NewMockRunner(gomock.NewController(t))
		runner.EXPECT().Run(gomock.Any(), gomock.Any()).Return(process.Result{Started: true, ExitCode: 3, Err: errUtils.ErrProcessWaitFailed})
		_, err := New(WithProcessRunner(runner)).Execute(t.Context(), script.Spec{
			Name: "test.star", Source: `exec.run(["tool"], output = "viewport")`,
			Viewport: func(_ string, run func(stdout, stderr io.Writer) error) error { return run(io.Discard, io.Discard) },
		})
		require.ErrorIs(t, err, errUtils.ErrStarlarkProcessFailed)
	})
}
