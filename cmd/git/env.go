package git

import (
	"context"
	"os"

	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/perf"
)

// composeEnv builds the subprocess environment for a Git operation.
// When identity is non-empty, the identity environment (GIT_CONFIG_* from
// GitHub STS etc.) is merged over the current process environment via
// atmosgit.ComposeEnvironment. When identity is empty, the process environment
// is returned unchanged (ambient credentials from the developer's Git config
// or GITHUB_TOKEN apply). Auth manager injection is a planned future enhancement.
func composeEnv(ctx context.Context, identity string) ([]string, error) {
	defer perf.Track(nil, "git.composeEnv")()

	return atmosgit.ComposeEnvironment(ctx, os.Environ(), identity, nil)
}
