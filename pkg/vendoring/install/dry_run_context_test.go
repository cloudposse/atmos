package install

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	execpkg "github.com/cloudposse/atmos/pkg/exec"
	"github.com/cloudposse/atmos/pkg/github"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestDryRunGitTokenLookupHonorsCallerCancellation(t *testing.T) {
	for _, kind := range []string{"vendor manifest", "component manifest"} {
		t.Run(kind, func(t *testing.T) {
			t.Setenv("ATMOS_GITHUB_TOKEN", "")
			t.Setenv("GITHUB_TOKEN", "")
			t.Setenv("ATMOS_PRO_GITHUB_TOKEN", "")
			t.Setenv("ATMOS_GITHUB_CLI", "gh")
			t.Setenv("GIT_CONFIG_COUNT", "0")
			config := &schema.AtmosConfiguration{BasePath: t.TempDir(), Settings: schema.AtmosSettings{InjectGithubToken: true}}
			source := "github.com/example/component"
			pkg := NewAtmosVendorPackage(&AtmosPackageParams{Name: "example", URI: source, TargetPath: filepath.Join(config.BasePath, "component"), PkgType: PkgTypeRemote})
			if kind == "component manifest" {
				pkg = NewComponentVendorPackage(&ComponentPackageParams{Name: "example", URI: source, ComponentPath: pkg.Target(), PkgType: PkgTypeRemote})
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			executable, err := os.Executable()
			require.NoError(t, err)
			executor := execpkg.NewMockCommandExecutor(gomock.NewController(t))
			executor.EXPECT().CommandContext(gomock.Any(), "gh", "auth", "token").DoAndReturn(func(commandCtx context.Context, _ string, _ ...string) *exec.Cmd {
				cancel()
				assert.ErrorIs(t, commandCtx.Err(), context.Canceled, "the token subprocess must inherit the dry-run context")
				return exec.CommandContext(ctx, executable, "-test.run=^$")
			})
			t.Cleanup(github.SetCommanderForTesting(executor))
			result, err := InstallContext(ctx, config, pkg, InstallOptions{DryRun: true})
			require.NoError(t, err)
			require.ErrorIs(t, result.Err, context.Canceled, "interrupted best-effort token lookup must not report a successful dry-run")
			require.NoDirExists(t, pkg.Target())
		})
	}
}
