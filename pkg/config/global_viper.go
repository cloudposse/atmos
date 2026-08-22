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

var globalViper = &SafeViper{}

// GlobalViper returns the mutex-guarded wrapper around the process-wide global
// Viper singleton. Use its methods instead of calling
// viper.GetViper()/viper.Get*/viper.Set directly from any code path that may
// run concurrently (e.g. per-DAG-node hook processing).
func GlobalViper() *SafeViper {
	return globalViper
}

// mergedFilesTracker tracks the config files merged during the most recent
// LoadConfig call, guarded by its own mutex since LoadConfig can run
// concurrently across DAG-scheduler worker goroutines (pkg/scheduler).
type mergedFilesTracker struct {
	mu    sync.Mutex
	files []string
}

func (t *mergedFilesTracker) reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.files = nil
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

var mergedFiles = &mergedFilesTracker{}
