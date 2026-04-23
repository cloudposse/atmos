package filesystem

import (
	"expvar"
	"sync"
	"sync/atomic"
)

// globExpvarOnce ensures expvar variables are registered at most once.
// expvar.Publish panics on duplicate registration, so callers that call
// RegisterGlobCacheExpvars multiple times (e.g., in tests) are protected.
var globExpvarOnce sync.Once

// RegisterGlobCacheExpvars publishes the glob-cache counters as expvar integers
// under the /debug/vars HTTP endpoint.  The function is no-op after the first call
// (duplicate registration would panic).
//
// Call this once at program startup to expose cache performance metrics:
//
//	import _ "expvar" // enable /debug/vars
//	filesystem.RegisterGlobCacheExpvars()
//
// The following variables are published:
//   - atmos_glob_cache_hits      – number of cache hits since last reset
//   - atmos_glob_cache_misses    – number of cache misses since last reset
//   - atmos_glob_cache_evictions – number of LRU evictions since last reset
//   - atmos_glob_cache_len       – current number of entries in the cache
func RegisterGlobCacheExpvars() {
	globExpvarOnce.Do(func() {
		if expvar.Get("atmos_glob_cache_hits") == nil {
			expvar.Publish("atmos_glob_cache_hits", expvar.Func(func() any {
				return atomic.LoadInt64(&globMatchesHits)
			}))
		}
		if expvar.Get("atmos_glob_cache_misses") == nil {
			expvar.Publish("atmos_glob_cache_misses", expvar.Func(func() any {
				return atomic.LoadInt64(&globMatchesMisses)
			}))
		}
		if expvar.Get("atmos_glob_cache_evictions") == nil {
			expvar.Publish("atmos_glob_cache_evictions", expvar.Func(func() any {
				return atomic.LoadInt64(&globMatchesEvictions)
			}))
		}
		if expvar.Get("atmos_glob_cache_len") == nil {
			expvar.Publish("atmos_glob_cache_len", expvar.Func(func() any {
				globMatchesLRUMu.RLock()
				defer globMatchesLRUMu.RUnlock()
				return globMatchesLRU.Len()
			}))
		}
	})
}
