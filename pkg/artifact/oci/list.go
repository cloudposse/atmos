package oci

import (
	"context"
	"sort"
	"strings"

	"github.com/opencontainers/go-digest"
	"golang.org/x/sync/errgroup"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
)

// cursorSeparator splits the name from the version in an opaque list cursor. Neither may contain
// it, so the pair is unambiguous.
const cursorSeparator = "|"

// List implements [artifact.Repository]. A name-scoped listing enumerates the name's tags and
// works without any catalog. A collection listing uses the registry's repository listing scoped
// to the collection prefix, or the layout's references; a registry that cannot enumerate
// returns an unsupported-capability error that says direct named operations still work, never an
// empty page. Only Atmos packages are listed: legacy plugin releases and foreign images that
// share the prefix are never presented as generic artifacts.
//
//nolint:gocritic // The query is passed by value because artifact.Repository fixes the signature.
func (r *Repository) List(ctx context.Context, query artifact.ListQuery) (*artifact.ListPage, error) {
	defer perf.Track(nil, "oci.Repository.List")()

	size := query.PageSize
	if size < 1 {
		size = r.settings.pageSize
	}
	names, err := r.namesFor(ctx, query.Name)
	if err != nil {
		return nil, err
	}
	cursor := parseCursor(query.Cursor)
	var entries []artifact.ListEntry
	for _, name := range names {
		if cursor.skipsName(name) {
			continue
		}
		items, err := r.entriesOf(ctx, name, query.Labels, query.Known)
		if err != nil {
			return nil, err
		}
		entries = append(entries, cursor.remaining(name, items)...)
		if len(entries) > size {
			last := entries[size-1]
			return &artifact.ListPage{Entries: entries[:size], NextCursor: last.Name + cursorSeparator + last.Version}, nil
		}
	}
	return &artifact.ListPage{Entries: entries}, nil
}

// listCursor is a decoded page cursor: the last name and version already returned.
type listCursor struct {
	name    string
	version string
	set     bool
}

func parseCursor(raw string) listCursor {
	name, version, _ := strings.Cut(raw, cursorSeparator)
	return listCursor{name: name, version: version, set: raw != ""}
}

// skipsName reports whether every entry of name precedes the cursor.
func (c listCursor) skipsName(name string) bool { return c.set && name < c.name }

// remaining returns the entries of name that follow the cursor.
func (c listCursor) remaining(name string, items []artifact.ListEntry) []artifact.ListEntry {
	if !c.set || name != c.name {
		return items
	}
	var rest []artifact.ListEntry
	for index := range items {
		if items[index].Version > c.version {
			rest = append(rest, items[index])
		}
	}
	return rest
}

// namesFor returns the names to list: the requested one, or the enumerated collection.
func (r *Repository) namesFor(ctx context.Context, name string) ([]string, error) {
	if name != "" {
		if err := artifact.ValidateName(name); err != nil {
			return nil, err
		}
		return []string{name}, nil
	}
	return r.be.names(ctx)
}

// entriesOf lists the Atmos package versions of one name, sorted by version, that match labels.
// Each tag costs a manifest read and a Record read, bounded by the configured concurrency, except
// a tag whose resolved digest is in known: its entry is reused and only the tag is resolved.
func (r *Repository) entriesOf(ctx context.Context, name string, labels []artifact.Label, known map[digest.Digest]artifact.ListEntry) ([]artifact.ListEntry, error) {
	versions, err := r.be.versions(ctx, name)
	if err != nil {
		return nil, err
	}
	sort.Strings(versions)
	found := make([]*artifact.ListEntry, len(versions))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(r.settings.concurrency)
	for index, version := range versions {
		if artifact.ValidateVersion(version) != nil {
			continue
		}
		group.Go(func() error {
			entry, err := r.entryOf(groupCtx, artifact.Ref{Name: name, Version: version}, known)
			found[index] = entry
			return err
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}
	var entries []artifact.ListEntry
	for _, entry := range found {
		if entry != nil && matchesLabels(entry.Labels, labels) {
			entries = append(entries, *entry)
		}
	}
	return entries, nil
}

// entryOf builds the list entry of one tag, or nil when the tag is not an Atmos package. A tag
// whose resolved digest names a known entry for the same name and version reuses that entry.
func (r *Repository) entryOf(ctx context.Context, ref artifact.Ref, known map[digest.Digest]artifact.ListEntry) (*artifact.ListEntry, error) {
	st, desc, err := r.resolveDescriptor(ctx, ref)
	if err != nil {
		if errorsIsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	if entry, reused := reusable(known, desc.Digest, ref); reused {
		entry.Repository = r.alias
		return &entry, nil
	}
	loaded, err := r.loadCachedRoot(ctx, st, ref.Name, desc)
	if err != nil {
		return nil, err
	}
	if loaded.class != classAtmos {
		return nil, nil
	}
	record, err := r.readRecord(ctx, st, ref.Name, loaded)
	if err != nil {
		return nil, err
	}
	if !recordMatches(record, ref) {
		// A tag serving another version's Record is an alias (or tampering); the Record is listed
		// once, under its own tag, so this tag is not an entry.
		return nil, nil
	}
	pinned := r.pinnedFor(ref, loaded)
	pinned.ContentDigest = record.ContentDigest
	return &artifact.ListEntry{
		Pinned: pinned, Name: record.Name, Version: record.Version, Kind: record.Kind,
		Description: record.Metadata.Description, Labels: record.Metadata.Labels,
		Documents: record.Documents, Repository: r.alias,
	}, nil
}

// matchesLabels reports whether labels hold every selector with an exactly equal value.
func matchesLabels(labels map[string]string, selectors []artifact.Label) bool {
	for _, selector := range selectors {
		if value, ok := labels[selector.Key]; !ok || value != selector.Value {
			return false
		}
	}
	return true
}

// recordMatches reports whether a Record is the one its reference names.
func recordMatches(record *artifact.Record, ref artifact.Ref) bool {
	return requireSubjectOf(record, ref) == nil
}

// reusable returns the known entry of a resolved digest when it describes the same name and version
// as the reference and was bound to that same digest; anything else must be read again.
func reusable(known map[digest.Digest]artifact.ListEntry, resolved digest.Digest, ref artifact.Ref) (artifact.ListEntry, bool) {
	entry, found := known[resolved]
	if !found || entry.Name != ref.Name || entry.Version != ref.Version || entry.Pinned.ResolvedDigest != resolved {
		return artifact.ListEntry{}, false
	}
	return entry, true
}
