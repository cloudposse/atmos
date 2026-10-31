package artifacttest

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/opencontainers/go-digest"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Publish implements [artifact.Repository] with the publication lifecycle of the contracts: the
// pending operation is recorded before payload changes, the immutable record is written, and the
// completed reference is published last. Injected failures leave the state a real interruption
// would. A reference that is pending for a different intent stays unavailable, with the owning
// operation ID in the error, instead of being stolen.
func (r *Repository) Publish(ctx context.Context, prepared *artifact.PreparedArtifact, options artifact.PublishOptions) (*artifact.PublishResult, error) {
	defer perf.Track(nil, "artifacttest.Repository.Publish")()

	r.mu.Lock()
	defer r.mu.Unlock()
	r.count("Publish")
	if !r.caps.Write {
		return nil, artifact.NewUnsupportedCapability(r.opts.Alias, "publish", "write", "the repository is read-only")
	}
	if err := r.trip(FailBeforePayload); err != nil {
		return nil, err
	}
	record := prepared.Record
	key := refKey(record.Name, record.Version)
	recordDigest, err := record.RecordDigest()
	if err != nil {
		return nil, err
	}
	outcome, existing, err := r.admit(key, prepared, options, recordDigest)
	if err != nil {
		return nil, err
	}
	if outcome == artifact.OutcomeUnchanged {
		return r.publishResult(&existing.record, existing, artifact.OutcomeUnchanged), nil
	}
	r.pendings[key] = &pending{operationID: prepared.OperationID, intent: prepared.IntentDigest}
	blobs, err := r.readBlobs(ctx, prepared)
	if err != nil {
		return nil, err
	}
	s := &stored{record: record, recordDigest: recordDigest, packaging: prepared.Packaging, blobs: blobs, payloadAvailable: true}
	if s.canonical, err = record.Canonical(); err != nil {
		return nil, err
	}
	r.records[recordDigest] = s
	if err := r.trip(FailAfterMetadata); err != nil {
		return nil, err
	}
	r.commit(key, recordDigest)
	if err := r.trip(FailAfterCommit); err != nil {
		return nil, err
	}
	return r.publishResult(&record, s, outcome), nil
}

// admit applies conditional-update, intent, and force rules and returns the outcome to produce.
// For an unchanged outcome it also returns the stored record that already satisfies the intent.
func (r *Repository) admit(key string, prepared *artifact.PreparedArtifact, options artifact.PublishOptions, recordDigest digest.Digest) (artifact.Outcome, *stored, error) {
	record := &prepared.Record
	if held, isPending := r.pendings[key]; isPending {
		if held.intent != prepared.IntentDigest {
			return "", nil, fmt.Errorf("%w: %s:%s is pending for operation %s with a different intent; recover it before publishing", errUtils.ErrArtifactPublicationUnavailable, record.Name, record.Version, held.operationID)
		}
		if _, exists := r.refs[key]; exists {
			return artifact.OutcomeReplaced, nil, nil
		}
		return artifact.OutcomeCreated, nil, nil
	}
	current, exists := r.refs[key]
	if err := checkExpectedPrior(key, current, exists, options.ExpectedPrior); err != nil {
		return "", nil, err
	}
	if !exists {
		return artifact.OutcomeCreated, nil, nil
	}
	existing := r.records[current]
	if current == recordDigest {
		return artifact.OutcomeUnchanged, existing, nil
	}
	if intent, err := existing.record.IntentDigest(); err == nil && intent == prepared.IntentDigest {
		return artifact.OutcomeUnchanged, existing, nil
	}
	if !options.Force {
		return "", nil, fmt.Errorf("%w: %s already exists with a different intent", errUtils.ErrArtifactConflict, key)
	}
	return artifact.OutcomeReplaced, nil, nil
}

// checkExpectedPrior is the compare-and-swap guard.
func checkExpectedPrior(key string, current digest.Digest, exists bool, expected *artifact.PinnedReference) error {
	switch {
	case expected == nil && exists:
		return fmt.Errorf("%w: %s now exists (%s) but the writer expected it to be absent", errUtils.ErrArtifactConflict, key, current)
	case expected != nil && !exists:
		return fmt.Errorf("%w: %s no longer exists but the writer expected %s", errUtils.ErrArtifactConflict, key, expected.ResolvedDigest)
	case expected != nil && expected.ResolvedDigest != current:
		return fmt.Errorf("%w: %s is %s but the writer expected %s", errUtils.ErrArtifactConflict, key, current, expected.ResolvedDigest)
	}
	return nil
}

