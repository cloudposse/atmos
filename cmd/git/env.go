package git

import (
	"context"
	"os"

	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/auth/gitenv"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
)

// envComposer builds Git subprocess environments from Atmos Auth identities. Tests replace it.
var envComposer = gitenv.NewComposer()

// composeEnv builds the subprocess environment for a Git operation: the process environment plus
// the environment of the effective identity. The global --identity flag (or ATMOS_IDENTITY) wins
// over the repository's configured identity; with neither, the process environment is returned
// unchanged (ambient credentials from the developer's Git config or GITHUB_TOKEN apply).
func composeEnv(ctx context.Context, identity string) ([]string, error) {
	defer perf.Track(nil, "git.composeEnv")()

	return envComposer.Compose(ctx, atmosConfigPtr, os.Environ(), viper.GetString(cfg.IdentityFlagName), identity)
}
