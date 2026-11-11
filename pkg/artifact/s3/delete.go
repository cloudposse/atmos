package s3

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/blob"
	"github.com/cloudposse/atmos/pkg/artifact/layout"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Delete implements [artifact.Repository]. Deletion is scoped to the selected artifact: its
// reference, its Record, its representation, its detached evidence, and the payload objects it
// exclusively owns. Objects the repository does not manage (a user's extra object below a payload
// prefix) stay in place, and nothing outside the repository prefix is touched.
//
// The reference is first made unavailable using the existing pending state; its journal retains
// payload ownership until cleanup succeeds. A later publication can finish interrupted deletion.
// A pending publication is never deleted: its owner must repair it with the same intent.
//
//nolint:gocritic // The pinned reference is passed by value because artifact.Repository fixes the signature.
func (r *Repository) Delete(ctx context.Context, pinned artifact.PinnedReference, options artifact.DeleteOptions) (result *artifact.DeleteResult, err error) {
	defer perf.Track(nil, "s3.Repository.Delete")()

	if options.Scope != "" && options.Scope != artifact.DeleteObject {
		return nil, artifact.NewUnsupportedCapability(r.alias, "delete", "delete/"+string(options.Scope),
			"S3 repositories delete the reference and the payload it owns together (object scope)")
	}
	record, _, err := r.loadRecord(ctx, pinned.RecordDigest)
	if err != nil {
		return nil, err
	}
	release, err := r.holdClaim(ctx, options.Lock, record.Name+":"+record.Version)
	if err != nil {
		return nil, err
	}
	defer func() {
		if releaseErr := release(); err == nil {
			err = releaseErr
		}
	}()
	return r.deleteLocked(ctx, &pinned, &options)
}

func (r *Repository) deleteLocked(ctx context.Context, pinned *artifact.PinnedReference, options *artifact.DeleteOptions) (result *artifact.DeleteResult, err error) {
	v, err := r.openView(ctx, pinned)
	if err != nil {
		return nil, err
	}
	stored := v.ref.ref
	if options.ExpectedCurrent != nil && options.ExpectedCurrent.ResolvedDigest != stored.RecordDigest {
		return nil, fmt.Errorf("%w: %s:%s is %s but the delete expected %s", errUtils.ErrArtifactConflict, stored.Name, stored.Version, stored.RecordDigest, options.ExpectedCurrent.ResolvedDigest)
	}
	claims, err := r.acquire(ctx, claimsResource, r.settings.owner, "")
	if err != nil {
		return nil, err
	}
	defer func() {
		if releaseErr := claims.Release(ctx); err == nil {
			err = releaseErr
		}
	}()
	id := deletionPrefix + uuid.NewString()
	intent := deletionIntent(id)
	op := &layout.Operation{
		SchemaVersion: layout.SchemaVersion, RepositoryKind: artifact.RepositoryKindS3,
		OperationID: id, Name: stored.Name, Version: stored.Version, IntentDigest: intent,
		RecordDigest: stored.RecordDigest, CapturedAt: v.record.CreatedAt, Packaging: v.rep.Packaging,
		Location: v.rep.Location, Payload: append([]ocispec.Descriptor{}, v.rep.Payload...),
		Documents: append([]ocispec.Descriptor{}, v.rep.Documents...), Owner: r.settings.owner,
	}
	if err := r.writeOperation(ctx, op); err != nil {
		return nil, deleteFailed(err)
	}
	pending := layout.NewPendingReference(stored.Name, stored.Version, id, intent, stored)
	etag, err := r.writeReference(ctx, &pending, blob.IfMatch(v.ref.etag))
	if err != nil {
		return nil, deleteFailed(err)
	}
	pendingState := &refState{ref: &pending, etag: etag}
	keys, err := r.finishDeletion(ctx, pendingState, op)
	if err != nil {
		return nil, err
	}
	r.tidyOperation(ctx, stored.OperationID)
	return &artifact.DeleteResult{Deleted: r.deletedLocators(v, keys), Digest: stored.RecordDigest}, nil
}

