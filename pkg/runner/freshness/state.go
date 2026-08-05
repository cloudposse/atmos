package freshness

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/cloudposse/atmos/pkg/cache"
	"github.com/cloudposse/atmos/pkg/perf"
)

// stateDirMode is the permission mode used when creating the freshness state directory.
const stateDirMode = 0o755

// Record is the persisted state for one step's checksum-based freshness check.
type Record struct {
	SourcesHash string `json:"sources_hash"`
}

// StateStore persists/retrieves the last-recorded Record for a step, keyed by a caller-computed
// stable identity (see Checker.stateKey). Abstracted for testability -- the real implementation
// is one JSON file per key under stateDir, guarded by pkg/cache.FileLock so concurrent steps
// (e.g. inside a `parallel` block, or two CI jobs sharing an archived cache directory) can't
// corrupt each other's state.
type StateStore interface {
	Load(stateDir, key string) (Record, bool, error)
	Save(stateDir, key string, r Record) error
}

type fileStateStore struct{}

// NewStateStore returns the real, production StateStore.
func NewStateStore() StateStore {
	defer perf.Track(nil, "freshness.NewStateStore")()

	return fileStateStore{}
}

func (fileStateStore) Load(stateDir, key string) (Record, bool, error) {
	defer perf.Track(nil, "freshness.StateStore.Load")()

	// Nothing has ever been recorded yet -- avoid creating a lock file (which requires the
	// parent directory to already exist) for a directory that doesn't exist at all.
	if _, err := os.Stat(stateDir); os.IsNotExist(err) {
		return Record{}, false, nil
	}

	path := recordPath(stateDir, key)
	lock := cache.NewFileLock(path)

	var record Record
	found := false
	err := lock.WithRLock(func() error {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			if os.IsNotExist(readErr) {
				return nil
			}
			return readErr
		}
		if unmarshalErr := json.Unmarshal(data, &record); unmarshalErr != nil {
			// A corrupted state file is a cache miss, never a hard failure -- the step
			// just reruns and overwrites it on success.
			//nolint:nilerr // intentional: corrupted state degrades to a cache miss, not an error.
			return nil
		}
		found = true
		return nil
	})
	if err != nil {
		return Record{}, false, err
	}
	return record, found, nil
}

func (fileStateStore) Save(stateDir, key string, r Record) error {
	defer perf.Track(nil, "freshness.StateStore.Save")()

	if err := os.MkdirAll(stateDir, stateDirMode); err != nil {
		return err
	}
	path := recordPath(stateDir, key)
	lock := cache.NewFileLock(path)

	return lock.WithLock(func() error {
		data, err := json.Marshal(r)
		if err != nil {
			return err
		}
		tmp := path + ".tmp"
		if err := os.WriteFile(tmp, data, 0o600); err != nil {
			return err
		}
		return os.Rename(tmp, path)
	})
}

func recordPath(stateDir, key string) string {
	return filepath.Join(stateDir, key+".json")
}
