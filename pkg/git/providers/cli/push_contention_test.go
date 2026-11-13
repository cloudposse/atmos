package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/retry"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/require"
)

// advancingPushClock records waits without introducing timing-dependent tests.
type advancingPushClock struct {
	now    time.Time
	waits  []time.Duration
	onWait func()
}

func (c *advancingPushClock) Now() time.Time { return c.now }
func (c *advancingPushClock) After(d time.Duration) <-chan time.Time {
	c.waits = append(c.waits, d)
	c.now = c.now.Add(d)
	if c.onWait != nil {
		c.onWait()
	}
	ch := make(chan time.Time, 1)
	ch <- c.now
	return ch
}

func pushTestPolicy(attempts int) *schema.RetryConfig {
	delay, capDelay, budget := time.Second, 2*time.Second, 10*time.Second
	return &schema.RetryConfig{
		MaxAttempts: &attempts, BackoffStrategy: schema.BackoffExponential,
		InitialDelay: &delay, MaxDelay: &capDelay, MaxElapsedTime: &budget,
	}
}

func TestPushRetryScheduling(t *testing.T) {
	for _, mode := range []string{"exhaustion", "budget", "cancel", "conflict", "policy", "auth"} {
		t.Run(mode, func(t *testing.T) {
			runner := newFakeRunner()
			runner.on("push origin main", atmosgit.RunResult{StderrTail: "non-fast-forward"}, exitErr(1))
			clock := &advancingPushClock{}
			config := pushTestPolicy(4)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch mode {
			case "budget":
				budget := time.Second
				config.MaxElapsedTime = &budget
			case "cancel":
				clock.onWait = cancel
			case "conflict":
				runner.on("pull --rebase origin main", atmosgit.RunResult{StderrTail: "CONFLICT"}, exitErr(1))
			case "policy":
				runner = newFakeRunner()
				runner.on("push origin main", atmosgit.RunResult{StderrTail: "[remote rejected] protected branch"}, exitErr(1))
			case "auth":
				runner = newFakeRunner()
				runner.on("push origin main", atmosgit.RunResult{StderrTail: "authentication failed non-fast-forward"}, exitErr(1))
			}
			err := New(WithRunner(runner)).pushWithRetry(ctx, &atmosgit.PushOptions{RepoContext: atmosgit.RepoContext{Branch: "main"}}, config, retry.WithClock(clock))
			require.Error(t, err)
			switch mode {
			case "exhaustion":
				require.ErrorIs(t, err, errUtils.ErrGitPushRejected)
				require.Equal(t, []time.Duration{time.Second, 2 * time.Second, 2 * time.Second}, clock.waits)
			case "budget":
				var timeout retry.MaxElapsedTimeError
				require.ErrorAs(t, err, &timeout)
				require.Len(t, runner.calls, 3)
			case "cancel":
				require.ErrorIs(t, err, context.Canceled)
				require.Len(t, runner.calls, 1)
			case "conflict":
				require.Len(t, runner.calls, 2)
				require.NotErrorIs(t, err, errUtils.ErrGitPushRejected)
			default:
				require.Empty(t, clock.waits)
				require.Len(t, runner.calls, 1)
			}
		})
	}
}

// competingPushRunner synchronizes a real competing publisher immediately before
// each selected push, after our rebase. This deterministically recreates sustained
// contention without relying on process scheduling or artificial Git errors.
type competingPushRunner struct {
	runner     atmosgit.Runner
	beforePush func()
	pushes     int
}

//nolint:gocritic // The Runner interface requires value-based options.
func (r *competingPushRunner) Run(ctx context.Context, name string, args []string, opts atmosgit.RunOptions) (atmosgit.RunResult, error) {
	if len(args) > 0 && args[0] == "push" {
		r.pushes++
		r.beforePush()
	}
	return r.runner.Run(ctx, name, args, opts)
}

func TestPushSustainedContention(t *testing.T) {
	requireGitBinary(t)
	for _, mode := range []string{"success", "exhaustion", "cancel", "conflict"} {
		t.Run(mode, func(t *testing.T) {
			env := gitTestEnv(t)
			root := t.TempDir()
			remote, local, competitor := filepath.Join(root, "origin.git"), filepath.Join(root, "local"), filepath.Join(root, "competitor")
			gitRun(t, env, "", "init", "--bare", remote)
			gitRun(t, env, "", "clone", remote, local)
			commitPushFile(t, env, local, "shared", "base\n")
			gitRun(t, env, local, "push", "origin", "main")
			gitRun(t, env, "", "clone", remote, competitor)
			file := "local"
			if mode == "conflict" {
				file = "shared"
			}
			commitPushFile(t, env, local, file, "local\n")
			runner := &competingPushRunner{runner: atmosgit.NewExecRunner()}
			runner.beforePush = func() {
				if mode == "success" && runner.pushes > 2 {
					return
				}
				name := fmt.Sprintf("competitor-%d", runner.pushes)
				if mode == "conflict" {
					name = "shared"
				}
				commitPushFile(t, env, competitor, name, "competing\n")
				gitRun(t, env, competitor, "push", "origin", "main")
			}
			clock := &advancingPushClock{}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "cancel" {
				clock.onWait = cancel
			}
			provider := New(WithRunner(runner))
			opts := &atmosgit.PushOptions{RepoContext: atmosgit.RepoContext{Workdir: local, Env: env, Branch: "main"}}
			err := provider.pushWithRetry(ctx, opts, pushTestPolicy(3), retry.WithClock(clock))
			switch mode {
			case "success":
				require.NoError(t, err)
				require.Equal(t, 3, runner.pushes)
				for _, name := range []string{"local", "competitor-1", "competitor-2"} {
					require.NotEmpty(t, gitRun(t, env, remote, "show", "main:"+name))
				}
				head := gitRun(t, env, remote, "rev-parse", "main")
				require.NoError(t, provider.pushWithRetry(ctx, opts, pushTestPolicy(3), retry.WithClock(clock)))
				require.Equal(t, head, gitRun(t, env, remote, "rev-parse", "main"))
			case "exhaustion":
				require.ErrorIs(t, err, errUtils.ErrGitPushRejected)
				require.Equal(t, 3, runner.pushes)
			case "cancel":
				require.ErrorIs(t, err, context.Canceled)
				require.Equal(t, 1, runner.pushes)
			case "conflict":
				require.Error(t, err)
				require.Equal(t, 1, runner.pushes)
				require.NotEmpty(t, strings.TrimSpace(gitRun(t, env, local, "diff", "--name-only", "--diff-filter=U")))
			}
		})
	}
}

func commitPushFile(t *testing.T, env []string, dir, name, content string) {
	t.Helper()
	writeFile(t, filepath.Join(dir, name), content)
	gitRun(t, env, dir, "add", name)
	gitRun(t, env, dir, "commit", "-m", name)
}
