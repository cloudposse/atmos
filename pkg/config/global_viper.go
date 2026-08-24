package config

import (
	"slices"
	"sync"

	"github.com/spf13/viper"
)

// SafeViper wraps the process-wide global Viper singleton with a mutex.
//
// LoadConfig bridges several config-derived values back into the global Viper
// singleton (e.g. profiles.base_path, vendor.update.*, vendor.ci.*) so other
// packages that only have Viper access -- not a *schema.AtmosConfiguration --
// can read them (see resolveProfileSelectionSentinel, bridgeVendorUpdaterConfig
// below, and readers such as pkg/auth/profile_fallback.go and
// cmd/terraform/utils.go). The spf13/viper package has no internal locking of
// its own, and the DAG scheduler (pkg/scheduler) runs many LoadConfig calls concurrently --
// one per graph node -- under --max-concurrency > 1, so every access to the
// singleton must go through GlobalViper() to avoid "concurrent map writes" panics.
//
// This applies even to reads/writes of unrelated keys: viper.Set/Get traverse
// and mutate ONE shared underlying map (Viper.override) via deepSearch, and Go
// maps are not safe for any concurrent read/write access, regardless of which
// key each goroutine touches -- a write to "vendor.update.execution.mode" can
// still race with a concurrent read of an unrelated key like "mask".
//
// Deliberately does not cache *viper.Viper in a field: tests (and
// viper.Reset()-calling production paths) replace viper's default instance at
// runtime, so every method re-resolves viper.GetViper() under the lock rather
// than risk diverging from whatever instance is currently "the" global one.
type SafeViper struct {
	mu sync.RWMutex
}

func (s *SafeViper) Set(key string, value any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	viper.GetViper().Set(key, value)
}

func (s *SafeViper) GetString(key string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return viper.GetViper().GetString(key)
}

func (s *SafeViper) GetBool(key string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return viper.GetViper().GetBool(key)
}

func (s *SafeViper) GetStringSlice(key string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return viper.GetViper().GetStringSlice(key)
}

func (s *SafeViper) IsSet(key string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return viper.GetViper().IsSet(key)
}

// View executes fn with a read lock held on the global Viper singleton,
// giving fn a consistent snapshot for the whole call. Use this instead of
// separate Set/GetBool/IsSet calls whenever a decision combines more than one
// read (e.g. an IsSet presence check followed by a GetBool value read):
// each individual SafeViper method locks and unlocks independently, so a
// concurrent Set() between two separate calls could let the decision combine
// one snapshot's presence result with a different snapshot's value.
// The callback must only call methods on the *viper.Viper it receives, not
// GlobalViper()'s own methods, which would deadlock re-acquiring this lock.
func (s *SafeViper) View(fn func(v *viper.Viper)) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	fn(viper.GetViper())
}

var globalViper = &SafeViper{}

// GlobalViper returns the mutex-guarded wrapper around the process-wide global
// Viper singleton. Use its methods instead of calling
// viper.GetViper()/viper.Get*/viper.Set directly from any code path that may
// run concurrently (e.g. per-DAG-node hook processing).
func GlobalViper() *SafeViper {
	return globalViper
}

// mergedFilesTracker tracks the config files merged during a single LoadConfig
// call, guarded by its own mutex.
type mergedFilesTracker struct {
	mu    sync.Mutex
	files []string
}

func (t *mergedFilesTracker) track(path string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if path != "" && !slices.Contains(t.files, path) {
		t.files = append(t.files, path)
	}
}

func (t *mergedFilesTracker) snapshot() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]string, len(t.files))
	copy(out, t.files)
	return out
}

// mergedFilesRegistry correlates each concurrent LoadConfig call's tracked
// config files with the local *viper.Viper instance that call created ("v" in
// load.go). That pointer already flows through every merge/case-preservation
// helper LoadConfig calls (mergeConfig, mergeConfigFile,
// collectConfigFilesForCasePreservation, ...), so using its identity as the
// correlation key gives each concurrent LoadConfig call an isolated tracker
// without threading a brand-new parameter through that entire call graph.
//
// A mutex alone does not provide this: it prevents concurrent map writes, but
// a single shared tracker still lets one LoadConfig call's reset()/track()
// calls interleave with another's, so a call can snapshot a different call's
// files. Keying by the call's own *viper.Viper instance gives each concurrent
// LoadConfig call its own tracker instead.
//
// Entries are removed by finish, called via defer immediately after start, so
// a later LoadConfig call can never observe a finished call's tracker even if
// Go reuses the same address after garbage collection -- the entry is gone
// before the old v could become collectible.
type mergedFilesRegistry struct {
	mu       sync.Mutex
	trackers map[*viper.Viper]*mergedFilesTracker
}

func (r *mergedFilesRegistry) start(v *viper.Viper) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.trackers[v] = &mergedFilesTracker{}
}

func (r *mergedFilesRegistry) track(v *viper.Viper, path string) {
	r.mu.Lock()
	tracker := r.trackers[v]
	r.mu.Unlock()
	if tracker != nil {
		tracker.track(path)
	}
}

func (r *mergedFilesRegistry) snapshot(v *viper.Viper) []string {
	r.mu.Lock()
	tracker := r.trackers[v]
	r.mu.Unlock()
	if tracker == nil {
		return nil
	}
	return tracker.snapshot()
}

// finish removes v's tracker from the registry and returns its final
// snapshot. Safe to call on a v that was never started (returns nil).
func (r *mergedFilesRegistry) finish(v *viper.Viper) []string {
	r.mu.Lock()
	tracker, ok := r.trackers[v]
	delete(r.trackers, v)
	r.mu.Unlock()
	if !ok {
		return nil
	}
	return tracker.snapshot()
}

var mergedFilesReg = &mergedFilesRegistry{trackers: make(map[*viper.Viper]*mergedFilesTracker)}

// lastLoadedFilesHolder holds the most recently completed LoadConfig call's
// tracked files, guarded by its own mutex. Backs the exported LoadedConfigFiles
// API, whose only consumer (`atmos config list`, cmd/config/list.go) runs a
// single LoadConfig call and reads the result immediately afterward -- it is
// never invoked concurrently with itself, so "last completed call" is a
// well-defined answer for it, unlike the per-call registry above.
type lastLoadedFilesHolder struct {
	mu    sync.Mutex
	files []string
}

func (h *lastLoadedFilesHolder) set(files []string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.files = files
}

func (h *lastLoadedFilesHolder) get() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]string, len(h.files))
	copy(out, h.files)
	return out
}

var lastLoadedFiles = &lastLoadedFilesHolder{}
