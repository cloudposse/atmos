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
