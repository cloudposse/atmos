package exec

import (
	"context"
	"fmt"

	gh "github.com/cloudposse/atmos/pkg/github"
	log "github.com/cloudposse/atmos/pkg/logger"
)

// warnIfArchivedGitHubRepo checks whether the given URI references an archived GitHub
// repository and logs a warning if it does. The check is best-effort: any failure to
// reach the GitHub API is silently ignored so that vendoring is never blocked.
// The component argument is included in the warning when non-empty.
func warnIfArchivedGitHubRepo(ctx context.Context, uri, component string) {
	owner, repo, ok := gh.ParseGitHubOwnerRepo(uri)
	if !ok {
		return
	}

	archived, err := gh.IsRepoArchived(ctx, owner, repo)
	if err != nil {
		// Best-effort check: silently skip when the GitHub API is unavailable.
		return
	}

	if archived {
		logArgs := []any{
			"repository", fmt.Sprintf("%s/%s", owner, repo),
		}
		if component != "" {
			logArgs = append(logArgs, "component", component)
		}
		log.Warn("GitHub repository is archived and no longer actively maintained. "+
			"Vendoring from an archived repository may include outdated or unsupported code.",
			logArgs...,
		)
	}
}
