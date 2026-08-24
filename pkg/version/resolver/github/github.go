// Package github implements the "github-tags" and "github-releases"
// datasource resolvers backed by the shared pkg/github client (token chain and
// rate-limit handling included).
package github

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	gh "github.com/cloudposse/atmos/pkg/github"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/version/resolver"
)

// Datasource names served by this resolver.
const (
	DatasourceTags     = "github-tags"
	DatasourceReleases = "github-releases"
)

// packageParts is the number of segments in a GitHub package coordinate.
const packageParts = 2

// ErrInvalidPackage is returned when a package is not in owner/repo form.
var ErrInvalidPackage = errors.New("github package must be in owner/repo form")

// Resolver resolves GitHub tag and release versions.
type Resolver struct{}

// Names returns the datasource names this resolver serves.
func (Resolver) Names() []string {
	defer perf.Track(nil, "github.Resolver.Names")()

	return []string{DatasourceTags, DatasourceReleases}
}

// Versions lists candidate versions from GitHub tags or releases. Releases
// carry publish timestamps (used for update cooldowns) and prerelease flags;
// tags carry the commit SHA as the candidate digest.
func (Resolver) Versions(ctx context.Context, req *resolver.Request) ([]resolver.Candidate, error) {
	defer perf.Track(nil, "github.Resolver.Versions")()

	owner, repo, err := splitPackage(req.Package)
	if err != nil {
		return nil, err
	}
	if req.Datasource == DatasourceReleases {
		return releaseCandidates(owner, repo)
	}
	return tagCandidates(ctx, owner, repo)
}

// Pin resolves a tag or release version to its commit SHA.
func (Resolver) Pin(ctx context.Context, req *resolver.Request, version string) (string, error) {
	defer perf.Track(nil, "github.Resolver.Pin")()

	owner, repo, err := splitPackage(req.Package)
	if err != nil {
		return "", err
	}
	return gh.GetRefSHA(ctx, owner, repo, version)
}

// releaseCandidates lists GitHub releases as candidates.
func releaseCandidates(owner, repo string) ([]resolver.Candidate, error) {
	releases, err := gh.GetReleases(gh.ReleasesOptions{
		Owner:              owner,
		Repo:               repo,
		IncludePrereleases: true,
	})
	if err != nil {
		return nil, err
	}
	candidates := make([]resolver.Candidate, 0, len(releases))
	for _, release := range releases {
		if release == nil || release.TagName == nil {
			continue
		}
		var publishedAt *time.Time
		if release.PublishedAt != nil {
			value := release.PublishedAt.Time
			publishedAt = &value
		}
		candidates = appendReleaseCandidate(candidates, release.GetTagName(), release.GetPrerelease(), publishedAt)
	}
	return candidates, nil
}

func appendReleaseCandidate(candidates []resolver.Candidate, version string, prerelease bool, releasedAt *time.Time) []resolver.Candidate {
	if version == "" {
		return candidates
	}
	return append(candidates, resolver.Candidate{
		Version:    version,
		Prerelease: prerelease,
		ReleasedAt: releasedAt,
	})
}

// tagCandidates lists GitHub tags as candidates, carrying commit SHAs.
func tagCandidates(ctx context.Context, owner, repo string) ([]resolver.Candidate, error) {
	tags, err := gh.ListTags(ctx, owner, repo)
	if err != nil {
		return nil, err
	}
	candidates := make([]resolver.Candidate, 0, len(tags))
	for _, tag := range tags {
		if tag == nil || tag.Name == nil {
			continue
		}
		digest := ""
		if tag.Commit != nil {
			digest = tag.Commit.GetSHA()
		}
		candidates = appendTagCandidate(candidates, tag.GetName(), digest)
	}
	return candidates, nil
}

func appendTagCandidate(candidates []resolver.Candidate, version, digest string) []resolver.Candidate {
	if version == "" {
		return candidates
	}
	return append(candidates, resolver.Candidate{Version: version, Digest: digest})
}

// splitPackage parses an owner/repo package coordinate, tolerating extra path
// segments (e.g. reusable workflow paths) by using the first two.
func splitPackage(pkg string) (string, string, error) {
	parts := strings.Split(pkg, "/")
	if len(parts) < packageParts || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("%w: %q", ErrInvalidPackage, pkg)
	}
	return parts[0], parts[1], nil
}

func init() {
	resolver.Register(Resolver{})
}
