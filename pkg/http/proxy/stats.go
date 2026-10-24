package proxy

import (
	"sync"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Stats tracks per-run, in-memory cache statistics for the savings report. There
// is deliberately no persistent hit/miss store: the filesystem is the index, and
// hit rate is a per-run signal surfaced only by the end-of-run savings report. The
// report has two halves: bytes served from cache (hits → bandwidth saved) and bytes
// fetched from upstream and committed (cacheable misses → cache warmed).
type Stats struct {
	mu            sync.Mutex
	hits          int
	misses        int
	bytesSaved    int64
	objectsCached int
	bytesCached   int64
}

// StatsSnapshot is an immutable copy of Stats at a point in time.
type StatsSnapshot struct {
	Hits          int
	Misses        int
	BytesSaved    int64
	ObjectsCached int
	BytesCached   int64
}

// RecordHit records a cache hit, adding size (the bytes actually streamed to the
// client, not the on-disk object size) to the bandwidth-saved total. A hit is still
// counted when a transfer is interrupted; size then reflects only the bytes delivered.
func (s *Stats) RecordHit(size int64) {
	defer perf.Track(nil, "proxy.Stats.RecordHit")()

	s.mu.Lock()
	defer s.mu.Unlock()
	s.hits++
	s.bytesSaved += size
}

// RecordMiss records a cache miss whose upstream response was not committed to the
// cache (e.g. a non-cacheable status replayed to the client).
func (s *Stats) RecordMiss() {
	defer perf.Track(nil, "proxy.Stats.RecordMiss")()

	s.mu.Lock()
	defer s.mu.Unlock()
	s.misses++
}

// RecordCached records a cache miss whose upstream response was fetched and committed
// to the cache: it counts as a miss and adds size bytes to the warmed total.
func (s *Stats) RecordCached(size int64) {
	defer perf.Track(nil, "proxy.Stats.RecordCached")()

	s.mu.Lock()
	defer s.mu.Unlock()
	s.misses++
	s.objectsCached++
	s.bytesCached += size
}

// Snapshot returns a copy of the current statistics.
func (s *Stats) Snapshot() StatsSnapshot {
	defer perf.Track(nil, "proxy.Stats.Snapshot")()

	s.mu.Lock()
	defer s.mu.Unlock()
	return StatsSnapshot{
		Hits:          s.hits,
		Misses:        s.misses,
		BytesSaved:    s.bytesSaved,
		ObjectsCached: s.objectsCached,
		BytesCached:   s.bytesCached,
	}
}
