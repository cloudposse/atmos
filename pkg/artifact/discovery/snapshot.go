package discovery

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/opencontainers/go-digest"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/discovery/cache"
)

// snapshotVersion is the artifact snapshot schema version.
const snapshotVersion = 1

// Snapshot source kinds. They are part of the snapshot key and header.
const (
	kindCollection = "collection"
	kindNamed      = "named"
	kindCatalog    = "catalog"
)

// snapshot is the artifact schema stored in a source's snapshot file: the shared header, the
// digest of the catalog it was read from (catalog snapshots only), and the unfiltered entries.
// Labels and text are filtered after the snapshot is read, so one snapshot answers every query.
type snapshot struct {
	cache.Header
	Digest  string               `json:"digest,omitempty"`
	Entries []artifact.ListEntry `json:"entries"`
}

// decodeSnapshot parses a snapshot file and rejects content that is not a coherent listing. A
// snapshot that fails here is treated as absent: replaced online, rejected offline.
func decodeSnapshot(raw []byte) (*snapshot, error) {
	var value snapshot
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(value.Entries))
	for index := range value.Entries {
		entry := &value.Entries[index]
		if err := validEntry(entry); err != nil {
			return nil, err
		}
		key := entry.Name + "\x00" + entry.Version
		if seen[key] {
			return nil, fmt.Errorf("%w: %s:%s is listed twice", errUtils.ErrArtifactRecordInvalid, entry.Name, entry.Version)
		}
		seen[key] = true
	}
	return &value, nil
}

// validEntry checks that a snapshot entry names a version and carries a coherent pinned reference.
func validEntry(entry *artifact.ListEntry) error {
	ref := artifact.Ref{Name: entry.Name, Version: entry.Version}
	if err := ref.Validate(); err != nil {
		return err
	}
	if err := artifact.ValidateKind(entry.Kind); err != nil {
		return err
	}
	pinned := &entry.Pinned
	if pinned.Ref != ref {
		return fmt.Errorf("%w: %s is pinned as %s", errUtils.ErrArtifactRecordInvalid, ref, pinned.Ref)
	}
	if err := artifact.ValidateDigest(pinned.ResolvedDigest); err != nil {
		return err
	}
	for _, value := range []digest.Digest{pinned.RecordDigest, pinned.ContentDigest} {
		if value == "" {
			continue
		}
		if err := artifact.ValidateDigest(value); err != nil {
			return err
		}
	}
	return nil
}

// sortEntries orders entries deterministically: name, then version.
func sortEntries(entries []artifact.ListEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Name != entries[j].Name {
			return entries[i].Name < entries[j].Name
		}
		return entries[i].Version < entries[j].Version
	})
}

// knownByDigest indexes a previous snapshot's entries by resolved digest, so a refresh can reuse
// the metadata of references whose digest did not change.
func knownByDigest(previous *snapshot) map[digest.Digest]artifact.ListEntry {
	if previous == nil {
		return nil
	}
	known := make(map[digest.Digest]artifact.ListEntry, len(previous.Entries))
	for index := range previous.Entries {
		known[previous.Entries[index].Pinned.ResolvedDigest] = previous.Entries[index]
	}
	return known
}
