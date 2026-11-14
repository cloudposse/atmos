package artifacttest

import (
	"fmt"
	"sync"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
)

// DefaultPageSize is the fake's list page size; it is small so tests exercise pagination.
const DefaultPageSize = 2

// FailPoint names an injectable interruption in the publication lifecycle.
type FailPoint string

// Injectable interruptions, in lifecycle order.
const (
	// FailBeforePayload fails before anything changed.
	FailBeforePayload FailPoint = "before-payload"

	// FailDuringPayload fails while payload blobs are being read; the publication stays pending.
	FailDuringPayload FailPoint = "during-payload"

	// FailAfterMetadata fails after the immutable record was written but before the reference moved.
	FailAfterMetadata FailPoint = "after-metadata"

	// FailAfterCommit fails after the reference was published: the server-side commit succeeded
	// but the response was lost.
	FailAfterCommit FailPoint = "after-commit"
)

// Options configure a [Repository].
type Options struct {
	// Alias names the repository in locators and errors; default "memory".
	Alias string

	// Capabilities overrides the default full-featured capabilities.
	Capabilities *artifact.Capabilities

	// PageSize is the list page size; default [DefaultPageSize].
	PageSize int

	// OnResolved runs after Resolve has pinned a version reference and released its internal
	// lock, so a test can move the reference to simulate a tag changing mid-operation.
	OnResolved func(ref artifact.Ref)
}

// blob is one stored payload or document blob.
type blob struct {
	descriptor   ocispec.Descriptor
	role         artifact.BlobRole
	documentRole artifact.DocumentRole
	path         string
	data         []byte
}

// stored is one immutable published record with its blobs.
type stored struct {
	record           artifact.Record
	canonical        []byte
	recordDigest     digest.Digest
	blobs            []blob
	packaging        artifact.Packaging
	payloadAvailable bool
}

// pending is an in-flight or interrupted publication.
type pending struct {
	operationID string
	intent      digest.Digest
}

// failure is an armed injected failure.
type failure struct {
	err   error
	times int
}

// Repository is an in-memory [artifact.Repository]. It is safe for concurrent use.
type Repository struct {
	mu       sync.Mutex
	opts     Options
	caps     artifact.Capabilities
	refs     map[string]digest.Digest
	records  map[digest.Digest]*stored
	pendings map[string]*pending
	evidence map[digest.Digest][]artifact.EvidenceBlob
	locks    map[string]*lockState
	failures map[FailPoint]*failure
	calls    map[string]int
}

var (
	_ artifact.Repository     = (*Repository)(nil)
	_ artifact.EvidenceReader = (*Repository)(nil)
	_ artifact.EvidenceWriter = (*Repository)(nil)
)

// New returns an empty in-memory repository.
func New(opts Options) *Repository {
	defer perf.Track(nil, "artifacttest.New")()

	if opts.Alias == "" {
		opts.Alias = "memory"
	}
	if opts.PageSize <= 0 {
		opts.PageSize = DefaultPageSize
	}
	caps := DefaultCapabilities()
	if opts.Capabilities != nil {
		caps = *opts.Capabilities
	}
	return &Repository{
		opts:     opts,
		caps:     caps,
		refs:     map[string]digest.Digest{},
		records:  map[digest.Digest]*stored{},
		pendings: map[string]*pending{},
		evidence: map[digest.Digest][]artifact.EvidenceBlob{},
		locks:    map[string]*lockState{},
		failures: map[FailPoint]*failure{},
		calls:    map[string]int{},
	}
}

// DefaultCapabilities is the full-featured capability set of the in-memory repository.
func DefaultCapabilities() artifact.Capabilities {
	defer perf.Track(nil, "artifacttest.DefaultCapabilities")()

	return artifact.Capabilities{
		Read:                true,
		Write:               true,
		EnumerateNamed:      true,
		EnumerateCollection: true,
		Delete:              artifact.DeleteObject,
		ConditionalUpdate:   true,
		Packagings:          []artifact.Packaging{artifact.PackagingNone, artifact.PackagingTar, artifact.PackagingTgz, artifact.PackagingZip},
		PreservesModes:      true,
		SubjectKinds:        []artifact.SubjectKind{artifact.SubjectRecord},
		EvidenceFormats:     []artifact.EvidenceFormat{FakeEvidenceFormat},
		LockScopes:          []artifact.LockScope{artifact.LockScopeHost},
	}
}

