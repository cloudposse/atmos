package github

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	execpkg "github.com/cloudposse/atmos/pkg/exec"
)

func TestGitHubTokenContextCancelsRunningCLI(t *testing.T) {
	t.Setenv("ATMOS_GITHUB_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("ATMOS_PRO_GITHUB_TOKEN", "")
	t.Setenv("ATMOS_GITHUB_CLI", "gh")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	marker := filepath.Join(t.TempDir(), "ready")
	executable, err := os.Executable()
	require.NoError(t, err)
	ctrl := gomock.NewController(t)
	executor := execpkg.NewMockCommandExecutor(ctrl)
	executor.EXPECT().CommandContext(gomock.Any(), "gh", "auth", "token").DoAndReturn(
		func(commandCtx context.Context, _ string, _ ...string) *exec.Cmd {
			cmd := exec.CommandContext(commandCtx, executable, "-test.run=^TestHelperProcess$")
			cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1", "HELPER_READY_FILE="+marker)
			return cmd
		},
	)
	withCommander(t, executor)
	finished := make(chan string, 1)
	go func() { finished <- GetGitHubTokenContext(ctx) }()
	require.Eventually(t, func() bool { _, err := os.Stat(marker); return err == nil }, 2*time.Second, 10*time.Millisecond, "helper must be running before cancellation")
	cancel()
	select {
	case token := <-finished:
		assert.Empty(t, token)
	case <-time.After(time.Second):
		t.Fatal("CLI ignored caller cancellation")
	}
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
