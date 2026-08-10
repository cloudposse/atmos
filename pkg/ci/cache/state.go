package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// StateFileName is the marker file inside stateDirName.
	stateFileName = "state.json"

	// StateFilePerm is the permission for the state marker file.
	stateFilePerm = 0o600

	// Restore-source markers record how an entry was resolved at restore time.
	// Save uses restoredExact to skip re-uploading unchanged content
	// (write-once), mirroring actions/cache's cache-hit logic.
	restoredExact  = "exact"
	restoredPrefix = "prefix"
	restoredMiss   = "miss"
)

// stateMu guards on-disk state reads/writes within a process. Cross-process
// safety is not required: each key entry is independently meaningful and the
// last writer wins, which matches the idempotent semantics we want.
var stateMu sync.Mutex

// entryState records what happened for a single key during this lifecycle.
type entryState struct {
	// RestoredFrom is restoredExact, restoredPrefix, or restoredMiss.
	RestoredFrom string `json:"restored_from,omitempty"`

	// MatchedKey is the key actually matched on restore (may differ from the
	// requested key when a restore-key prefix matched).
	MatchedKey string `json:"matched_key,omitempty"`

	// Saved is true once this key has been uploaded in this lifecycle.
	Saved bool `json:"saved,omitempty"`
}

// state is the on-disk marker: a map of cache key to entryState.
type state struct {
	Entries map[string]*entryState `json:"entries"`
}

// statePath returns the marker file path for a cache root.
func statePath(root string) string {
	return filepath.Join(root, stateDirName, stateFileName)
}

// loadState reads the marker for root, returning an empty state when absent.
func loadState(root string) *state {
	defer perf.Track(nil, "cache.loadState")()

	s := &state{Entries: map[string]*entryState{}}
	data, err := os.ReadFile(statePath(root))
	if err != nil {
		return s
	}
	_ = json.Unmarshal(data, s)
	if s.Entries == nil {
		s.Entries = map[string]*entryState{}
	}
	return s
}

// saveState writes the marker for root, creating the state directory as needed.
func saveState(root string, s *state) error {
	defer perf.Track(nil, "cache.saveState")()

	dir := filepath.Join(root, stateDirName)
	if err := os.MkdirAll(dir, archiveDirPerm); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(statePath(root), data, stateFilePerm)
}

// recordRestore persists the restore outcome for a key.
func recordRestore(root, key, restoredFrom, matchedKey string) {
	stateMu.Lock()
	defer stateMu.Unlock()

	s := loadState(root)
	s.Entries[key] = &entryState{RestoredFrom: restoredFrom, MatchedKey: matchedKey}
	_ = saveState(root, s)
}

// recordSaved marks a key as saved.
func recordSaved(root, key string) {
	stateMu.Lock()
	defer stateMu.Unlock()

	s := loadState(root)
	e := s.Entries[key]
	if e == nil {
		e = &entryState{}
		s.Entries[key] = e
	}
	e.Saved = true
	_ = saveState(root, s)
}

// lookupEntry returns the recorded state for a key (nil when absent).
func lookupEntry(root, key string) *entryState {
	stateMu.Lock()
	defer stateMu.Unlock()

	return loadState(root).Entries[key]
}