// Capabilities implements [artifact.Repository].
func (r *Repository) Capabilities() artifact.Capabilities {
	defer perf.Track(nil, "artifacttest.Repository.Capabilities")()

	return r.caps
}

// InjectFailure arms err to be returned from Publish at point, times times (0 means once).
func (r *Repository) InjectFailure(point FailPoint, err error, times int) {
	defer perf.Track(nil, "artifacttest.Repository.InjectFailure")()

	r.mu.Lock()
	defer r.mu.Unlock()
	if times <= 0 {
		times = 1
	}
	r.failures[point] = &failure{err: err, times: times}
}

// Calls returns how many times the named repository method was called.
func (r *Repository) Calls(method string) int {
	defer perf.Track(nil, "artifacttest.Repository.Calls")()

	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls[method]
}

// TotalCalls returns the total number of repository method calls (including Capabilities-free
// methods only), which dry-run tests assert is zero.
func (r *Repository) TotalCalls() int {
	defer perf.Track(nil, "artifacttest.Repository.TotalCalls")()

	r.mu.Lock()
	defer r.mu.Unlock()
	total := 0
	for _, n := range r.calls {
		total += n
	}
	return total
}

// MoveRef repoints the version reference at another stored record digest, simulating a mutable
// tag being moved. It reports false when the digest is unknown.
func (r *Repository) MoveRef(ref artifact.Ref, to digest.Digest) bool {
	defer perf.Track(nil, "artifacttest.Repository.MoveRef")()

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.records[to]; !ok {
		return false
	}
	r.refs[refKey(ref.Name, ref.Version)] = to
	return true
}

// SetOnResolved installs (or clears) the hook that runs after a version reference was pinned.
func (r *Repository) SetOnResolved(hook func(ref artifact.Ref)) {
	defer perf.Track(nil, "artifacttest.Repository.SetOnResolved")()

	r.mu.Lock()
	defer r.mu.Unlock()
	r.opts.OnResolved = hook
}

// IsPending reports whether the reference has an in-flight or interrupted publication.
func (r *Repository) IsPending(ref artifact.Ref) bool {
	defer perf.Track(nil, "artifacttest.Repository.IsPending")()

	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.pendings[refKey(ref.Name, ref.Version)]
	return ok
}

// RecordDigestOf returns the record digest a version reference currently points at.
func (r *Repository) RecordDigestOf(ref artifact.Ref) (digest.Digest, bool) {
	defer perf.Track(nil, "artifacttest.Repository.RecordDigestOf")()

	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.refs[refKey(ref.Name, ref.Version)]
	return d, ok
}

func refKey(name, version string) string {
	return name + ":" + version
}

// count records a call; callers hold r.mu.
func (r *Repository) count(method string) {
	r.calls[method]++
}

// trip consumes an armed failure at point; callers hold r.mu.
func (r *Repository) trip(point FailPoint) error {
	armed, ok := r.failures[point]
	if !ok || armed.times <= 0 {
		return nil
	}
	armed.times--
	return armed.err
}

func (r *Repository) locator(name, version string) string {
	return fmt.Sprintf("mem://%s/%s/%s", r.opts.Alias, name, version)
}

func (r *Repository) pinnedFor(ref artifact.Ref, s *stored) artifact.PinnedReference {
	return artifact.PinnedReference{
		Ref:            ref,
		Repository:     "mem://" + r.opts.Alias,
		ResolvedDigest: s.recordDigest,
		RecordDigest:   s.recordDigest,
		ContentDigest:  s.record.ContentDigest,
	}
}

func (r *Repository) representation(s *stored) artifact.Representation {
	rep := artifact.Representation{
		Packaging:    s.packaging,
		Locator:      r.locator(s.record.Name, s.record.Version),
		RecordDigest: s.recordDigest,
	}
	payloadDigests := ""
	for index := range s.blobs {
		if s.blobs[index].role == artifact.BlobPayload {
			rep.Payload = append(rep.Payload, s.blobs[index].descriptor)
			payloadDigests += s.blobs[index].descriptor.Digest.String() + "\n"
		} else {
			rep.Documents = append(rep.Documents, s.blobs[index].descriptor)
		}
	}
	rep.RepresentationDigest = digest.FromString(payloadDigests)
	return rep
}
