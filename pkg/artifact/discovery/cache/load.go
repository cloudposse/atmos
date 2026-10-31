package cache

import (
	"context"
	"os"

	"github.com/cloudposse/atmos/pkg/filelock"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Mode selects how [Load] may use the network.
type Mode struct {
	// Refresh bypasses snapshot freshness and fetches again. It fails when Offline is set.
	Refresh bool

	// Offline serves an existing valid snapshot of any age and never fetches.
	Offline bool
}

// Source describes one discovery source and the callbacks [Load] needs. P is a pointer to the
// caller's snapshot struct T, which embeds [Header].
type Source[T any, P interface {
	*T
	Snapshot
}] struct {
	// Kind and Reference identify the source and select its snapshot file.
	Kind, Reference string

	// Version is the snapshot schema version a readable snapshot must carry.
	Version int

	// Decode parses and validates the content of a snapshot file. A snapshot that fails decoding
	// is treated as absent: it is replaced online and rejected offline.
	Decode func(raw []byte) (P, error)

	// Fetch returns a fresh snapshot, given the previous valid one (nil when none). It may reuse
	// the previous snapshot's content when nothing changed. Returning nil without an error means
	// the source is unavailable but optional: nothing is cached.
	Fetch func(ctx context.Context, previous P) (P, error)

	// Authorize runs the current policy before a snapshot is served from cache (cached true) and
	// before the source is contacted (cached false). It may be nil.
	Authorize func(ctx context.Context, cached bool) error

	// ErrOffline is returned in offline mode when no valid snapshot exists or a refresh is requested.
	ErrOffline error

	// ErrTooLarge is returned when a fetched snapshot exceeds the size bound.
	ErrTooLarge error
}

// Load returns the snapshot of a source and whether it came from the cache. Concurrent loads of the
// same source (in this or another process) are serialized on a lock file, so a cold cache is
// filled once and the others read the result.
func Load[T any, P interface {
	*T
	Snapshot
}](ctx context.Context, store *Store, source *Source[T, P], mode Mode) (P, bool, error) {
	defer perf.Track(nil, "cache.Load")()

	if err := os.MkdirAll(store.directory, DirectoryMode); err != nil {
		return nil, false, err
	}
	path := store.Path(source.Kind, source.Reference)
	var (
		result P
		cached bool
	)
	err := filelock.New(path+".lock").WithExclusive(ctx, func() error {
		var loadErr error
		result, cached, loadErr = update(ctx, store, source, mode, path)
		return loadErr
	})
	if err != nil {
		return nil, false, err
	}
	return result, cached, nil
}

// update applies the freshness, offline, and refresh rules while the source lock is held.
func update[T any, P interface {
	*T
	Snapshot
}](ctx context.Context, store *Store, source *Source[T, P], mode Mode, path string) (P, bool, error) {
	previous := readSnapshot(store, source, path)
	if previous != nil && (mode.Offline || (!mode.Refresh && store.Fresh(previous.SnapshotHeader().FetchedAt))) {
		if err := authorize(ctx, source, true); err != nil {
			return nil, true, err
		}
		return previous, true, nil
	}
	if mode.Offline {
		return nil, false, source.ErrOffline
	}
	current, err := fetch(ctx, store, source, path, previous)
	return current, false, err
}

// fetch reads the source and, only after the read completed, atomically replaces the snapshot. A
// nil snapshot without an error means the source is unavailable but optional.
func fetch[T any, P interface {
	*T
	Snapshot
}](ctx context.Context, store *Store, source *Source[T, P], path string, previous P) (P, error) {
	if err := authorize(ctx, source, false); err != nil {
		return nil, err
	}
	current, err := source.Fetch(ctx, previous)
	if err != nil {
		return nil, err
	}
	if current == nil {
		// Do not cache an unavailable optional source as an empty result.
		return nil, nil
	}
	current.SetSnapshotHeader(Header{Version: source.Version, Source: source.Reference, Kind: source.Kind, FetchedAt: store.now().UTC()})
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := store.WriteSnapshot(path, current, source.ErrTooLarge); err != nil {
		return nil, err
	}
	return current, nil
}

func authorize[T any, P interface {
	*T
	Snapshot
}](ctx context.Context, source *Source[T, P], cached bool) error {
	if source.Authorize == nil {
		return nil
	}
	return source.Authorize(ctx, cached)
}

// readSnapshot returns the valid snapshot of a source, or nil when it is absent, oversized,
// corrupt, written for another source or schema version, or fails content validation.
func readSnapshot[T any, P interface {
	*T
	Snapshot
}](store *Store, source *Source[T, P], path string) P {
	raw := store.read(path)
	if raw == nil {
		return nil
	}
	value, err := source.Decode(raw)
	if err != nil || value == nil {
		return nil
	}
	header := value.SnapshotHeader()
	if header.Version != source.Version || header.Source != source.Reference || header.Kind != source.Kind {
		return nil
	}
	return value
}