// readBlobs reads every staged blob, verifying its descriptor.
func (r *Repository) readBlobs(ctx context.Context, prepared *artifact.PreparedArtifact) ([]blob, error) {
	blobs := make([]blob, 0, len(prepared.Blobs))
	for index := range prepared.Blobs {
		staged := &prepared.Blobs[index]
		reader, err := staged.Open(ctx)
		if err != nil {
			return nil, err
		}
		if index == 0 {
			if err := r.trip(FailDuringPayload); err != nil {
				_ = reader.Close()
				return nil, err
			}
		}
		data, readErr := io.ReadAll(reader)
		closeErr := reader.Close()
		if err := errors.Join(readErr, closeErr); err != nil {
			return nil, err
		}
		if int64(len(data)) != staged.Descriptor.Size || digest.FromBytes(data) != staged.Descriptor.Digest {
			return nil, fmt.Errorf("%w: staged blob %q does not match its descriptor", errUtils.ErrArtifactIntegrityFailed, staged.Path)
		}
		blobs = append(blobs, blob{descriptor: staged.Descriptor, role: staged.Role, documentRole: staged.DocumentRole, path: staged.Path, data: data})
	}
	return blobs, nil
}

// commit publishes the completed reference last and retires the replaced payload.
func (r *Repository) commit(key string, recordDigest digest.Digest) {
	previous, hadPrevious := r.refs[key]
	r.refs[key] = recordDigest
	delete(r.pendings, key)
	if hadPrevious && previous != recordDigest && !r.referenced(previous) {
		r.records[previous].payloadAvailable = false
	}
}

// referenced reports whether any reference still points at the record digest.
func (r *Repository) referenced(recordDigest digest.Digest) bool {
	for _, d := range r.refs {
		if d == recordDigest {
			return true
		}
	}
	return false
}

func (r *Repository) publishResult(record *artifact.Record, s *stored, outcome artifact.Outcome) *artifact.PublishResult {
	ref := artifact.Ref{Name: record.Name, Version: record.Version}
	return &artifact.PublishResult{
		Outcome:        outcome,
		Pinned:         r.pinnedFor(ref, s),
		Representation: r.representation(s),
		Location:       r.locator(record.Name, record.Version),
		LockScope:      artifact.LockScopeHost,
	}
}

// Delete implements [artifact.Repository]. The deletion is scoped to the pinned record: the
// references pointing at it and its exclusively owned payload.
//
//nolint:gocritic // The pinned reference is passed by value because artifact.Repository fixes the signature.
func (r *Repository) Delete(_ context.Context, pinned artifact.PinnedReference, options artifact.DeleteOptions) (*artifact.DeleteResult, error) {
	defer perf.Track(nil, "artifacttest.Repository.Delete")()

	r.mu.Lock()
	defer r.mu.Unlock()
	r.count("Delete")
	if r.caps.Delete == artifact.DeleteNone || r.caps.Delete == "" {
		return nil, artifact.NewUnsupportedCapability(r.opts.Alias, "delete", "delete", "the repository cannot delete")
	}
	s, ok := r.records[pinned.RecordDigest]
	if !ok {
		return nil, fmt.Errorf("%w: record %s", errUtils.ErrArtifactNotFound, pinned.RecordDigest)
	}
	if options.ExpectedCurrent != nil && options.ExpectedCurrent.ResolvedDigest != pinned.RecordDigest {
		return nil, fmt.Errorf("%w: the delete target changed", errUtils.ErrArtifactConflict)
	}
	result := &artifact.DeleteResult{Digest: pinned.RecordDigest}
	for key, d := range r.refs {
		if d != pinned.RecordDigest {
			continue
		}
		delete(r.refs, key)
		result.Deleted = append(result.Deleted, r.locator(s.record.Name, s.record.Version))
	}
	delete(r.records, pinned.RecordDigest)
	delete(r.evidence, pinned.RecordDigest)
	return result, nil
}
