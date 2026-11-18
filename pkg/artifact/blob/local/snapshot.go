package local

import (
	"encoding/base64"
	"strings"
	"time"

	"github.com/cloudposse/atmos/pkg/artifact/blob"
)

const (
	snapshotPrefix   = "\x00atmos-local:"
	snapshotLifetime = 5 * time.Minute
	maxSnapshots     = 8
)

type listSnapshot struct {
	entries           []entry
	prefix, delimiter string
	created           time.Time
}

// snapshotCursor stores the last ordering token as well as the cache key, so a cursor remains
// usable if the bounded cache is evicted or a caller uses a new Store.
func snapshotCursor(id, last string) string {
	return snapshotPrefix + id + ":" + base64.RawURLEncoding.EncodeToString([]byte(last))
}

func parseSnapshotCursor(cursor string) (id, last string, ok bool) {
	value, found := strings.CutPrefix(cursor, snapshotPrefix)
	if !found {
		return "", cursor, false
	}
	id, encoded, found := strings.Cut(value, ":")
	if !found || id == "" {
		return "", cursor, false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", cursor, false
	}
	return id, string(decoded), true
}

func (s *Store) lookupSnapshot(id string, opts blob.ListOptions) []entry {
	s.snapshotMu.Lock()
	defer s.snapshotMu.Unlock()
	snapshot, ok := s.snapshots[id]
	if !ok || time.Since(snapshot.created) > snapshotLifetime || snapshot.prefix != opts.Prefix || snapshot.delimiter != opts.Delimiter {
		return nil
	}
	return snapshot.entries
}

func (s *Store) rememberSnapshot(entries []entry, opts blob.ListOptions) string {
	id, err := tempName()
	if err != nil {
		return ""
	}
	s.snapshotMu.Lock()
	defer s.snapshotMu.Unlock()
	if s.snapshots == nil {
		s.snapshots = make(map[string]listSnapshot)
	}
	if len(s.snapshots) >= maxSnapshots {
		var oldestID string
		var oldest time.Time
		for key, snapshot := range s.snapshots {
			if oldestID == "" || snapshot.created.Before(oldest) {
				oldestID, oldest = key, snapshot.created
			}
		}
		delete(s.snapshots, oldestID)
	}
	s.snapshots[id] = listSnapshot{entries: entries, prefix: opts.Prefix, delimiter: opts.Delimiter, created: time.Now()}
	return id
}
