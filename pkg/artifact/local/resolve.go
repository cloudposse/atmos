package local

import (
	"context"
	"errors"
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
	defer perf.Track(nil, "local.Repository.Resolve")()

	if err := ctx.Err(); err != nil {
		return artifact.PinnedReference{}, err
	}
	t, err := r.openTree(false)
	if err != nil {
		return artifact.PinnedReference{}, r.missingRoot(err, ref)
	}
	defer t.Close()
	current, err := r.resolveRef(t, ref)
	if err != nil {
		return artifact.PinnedReference{}, err
	}
	return r.pinned(ref, current.record, current.ref.RecordDigest), nil
}

// Pin implements [artifact.Repository]. It resolves the reference and lists the evidence
// descriptors known now. It takes no lock and grants no trust.
func (r *Repository) Pin(ctx context.Context, ref artifact.Ref) (artifact.PinnedReference, error) {
	defer perf.Track(nil, "local.Repository.Pin")()

	pinned, err := r.Resolve(ctx, ref)
	if err != nil {
		return pinned, err
	}
	t, err := r.openTree(false)
	if err != nil {
		return artifact.PinnedReference{}, err
	}
	defer t.Close()
	pinned.Evidence = descriptorsOf(t.listEvidence(pinned.RecordDigest))
	return pinned, nil
}

// Inspect implements [artifact.Repository]. It reads only managed records and the small adjacent
// metadata copy; it never opens the payload.
//
//nolint:gocritic // The pinned reference is passed by value because artifact.Repository fixes the signature.
func (r *Repository) Inspect(ctx context.Context, pinned artifact.PinnedReference) (*artifact.Inspection, error) {
	defer perf.Track(nil, "local.Repository.Inspect")()

	t, err := r.openTree(false)
	if err != nil {
		return nil, r.missingRoot(err, pinned.Ref)
	}
	defer t.Close()
	v, err := r.openView(ctx, t, &pinned)
	if err != nil {
		return nil, err
	}
	if err := r.store.verifySidecars(t, v); err != nil {
		return nil, err
	}
	record := *v.record
	inspection := &artifact.Inspection{
		Pinned:         r.pinned(pinned.Ref, &record, v.ref.RecordDigest),
		Record:         &record,
		Representation: v.rep.Artifact(t.absolute(v.rep.Location)),
	}
	inspection.Evidence = t.listEvidence(v.ref.RecordDigest)
	return inspection, nil
}

// OpenDocument implements [artifact.Repository] without reading the payload. The reader verifies
// the document digest and size and rechecks the reference generation at EOF.
//
//nolint:gocritic // The pinned reference is passed by value because artifact.Repository fixes the signature.
func (r *Repository) OpenDocument(ctx context.Context, pinned artifact.PinnedReference, role artifact.DocumentRole) (*artifact.DocumentStream, error) {
	defer perf.Track(nil, "local.Repository.OpenDocument")()

	t, err := r.openTree(false)
	if err != nil {
		return nil, r.missingRoot(err, pinned.Ref)
	}
	defer t.Close()
	v, err := r.openView(ctx, t, &pinned)
	if err != nil {
		return nil, err
	}
	doc, ok := documentDescriptor(v.record, role)
	if !ok {
		return nil, fmt.Errorf("%w: %s", errUtils.ErrArtifactDocumentMissing, role)
	}
	reader, err := r.store.document(t, v, doc, r.guard(v.ref))
	if err != nil {
		return nil, err
	}
	return &artifact.DocumentStream{Document: *doc, Reader: reader}, nil
}

// List implements [artifact.Repository]. It enumerates managed completed references only: never
// arbitrary user files, pending publications, staging, or journals. Entries are sorted by name
// then version.
func (r *Repository) List(ctx context.Context, query artifact.ListQuery) (*artifact.ListPage, error) {
	defer perf.Track(nil, "local.Repository.List")()

	t, err := r.openTree(false)
	if err != nil {
		if errors.Is(err, errRootMissing) {
			return &artifact.ListPage{}, nil
		}
		return nil, err
	}
	defer t.Close()
	refs, err := t.listReferences()
	if err != nil {
		return nil, err
	}
	candidates := make([]listCandidate, 0, len(refs))
	for _, ref := range refs {
		if ref.Available() && (query.Name == "" || ref.Name == query.Name) {
			candidates = append(candidates, listCandidate{name: ref.Name, version: ref.Version, digest: ref.RecordDigest})
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].less(&candidates[j]) })
	return r.page(ctx, t, candidates, &query)
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

// page loads the Records for one page. Label selectors need every Record, so they are applied
// before paging; without them only the page's Records are read.
func (r *Repository) page(ctx context.Context, t *tree, candidates []listCandidate, query *artifact.ListQuery) (*artifact.ListPage, error) {
	size := query.PageSize
	if size <= 0 {
		size = defaultPageSize
	}
	if query.Cursor != "" {
		start := sort.Search(len(candidates), func(i int) bool { return cursorAfter(&candidates[i], query.Cursor) })
		candidates = candidates[start:]
	}
	page := &artifact.ListPage{}
	for index := range candidates {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		entry, matched, err := r.listEntry(t, &candidates[index], query.Labels)
		if err != nil {
			return nil, err
		}
		if !matched {
			continue
		}
		if len(page.Entries) == size {
			page.NextCursor = page.Entries[len(page.Entries)-1].Name + ":" + page.Entries[len(page.Entries)-1].Version
			return page, nil
		}
		page.Entries = append(page.Entries, entry)
	}
	return page, nil
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

func (r *Repository) listEntry(t *tree, candidate *listCandidate, labels []artifact.Label) (artifact.ListEntry, bool, error) {
	record, _, err := t.loadRecord(candidate.digest)
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
