package claudecode

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/approval"
)

func TestExecClaude_CancellationStopsDescendantWork(t *testing.T) {
	for _, reason := range []string{"timeout", "caller cancellation", "finish grace"} {
		t.Run(reason, func(t *testing.T) {
			scenario := scenarioDescendant
			if reason == "finish grace" {
				scenario = scenarioDescendantResult
			}
			c, _ := newFakeClient(t, scenario)
			dir := t.TempDir()
			t.Setenv(fakeDescendantDirEnv, dir)
			oldGrace := finishGrace
			finishGrace = 100 * time.Millisecond
			t.Cleanup(func() { finishGrace = oldGrace })
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			childReady := false
			c.SetProgressHandler(func(event approval.Event) {
				if event.Kind == approval.ToolStart {
					childReady = true
					if reason == "caller cancellation" {
						cancel()
					}
				}
			})
			if reason == "timeout" {
				// Both provider and child start the full test binary. Race builds
				// on busy CI runners need startup time before testing cancellation.
				// Stay well below the fixture's one-minute lifetime so an orphan
				// would still perform the forbidden work after this timeout.
				c.SetTimeout(10 * time.Second)
			}
			// Always clean up the descendant even when testing an unfixed implementation.
			t.Cleanup(func() {
				raw, err := os.ReadFile(filepath.Join(dir, "ready"))
				if err != nil {
					return
				}
				pid, err := strconv.Atoi(string(raw))
				if err != nil {
					return
				}
				process, err := os.FindProcess(pid)
				if err == nil {
					_ = process.Kill()
				}
			})
			out, err := c.SendMessage(ctx, "launch the Go helper child")
			require.True(t, childReady, "the provider must start its child before cancellation")
			switch reason {
			case "finish grace":
				require.NoError(t, err)
				require.Equal(t, "result with lingering child", out)
			case "timeout":
				require.ErrorIs(t, err, context.DeadlineExceeded)
			default:
				require.ErrorIs(t, err, context.Canceled)
			}
			require.NoError(t, os.WriteFile(filepath.Join(dir, "trigger"), []byte("write only after Atmos returned"), 0o600))
			require.Never(t, func() bool { _, err := os.Stat(filepath.Join(dir, "side-effect")); return err == nil }, 500*time.Millisecond, 10*time.Millisecond, "a cancelled provider child must not perform work after the invocation returned")
		})
	}
}

// TestStartProcess_CancellationPreservesCause covers a timeout or caller cancellation
// while Windows is attaching the suspended process to its job and resuming its thread.
func TestStartProcess_CancellationPreservesCause(t *testing.T) {
	for _, phase := range []string{"start", "attach"} {
		t.Run(phase, func(t *testing.T) {
			for _, reason := range []string{"timeout", "caller cancellation", "startup failure"} {
				t.Run(reason, func(t *testing.T) {
					c, _ := newFakeClient(t, scenarioHang)
					ctx, cancel := context.WithCancel(t.Context())
					defer cancel()
					runCtx, runCancel := context.WithCancel(ctx)
					defer runCancel()
					timer := newPausableTimer(time.Hour, runCancel)
					defer timer.stop()
					sess := &session{timer: timer}
					cause := os.ErrNotExist
					interrupt := func() {
						switch reason {
						case "timeout":
							timer.fire()
							cause = context.DeadlineExceeded
						case "caller cancellation":
							cancel()
							cause = context.Canceled
						}
					}
					cmd := exec.CommandContext(runCtx, c.binaryPath)
					if phase == "start" {
						// Missing executables have different OS error chains. Inject the
						// same startup failure before cancellation on every platform.
						cmd.Err = os.ErrNotExist
						interrupt()
					}
					tree, err := prepareProcessTree(cmd)
					require.NoError(t, err)
					defer tree.close()
					attached := false
					err = c.startProcess(ctx, sess, cmd, func() error {
						attached = true
						interrupt()
						// Cancellation can remove the suspended thread before enumeration.
						return os.ErrNotExist
					})
					require.ErrorIs(t, err, errUtils.ErrCLIProviderExecFailed)
					require.ErrorIs(t, err, cause)
					if phase == "attach" {
						require.True(t, attached)
						require.NotNil(t, cmd.ProcessState, "startup failure must wait for the killed process")
						require.False(t, cmd.ProcessState.Success(), "the failed provider must be terminated")
					} else {
						require.False(t, attached, "a process that did not start cannot be attached")
					}
				})
			}
		})
	}
}
