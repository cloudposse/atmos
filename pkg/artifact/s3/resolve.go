package s3

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
)

// defaultPageSize is the list page size when the query does not set one.
const defaultPageSize = 100

// Resolve implements [artifact.Repository]. A version reference resolves once to the digest of
// its immutable Record, which every later operation uses. A pending reference is unavailable; a
// digest reference checks the Record's embedded name and names only the current Record.
func (r *Repository) Resolve(ctx context.Context, ref artifact.Ref) (artifact.PinnedReference, error) {
	defer perf.Track(nil, "s3.Repository.Resolve")()

	if err := ctx.Err(); err != nil {
		return artifact.PinnedReference{}, err
	}
	current, err := r.resolveRef(ctx, ref)
	if err != nil {
		return artifact.PinnedReference{}, err
	}
	return r.pinned(ref, current.record, current.ref.ref.RecordDigest), nil
}

// Pin implements [artifact.Repository]. It resolves the reference and lists the evidence
// descriptors known now. It takes no lock and grants no trust.
func (r *Repository) Pin(ctx context.Context, ref artifact.Ref) (artifact.PinnedReference, error) {
	defer perf.Track(nil, "s3.Repository.Pin")()

	pinned, err := r.Resolve(ctx, ref)
	if err != nil {
		return pinned, err
	}
	loaded, _, err := r.loadEvidence(ctx, pinned.RecordDigest)
	if err != nil {
		return artifact.PinnedReference{}, err
	}
	pinned.Evidence = descriptorsOf(r.evidenceRefs(loaded))
	return pinned, nil
}

// Inspect implements [artifact.Repository]. It reads only managed records; it never opens the
// payload or its documents.
//
//nolint:gocritic // The pinned reference is passed by value because artifact.Repository fixes the signature.
func (r *Repository) Inspect(ctx context.Context, pinned artifact.PinnedReference) (*artifact.Inspection, error) {
	defer perf.Track(nil, "s3.Repository.Inspect")()

	v, err := r.openView(ctx, &pinned)
	if err != nil {
		return nil, err
	}
	record := *v.record
	loaded, _, err := r.loadEvidence(ctx, v.ref.ref.RecordDigest)
	if err != nil {
		return nil, err
	}
	return &artifact.Inspection{
		Pinned:         r.pinned(pinned.Ref, &record, v.ref.ref.RecordDigest),
		Record:         &record,
		Representation: v.rep.Artifact(r.uri(v.rep.Location)),
		Evidence:       r.evidenceRefs(loaded),
	}, nil
}

// listCandidate is one completed reference awaiting its Record.
type listCandidate struct {
	name    string
	version string
	digest  digest.Digest
}

func (c *listCandidate) less(other *listCandidate) bool {
	if c.name != other.name {
		return c.name < other.name
	}
	return c.version < other.version
}

// List implements [artifact.Repository]. It enumerates managed completed references only: never
// arbitrary user objects, pending publications, journals, or claims. The reference objects are
// listed with the store's pagination, so any number of references is found; entries are returned
// sorted by name then version, which the hashed reference keys cannot provide on their own.
func (r *Repository) List(ctx context.Context, query artifact.ListQuery) (*artifact.ListPage, error) {
	defer perf.Track(nil, "s3.Repository.List")()

	refs, err := r.listReferences(ctx)
	if err != nil {
		return nil, err
	}
	candidates := make([]listCandidate, 0, len(refs))
	for _, state := range refs {
		if state.ref.Available() && (query.Name == "" || state.ref.Name == query.Name) {
			candidates = append(candidates, listCandidate{name: state.ref.Name, version: state.ref.Version, digest: state.ref.RecordDigest})
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].less(&candidates[j]) })
	return r.page(ctx, candidates, &query)
}

// page loads the Records for one page. Records are read a page at a time, concurrently; label
// selectors may skip entries, in which case the next batch is read.
func (r *Repository) page(ctx context.Context, candidates []listCandidate, query *artifact.ListQuery) (*artifact.ListPage, error) {
	size := query.PageSize
	if size <= 0 {
		size = defaultPageSize
	}
	if query.Cursor != "" {
		start := sort.Search(len(candidates), func(i int) bool { return cursorAfter(&candidates[i], query.Cursor) })
		candidates = candidates[start:]
	}
	page := &artifact.ListPage{}
	for len(candidates) > 0 {
		batch := candidates[:min(len(candidates), size+1)]
		candidates = candidates[len(batch):]
		entries, matched, err := r.loadEntries(ctx, batch, query.Labels)
		if err != nil {
			return nil, err
		}
		for index := range entries {
			if !matched[index] {
				continue
			}
			if len(page.Entries) == size {
				last := page.Entries[len(page.Entries)-1]
				page.NextCursor = last.Name + ":" + last.Version
				return page, nil
			}
			page.Entries = append(page.Entries, entries[index])
		}
	}
	return page, nil
}

// loadEntries reads the Records of a batch of candidates concurrently.
func (r *Repository) loadEntries(ctx context.Context, batch []listCandidate, labels []artifact.Label) ([]artifact.ListEntry, []bool, error) {
	entries := make([]artifact.ListEntry, len(batch))
	matched := make([]bool, len(batch))
	err := r.forEach(ctx, len(batch), func(ctx context.Context, index int) error {
		entry, ok, err := r.listEntry(ctx, &batch[index], labels)
		entries[index], matched[index] = entry, ok
		return err
	})
	return entries, matched, err
}

// cursorAfter reports whether the candidate sorts strictly after the cursor's name and version.
func cursorAfter(candidate *listCandidate, cursor string) bool {
	index := strings.LastIndex(cursor, ":")
	if index < 0 {
		return true
	}
	after := listCandidate{name: cursor[:index], version: cursor[index+1:]}
	return after.less(candidate)
}

func (r *Repository) listEntry(ctx context.Context, candidate *listCandidate, labels []artifact.Label) (artifact.ListEntry, bool, error) {
	record, _, err := r.loadRecord(ctx, candidate.digest)
	if err != nil {
		return artifact.ListEntry{}, false, err
	}
	if record.Name != candidate.name || record.Version != candidate.version {
		return artifact.ListEntry{}, false, fmt.Errorf("%w: record %s describes %s:%s but its reference is %s:%s",
			errUtils.ErrArtifactIntegrityFailed, candidate.digest, record.Name, record.Version, candidate.name, candidate.version)
	}
	for _, label := range labels {
		if value, ok := record.Metadata.Labels[label.Key]; !ok || value != label.Value {
			return artifact.ListEntry{}, false, nil
		}
	}
	return artifact.ListEntry{
		Pinned:      r.pinned(artifact.Ref{Name: record.Name, Version: record.Version}, record, candidate.digest),
		Name:        record.Name,
		Version:     record.Version,
		Kind:        record.Kind,
		Description: record.Metadata.Description,
		Labels:      record.Metadata.Labels,
		Documents:   record.Documents,
	}, true, nil
}

// descriptorsOf lists evidence descriptors.
func descriptorsOf(evidence []artifact.SignatureEvidence) []ocispec.Descriptor {
	out := make([]ocispec.Descriptor, 0, len(evidence))
	for index := range evidence {
		out = append(out, evidence[index].Descriptor)
	}
	return out
}
