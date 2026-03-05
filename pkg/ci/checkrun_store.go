package ci

import (
	"sync"

	"github.com/cloudposse/atmos/pkg/ci/internal/plugin"
)

// Ensure syncMapCheckRunStore implements CheckRunStore.
var _ plugin.CheckRunStore = (*syncMapCheckRunStore)(nil)

// syncMapCheckRunStore implements CheckRunStore using sync.Map.
// It is safe for concurrent use across goroutines.
type syncMapCheckRunStore struct {
	m sync.Map
}

// Store saves a check run ID with the given key.
func (s *syncMapCheckRunStore) Store(key string, id int64) {
	s.m.Store(key, id)
}

// LoadAndDelete retrieves and removes a check run ID.
func (s *syncMapCheckRunStore) LoadAndDelete(key string) (int64, bool) {
	val, ok := s.m.LoadAndDelete(key)
	if !ok {
		return 0, false
	}
	id, ok := val.(int64)
	return id, ok
}

// defaultCheckRunStore is the package-level singleton used to correlate
// before/after hook events across Execute() calls within the same process.
var defaultCheckRunStore = &syncMapCheckRunStore{}
