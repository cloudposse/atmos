package s3

import (
	"context"
	"fmt"

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
// The reference is removed first, with a conditional delete on the observed ETag, so the artifact
// disappears atomically; an interruption afterwards leaves garbage that a later publication of the
// same reference clears, never a reference that points at missing bytes. A pending publication is
// never deleted: its owner must repair it with the same intent.
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
	if options.Lock == nil {
		lock, lockErr := r.acquire(ctx, record.Name+":"+record.Version, r.settings.owner, "")
		if lockErr != nil {
			return nil, lockErr
		}
		defer func() {
			if releaseErr := lock.Release(ctx); err == nil {
				err = releaseErr
			}
		}()
	}
	return r.deleteLocked(ctx, &pinned, &options)
}

func (r *Repository) deleteLocked(ctx context.Context, pinned *artifact.PinnedReference, options *artifact.DeleteOptions) (*artifact.DeleteResult, error) {
	v, err := r.openView(ctx, pinned)
	if err != nil {
		return nil, err
	}
	stored := v.ref.ref
	if options.ExpectedCurrent != nil && options.ExpectedCurrent.ResolvedDigest != stored.RecordDigest {
		return nil, fmt.Errorf("%w: %s:%s is %s but the delete expected %s", errUtils.ErrArtifactConflict, stored.Name, stored.Version, stored.RecordDigest, options.ExpectedCurrent.ResolvedDigest)
	}
	keys, err := setOfRecord(v.record).keys(v.rep.Packaging, v.rep.Location)
	if err != nil {
		return nil, deleteFailed(err)
	}
	if err := r.removeReference(ctx, v.ref); err != nil {
		return nil, err
	}
	if err := r.deleteAll(ctx, keys); err != nil {
		return nil, deleteFailed(err)
	}
	if err := r.deleteEvidence(ctx, stored.RecordDigest); err != nil {
		return nil, deleteFailed(err)
	}
	for _, owned := range []string{layout.RecordPath(stored.RecordDigest), layout.RepresentationPath(stored.Key())} {
		if err := r.deleteObject(ctx, owned); err != nil {
			return nil, deleteFailed(err)
		}
	}
	r.tidyOperation(ctx, stored.OperationID)
	return &artifact.DeleteResult{Deleted: r.deletedLocators(v, keys), Digest: stored.RecordDigest}, nil
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
