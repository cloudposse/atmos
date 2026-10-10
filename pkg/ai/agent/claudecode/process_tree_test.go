package claudecode

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

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
				c.SetTimeout(2 * time.Second)
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
