package cache

import (
	"github.com/cloudposse/atmos/pkg/auth/cachepaths"
	"github.com/cloudposse/atmos/pkg/perf"
)

// DefaultExcludedPaths returns the root-relative subpaths excluded from the
// CI cache by default, regardless of ci.cache.paths. Each entry is a
// well-known Atmos auth cache directory that lives under the same XDG cache
// root (~/.cache/atmos) that ci.cache archives by default, and persists real
// session material (tokens, refresh tokens, client secrets) or provisioned
// identity/role metadata to disk. Caching the whole root by default is
// intentional (it lets vendoring caches, remote stack-import clones, and
// plugin caches inherit caching for free), but that means these directories
// need an explicit, unconditional carve-out rather than relying on every
// ci.cache.paths configuration to know to avoid them.
//
// The list itself is owned by pkg/auth/cachepaths, not this package: each
// auth subdirectory self-registers there next to the constant that defines
// it (see that package's doc for what's deliberately excluded and why).
// This package only depends on that lightweight registry, not on the auth
// provider packages themselves (pkg/auth/providers/aws, .../azure, etc.),
// which pull in heavyweight, provider-specific transitive dependencies (AWS
// SDK, MSAL, Azure SDK) that this generic cache-archiving package — which
// runs on every Atmos invocation's startup path (auto-restore/save hooks) —
// has no other reason to depend on.
//
// Exported so cmd/ci/cache (the `atmos ci cache paths` command, which feeds
// the native actions/cache action instead of Atmos's own backend) can render
// the same exclusions.
func DefaultExcludedPaths() []string {
	defer perf.Track(nil, "cache.DefaultExcludedPaths")()

	return cachepaths.All()
}