// Deletions use the existing pending-reference and operation format so old writers refuse
// competing publications and reserve the old location until all payload cleanup is complete.
const deletionPrefix = "delete-"

func deletionIntent(id string) digest.Digest { return digest.FromString("atmos/s3-delete/" + id) }

func isDeletion(ref *layout.Reference) bool {
	return ref != nil && !ref.Available() && strings.HasPrefix(ref.OperationID, deletionPrefix) &&
		ref.IntentDigest == deletionIntent(ref.OperationID) && ref.PriorRecordDigest != ""
}

func validDeletionJournal(state *refState, op *layout.Operation) bool {
	return isDeletion(state.ref) && op != nil && op.OperationID == state.ref.OperationID &&
		op.IntentDigest == state.ref.IntentDigest && op.RecordDigest == state.ref.PriorRecordDigest &&
		op.Name == state.ref.Name && op.Version == state.ref.Version
}

func (r *Repository) finishDeletion(ctx context.Context, state *refState, op *layout.Operation) ([]string, error) {
	if !validDeletionJournal(state, op) {
		return nil, fmt.Errorf("%w: invalid deletion journal", errUtils.ErrArtifactIntegrityFailed)
	}
	keys, err := setOfDescriptors(op.Payload, op.Documents).keys(op.Packaging, op.Location)
	if err != nil {
		return nil, deleteFailed(err)
	}
	if err := r.deleteAll(ctx, keys); err != nil {
		return nil, deleteFailed(err)
	}
	if err := r.deleteEvidence(ctx, op.RecordDigest); err != nil {
		return nil, deleteFailed(err)
	}
	for _, key := range []string{layout.RecordPath(op.RecordDigest), layout.RepresentationPath(state.ref.Key())} {
		if err := r.deleteObject(ctx, key); err != nil {
			return nil, deleteFailed(err)
		}
	}
	if err := r.removeReference(ctx, state); err != nil {
		return nil, err
	}
	r.tidyOperation(ctx, op.OperationID)
	return keys, nil
}

// recoverDeletion is called with the reference lock held, before admitting a new publication.
func (r *Repository) recoverDeletion(ctx context.Context, state *refState) (err error) {
	claims, err := r.acquire(ctx, claimsResource, r.settings.owner, "")
	if err != nil {
		return err
	}
	defer func() {
		if releaseErr := claims.Release(ctx); err == nil {
			err = releaseErr
		}
	}()
	op, err := r.loadOperation(ctx, state.ref.OperationID)
	if err != nil {
		return err
	}
	_, err = r.finishDeletion(ctx, state, op)
	return err
}

// removeReference deletes the reference only while its ETag is unchanged. A lost response is settled
// by reading the reference: an absent one proves the delete happened.
func (r *Repository) removeReference(ctx context.Context, state *refState) error {
	err := r.deleteConditional(ctx, layout.ReferencePath(state.ref.Key()), state.etag)
	if err == nil {
		return nil
	}
	if blob.IsAmbiguous(err) {
		settle, cancel := detached(ctx)
		defer cancel()
		if current, readErr := r.loadReference(settle, state.ref.Key()); readErr == nil && current == nil {
			return nil
		}
	}
	return fmt.Errorf("%w: remove reference: %w", errUtils.ErrArtifactDeleteFailed, err)
}

// deletedLocators reports what a deletion removed: the archive objects, or the payload prefix for
// per-file packaging.
func (r *Repository) deletedLocators(v *view, keys []string) []string {
	if !isArchive(v.rep.Packaging) {
		return []string{r.uri(v.rep.Location) + "/"}
	}
	locators := make([]string, 0, len(keys))
	for _, key := range keys {
		locators = append(locators, r.uri(key))
	}
	return locators
}

// deleteFailed marks a failure to remove an artifact's objects.
func deleteFailed(err error) error {
	return fmt.Errorf("%w: %w", errUtils.ErrArtifactDeleteFailed, err)
}
