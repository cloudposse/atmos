package github

import (
	"github.com/cloudposse/atmos/pkg/ci/cache"
	githubcache "github.com/cloudposse/atmos/pkg/ci/cache/github"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Cache implements the provider.CacheProvider capability for GitHub Actions.
// It returns the GitHub Actions cache backend, or errUtils.ErrCacheUnavailable
// when not running inside a runner (the runtime cache token/URL are absent).
func (p *Provider) Cache() (cache.Backend, error) {
	defer perf.Track(nil, "github.Provider.Cache")()

	return githubcache.NewBackend(cache.Options{})
}
