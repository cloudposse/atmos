package discovery

import (
	"time"

	"github.com/cloudposse/atmos/pkg/artifact/discovery/cache"
	"github.com/cloudposse/atmos/pkg/artifact/service"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Option customizes a [Discovery].
type Option func(*Discovery)

// WithDirectory sets the snapshot directory. The default is the XDG cache directory
// atmos/artifacts/discovery.
func WithDirectory(directory string) Option {
	defer perf.Track(nil, "discovery.WithDirectory")()

	return func(d *Discovery) { d.directory = directory }
}

// WithTTL sets how long a snapshot stays fresh. The default is fifteen minutes.
func WithTTL(ttl time.Duration) Option {
	defer perf.Track(nil, "discovery.WithTTL")()

	return func(d *Discovery) { d.ttl = ttl }
}

// WithClock injects the clock that stamps and ages snapshots.
func WithClock(now func() time.Time) Option {
	defer perf.Track(nil, "discovery.WithClock")()

	return func(d *Discovery) { d.now = now }
}

// WithOffline decides, per repository, whether discovery is offline. Offline discovery serves only
// an existing valid snapshot and never contacts a repository; a refresh is rejected.
func WithOffline(offline func(info *service.RepositoryInfo) bool) Option {
	defer perf.Track(nil, "discovery.WithOffline")()

	return func(d *Discovery) { d.offline = offline }
}

// WithPolicy replaces the default [LocationPolicy].
func WithPolicy(policy Policy) Option {
	defer perf.Track(nil, "discovery.WithPolicy")()

	return func(d *Discovery) { d.policy = policy }
}

// WithMaxSize bounds one snapshot file in bytes.
func WithMaxSize(size int64) Option {
	defer perf.Track(nil, "discovery.WithMaxSize")()

	return func(d *Discovery) { d.maxSize = size }
}

// defaultTTL is the freshness window of a snapshot.
const defaultTTL = cache.DefaultTTL
