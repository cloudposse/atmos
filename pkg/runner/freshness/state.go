package freshness

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
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
// is one JSON file per key under stateDir, guarded by pkg/cache.FileLock on platforms where it
// provides real mutual exclusion. On Windows, pkg/cache.FileLock is explicitly best-effort (no
// native locking there); Save always writes to a uniquely-named temp file before the final
// rename, so concurrent writers never collide on the temp file itself regardless of platform,
// but a concurrent Save and Load can still transiently fail against each other on Windows if
// they land on the exact same instant -- callers already treat a Save/RecordSuccess failure as
// log-and-continue, not fatal, which is the correct posture for a best-effort cache.
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

	path, err := recordPath(stateDir, key)
	if err != nil {
		return Record{}, false, err
	}
	lock := cache.NewFileLock(path)

	var record Record
	found := false
	err = lock.WithRLock(func() error {
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
	path, err := recordPath(stateDir, key)
	if err != nil {
		return err
	}
	lock := cache.NewFileLock(path)

	return lock.WithLock(func() error {
		data, err := json.Marshal(r)
		if err != nil {
			return err
		}
		// Remove any *.tmp leftover from a previous Save that was interrupted (e.g. a killed
		// process) before it could rename its temp file into place -- otherwise these accumulate
		// forever in stateDir. Safe to do here, inside this key's own lock: a temp file matching
		// this pattern can only be a stale leftover, never one a concurrent Save for the SAME key
		// is actively writing (that call would be blocked on this same lock). Best-effort --
		// removal failures never block the actual save.
		removeStaleTempFiles(stateDir, key)
		// A uniquely-named temp file (rather than a fixed "<path>.tmp") means concurrent
		// writers never collide on the temp file itself -- required on Windows, where
		// pkg/cache.FileLock provides no real mutual exclusion (see the StateStore doc
		// comment), so a fixed temp name would let two writers race on the same open handle.
		tmpFile, err := os.CreateTemp(stateDir, key+".*.tmp")
		if err != nil {
			return err
		}
		tmp := tmpFile.Name()
		_, writeErr := tmpFile.Write(data)
		closeErr := tmpFile.Close()
		if writeErr != nil {
			_ = os.Remove(tmp) //nolint:gosec // G703: path is from os.CreateTemp, not user input.
			return writeErr
		}
		if closeErr != nil {
			_ = os.Remove(tmp) //nolint:gosec // G703: path is from os.CreateTemp, not user input.
			return closeErr
		}
		if err := os.Rename(tmp, path); err != nil { //nolint:gosec // G703: path is from os.CreateTemp, not user input.
			_ = os.Remove(tmp) //nolint:gosec // G703: path is from os.CreateTemp, not user input.
			return err
		}
		return nil
	})
}

// recordPath joins stateDir and key into the record's JSON path, rejecting a key that could
// escape stateDir (absolute, containing a path separator, or a "." /".." traversal component) --
// Save also uses key as an os.CreateTemp pattern, where the same characters would be equally
// unsafe.
func recordPath(stateDir, key string) (string, error) {
	switch {
	case key == "":
		return "", fmt.Errorf("%w: state key must not be empty", errUtils.ErrPathTraversal)
	case filepath.IsAbs(key):
		return "", fmt.Errorf("%w: state key %q must not be an absolute path", errUtils.ErrPathTraversal, key)
	case strings.ContainsAny(key, `/\`):
		return "", fmt.Errorf("%w: state key %q must not contain a path separator", errUtils.ErrPathTraversal, key)
	case key == ".." || key == ".":
		return "", fmt.Errorf("%w: state key %q must not be a traversal component", errUtils.ErrPathTraversal, key)
	}
	return filepath.Join(stateDir, key+".json"), nil
}

// removeStaleTempFiles best-effort removes every leftover Save temp file for key in stateDir.
// Glob/Remove errors are ignored: this is opportunistic cleanup, never a save-blocking operation.
func removeStaleTempFiles(stateDir, key string) {
	matches, err := filepath.Glob(filepath.Join(stateDir, key+".*.tmp"))
	if err != nil {
		return
	}
	for _, m := range matches {
		_ = os.Remove(m)
	}
}
