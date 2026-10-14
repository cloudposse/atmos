package starlark

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/process"
)

func TestExecRunRetry(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, options string
		attempts      int
		failure       process.Result
		wantError     bool
	}{
		{"retry success", ``, 2, process.Result{Started: true, ExitCode: 1}, false},
		{"matching condition", `, "conditions":["temporary"]`, 2, process.Result{Started: true, ExitCode: 1}, false},
		{"unmatched condition", `, "conditions":["permanent"]`, 1, process.Result{Started: true, ExitCode: 1}, true},
		{"launch failure", ``, 1, process.Result{ExitCode: -1, Err: errUtils.ErrProcessStartFailed}, true},
		{"signal", ``, 1, process.Result{Started: true, ExitCode: 137, Signaled: true}, true},
		{"io failure", ``, 1, process.Result{Started: true, ExitCode: 1, Err: io.ErrClosedPipe}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runner := NewMockRunner(gomock.NewController(t))
			attempt := 0
			runner.EXPECT().Run(gomock.Any(), gomock.Any()).Times(tc.attempts).DoAndReturn(func(_ context.Context, spec process.TaskSpec) process.Result {
				attempt++
				if attempt == 1 {
					_, _ = io.WriteString(spec.Streams.Stderr, "temporary")
					return tc.failure
				}
				_, _ = io.WriteString(spec.Streams.Stdout, `{"ok":true}`)
				return process.Result{Started: true}
			})
			result, err := runSource(t, `r = exec.run(["tool"], output="capture", retry={"max_attempts":2, "initial_delay":"0s"`+tc.options+`})
output = [r.data, r.stderr]`, WithProcessRunner(runner))
			if tc.wantError {
				require.ErrorIs(t, err, errUtils.ErrStarlarkProcessFailed)
				return
			}
			require.NoError(t, err)
			assert.JSONEq(t, `[{"ok":true},""]`, result.Value)
		})
	}
}

func TestExecRunDeadline(t *testing.T) {
	t.Parallel()
	runner := NewMockRunner(gomock.NewController(t))
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, _ process.TaskSpec) process.Result {
		_, deadline := ctx.Deadline()
		assert.True(t, deadline)
		<-ctx.Done()
		return process.Result{Started: true, Canceled: true, Err: ctx.Err()}
	})
	_, err := runSource(t, `exec.run(["tool"], timeout="10ms", retry={"max_attempts":3}, check=False)`, WithProcessRunner(runner))
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestExecRunPolicyValidation(t *testing.T) {
	t.Parallel()
	for _, options := range []string{
		`timeout="0s"`, `timeout="-1s"`, `timeout="invalid"`, `timeout=1`,
		`retry={"max_attempts":0}`, `retry={"conditions":["["]}`, `retry={"unknown":1}`, `retry=2`,
	} {
		_, err := runSource(t, `exec.run(["tool"], `+options+`)`, WithProcessRunner(NewMockRunner(gomock.NewController(t))))
		require.Error(t, err, options)
	}
}

func TestExecRunCheckFalseDoesNotRetry(t *testing.T) {
	t.Parallel()
	runner := NewMockRunner(gomock.NewController(t))
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).Return(process.Result{Started: true, ExitCode: 7})
	result, err := runSource(t, `output = exec.run(["tool"], check=False, retry={"max_attempts":3}).exit_code`, WithProcessRunner(runner))
	require.NoError(t, err)
	assert.Equal(t, "7", result.Value)
}
