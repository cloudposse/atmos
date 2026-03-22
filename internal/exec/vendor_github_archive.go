package exec

import (
	"context"
	"fmt"
	"sync"

	gh "github.com/cloudposse/atmos/pkg/github"
	log "github.com/cloudposse/atmos/pkg/logger"
)

// warnedArchivedRepos tracks repositories for which an archived warning has already
// been emitted during the current run. This prevents duplicate warnings when the same
// owner/repo pair is referenced by both vendor.yaml sources and component.yaml definitions.
// The deduplication key is "owner/repo" (not the full URI), so different URI formats
// pointing to the same repository (e.g., https:// vs github://) are correctly deduplicated.
var warnedArchivedRepos sync.Map

// warnIfArchivedGitHubRepo checks whether the given URI references an archived GitHub
// repository and logs a warning if it does. The check is best-effort: any failure to
// reach the GitHub API is logged at trace level so vendoring is never blocked.
// The component argument is included in the warning when non-empty.
func warnIfArchivedGitHubRepo(ctx context.Context, uri, component string) {
	owner, repo, ok := gh.ParseGitHubOwnerRepo(uri)
	if !ok {
		return
	}

	archived, err := gh.IsRepoArchived(ctx, owner, repo)
	if err != nil {
		// Best-effort check: log at trace level and continue so vendoring is never blocked.
		// Common causes: network unavailable, rate limit exceeded (set GITHUB_TOKEN),
		// or repository not found.
		log.Trace("Skipping archived-repo check", "repository", fmt.Sprintf("%s/%s", owner, repo), "error", err)
		return
	}

	if !archived {
		return
	}

	// Deduplicate: emit the warning only once per repo per run, even if the same
	// repo appears in both vendor.yaml sources and component.yaml definitions.
	repoKey := owner + "/" + repo
	if _, loaded := warnedArchivedRepos.LoadOrStore(repoKey, struct{}{}); loaded {
		// A warning was already emitted for this repo. Log at trace level so
		// engineers can confirm the suppression without polluting normal output.
		// Emit unconditionally; conditionally append the component key.
		traceArgs := []any{"repository", repoKey}
		if component != "" {
			traceArgs = append(traceArgs, "component", component)
		}
		log.Trace("Archived-repo warning already emitted; skipping duplicate", traceArgs...)
		return
	}

	logArgs := []any{
		"repository", repoKey,
	}
	if component != "" {
		logArgs = append(logArgs, "component", component)
	}
	log.Warn("GitHub repository is archived and no longer actively maintained. "+
		"Vendoring from an archived repository may include outdated or unsupported code.",
		logArgs...,
	)
}
