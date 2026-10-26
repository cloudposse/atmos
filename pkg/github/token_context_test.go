package github

import (
	"context"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	execpkg "github.com/cloudposse/atmos/pkg/exec"
)

// TestGitHubTokenContextCancelsRunningCLI cancels after process startup without
// depending on how quickly the child test binary initializes on a busy runner.
func TestGitHubTokenContextCancelsRunningCLI(t *testing.T) {
	t.Setenv("ATMOS_GITHUB_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("ATMOS_PRO_GITHUB_TOKEN", "")
	t.Setenv("ATMOS_GITHUB_CLI", "gh")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	executable, err := os.Executable()
	require.NoError(t, err)
	ctrl := gomock.NewController(t)
	executor := execpkg.NewMockCommandExecutor(ctrl)
	executor.EXPECT().CommandContext(gomock.Any(), "gh", "auth", "token").DoAndReturn(
		func(commandCtx context.Context, _ string, _ ...string) *exec.Cmd {
			cmd := exec.CommandContext(commandCtx, executable, "-test.run=^TestHelperProcess$")
			cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1", "HELPER_WAIT_FOR_STDIN=1")
			// os/exec starts copying stdin only after the process starts, so the
			// reader signals readiness without waiting for child initialization.
			cmd.Stdin = &cliStartupReader{started: started, done: commandCtx.Done()}
			return cmd
		},
	)
	withCommander(t, executor)
	finished := make(chan string, 1)
	exited := make(chan struct{})
	t.Cleanup(func() {
		cancel()
		select {
		case <-exited:
		case <-time.After(ghCLITimeout):
			t.Error("CLI lookup did not exit during cleanup")
		}
	})
	go func() {
		defer close(exited)
		finished <- GetGitHubTokenContext(ctx)
	}()
	select {
	case <-started:
	case <-finished:
		t.Fatal("CLI exited before process startup")
	case <-time.After(ghCLITimeout):
		t.Fatal("CLI process did not start within its timeout")
	}
	cancel()
	select {
	case token := <-finished:
		assert.Empty(t, token)
	case <-time.After(time.Second):
		t.Fatal("CLI ignored caller cancellation")
	}
}

// cliStartupReader signals that os/exec launched the child and holds its stdin
// open until cancellation, keeping startup independent of child initialization.
type cliStartupReader struct {
	started chan struct{}
	done    <-chan struct{}
}

// Read signals startup once and ends the stdin copy when the command is canceled.
func (r *cliStartupReader) Read(_ []byte) (int, error) {
	close(r.started)
	<-r.done
	return 0, io.EOF
}

func TestGitHubTokenContextPreservesEarlierDeadline(t *testing.T) {
	t.Setenv("ATMOS_GITHUB_CLI", "gh")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	deadline, _ := ctx.Deadline()
	ctrl := gomock.NewController(t)
	executor := execpkg.NewMockCommandExecutor(ctrl)
	executor.EXPECT().CommandContext(gomock.Any(), "gh", "auth", "token").DoAndReturn(
		func(commandCtx context.Context, _ string, _ ...string) *exec.Cmd {
			actual, ok := commandCtx.Deadline()
			assert.True(t, ok)
			assert.Equal(t, deadline, actual)
			return fakeCLICmd("scoped-token\n", 0)
		},
	)
	withCommander(t, executor)
	assert.Equal(t, "scoped-token", GetGitHubTokenFromCLIContext(ctx))
}

func TestGitHubTokenContextDoesNotSpawnAfterCancellation(t *testing.T) {
	t.Setenv("ATMOS_GITHUB_CLI", "gh")
	executor := execpkg.NewMockCommandExecutor(gomock.NewController(t))
	withCommander(t, executor)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.Empty(t, GetGitHubTokenFromCLIContext(ctx))
}
