package vendoring

import (
	"context"
	"time"

	ghclient "github.com/cloudposse/atmos/pkg/github"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/vendoring/version"
)

// archivedCheckTimeout bounds a single archived-repo lookup. It mirrors listTagsTimeout's
// pattern but stays tight since this is one lightweight metadata call (not a tag list), and must
// not meaningfully add to the total per-component check time.
const archivedCheckTimeout = 10 * time.Second

// archivedCheckMinRemaining is the GitHub API rate-limit threshold (remaining requests) below
// which the archived check backs off via github.WaitForRateLimit, mirroring the threshold
// convention pkg/github's own callers use.
const archivedCheckMinRemaining = 5

// ArchivedChecker reports whether a source's upstream Git repository is archived. It is an
// interface (mirroring version.RemoteLister) purely for testability: unit tests can fake "is
// this repo archived" without hitting the network.
type ArchivedChecker interface {
	// IsArchived reports whether the repository at gitURI (as produced by
	// version.ExtractGitURI) is archived upstream. Implementations must treat sources that
	// don't support archived detection (non-GitHub hosts) as simply "not archived, no error".
	IsArchived(ctx context.Context, gitURI string) (bool, error)
}

// githubArchivedChecker is the production ArchivedChecker, backed by the GitHub REST API
// (pkg/github.IsArchived). Non-GitHub sources (GitLab, Bitbucket, self-hosted Git servers) are
// reported as not archived with no error, since GitHub's archived flag doesn't apply to them.
type githubArchivedChecker struct{}

// DefaultArchivedChecker is the production ArchivedChecker used when UpdateParams.ArchivedChecker
// is nil.
var DefaultArchivedChecker ArchivedChecker = &githubArchivedChecker{}

// IsArchived implements ArchivedChecker.
func (c *githubArchivedChecker) IsArchived(ctx context.Context, gitURI string) (bool, error) {
	defer perf.Track(nil, "vendoring.githubArchivedChecker.IsArchived")()

	owner, repo, ok := ghclient.ParseOwnerRepo(gitURI)
	if !ok {
		return false, nil
	}

	// Reuse the existing rate-limit helper (rather than hammering the API with zero backoff
	// awareness) before this component's archived-check call. WaitForRateLimit only actually
	// waits when the remaining quota is low, and honors ctx's deadline (archivedCheckTimeout),
	// so it can never block longer than the caller's own timeout budget.
	if err := ghclient.WaitForRateLimit(ctx, archivedCheckMinRemaining); err != nil {
		return false, err
	}

	return ghclient.IsArchived(ctx, owner, repo)
}

// checkArchived runs the archived-upstream check for src, best-effort: any failure (non-GitHub
// source, network hiccup, rate limit, context deadline, etc.) is swallowed and reported as "not
// archived" so a transient GitHub API problem can never mask or break the real version-check
// result for the component. This is intentionally additive-only — it must never turn into a
// StatusFailed for the source.
func checkArchived(src *schema.AtmosVendorSource, checker ArchivedChecker) bool {
	if checker == nil {
		checker = DefaultArchivedChecker
	}

	ctx, cancel := context.WithTimeout(context.Background(), archivedCheckTimeout)
	defer cancel()

	gitURI := version.ExtractGitURI(src.Source)
	archived, err := checker.IsArchived(ctx, gitURI)
	if err != nil {
		log.Debug("Archived-repo check failed; proceeding as not archived", "component", src.Component, "error", err)
		return false
	}
	return archived
}
