package downloader

import (
	"context"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	execpkg "github.com/cloudposse/atmos/pkg/exec"
	"github.com/cloudposse/atmos/pkg/github"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestDownloadTokenLookupsInheritOperationCancellation(t *testing.T) {
	for _, kind := range []string{"http client", "git detector"} {
		t.Run(kind, func(t *testing.T) {
			t.Setenv("ATMOS_GITHUB_TOKEN", "")
			t.Setenv("GITHUB_TOKEN", "")
			t.Setenv("ATMOS_PRO_GITHUB_TOKEN", "")
			t.Setenv("ATMOS_GITHUB_CLI", "gh")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			executable, err := os.Executable()
			require.NoError(t, err)
			executor := execpkg.NewMockCommandExecutor(gomock.NewController(t))
			executor.EXPECT().CommandContext(gomock.Any(), "gh", "auth", "token").DoAndReturn(
				func(commandCtx context.Context, _ string, _ ...string) *exec.Cmd {
					cancel()
					assert.ErrorIs(t, commandCtx.Err(), context.Canceled, "token command must be a child of the download context")
					return exec.CommandContext(commandCtx, executable, "-test.run=^$")
				},
			)
			t.Cleanup(github.SetCommanderForTesting(executor))
			if kind == "http client" {
				client, err := (&goGetterClientFactory{}).NewClient(ctx, "local-source", "unused", ClientModeFile)
				require.NoError(t, err)
				assert.ErrorIs(t, client.(*goGetterClient).client.Ctx.Err(), context.Canceled)
			} else {
				config := schema.AtmosConfiguration{}
				detector := customDetectors(ctx, &config, "github.com/org/repo")[0].(*CustomGitDetector)
				token, source := detector.resolveToken(hostGitHub)
				assert.Empty(t, token)
				assert.Empty(t, source)
			}
		})
	}
}
