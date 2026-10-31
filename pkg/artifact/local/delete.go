package local

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/layout"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Delete implements [artifact.Repository]. Deletion is scoped to the selected artifact: its
// reference, its Record, its representation, its detached evidence, and the payload and sidecars
// it exclusively owns. Nothing outside the repository root is touched, and files the repository
// does not manage (a user's extra file inside a payload directory) stay in place.
//
// The reference record is removed first, so the artifact disappears atomically; an interruption
// afterwards leaves garbage that a later publication of the same reference clears, never a
// reference that points at missing bytes. A pending publication is never deleted: its owner
// must repair it with the same intent.
//
//nolint:gocritic // The pinned reference is passed by value because artifact.Repository fixes the signature.
func (r *Repository) Delete(ctx context.Context, pinned artifact.PinnedReference, options artifact.DeleteOptions) (*artifact.DeleteResult, error) {
	defer perf.Track(nil, "local.Repository.Delete")()

	if options.Scope != "" && options.Scope != artifact.DeleteObject {
		return nil, artifact.NewUnsupportedCapability(r.alias, "delete", "delete/"+string(options.Scope),
			"local repositories delete the reference and the payload it owns together (object scope)")
	}
	t, err := r.openTree(false)
	if err != nil {
		return nil, r.missingRoot(err, pinned.Ref)
	}
	defer t.Close()
	record, _, err := t.loadRecord(pinned.RecordDigest)
	if err != nil {
		return nil, err
	}
	var held *fileLock
	if options.Lock == nil {
		if held, err = r.acquire(ctx, record.Name+":"+record.Version, r.settings.owner, ""); err != nil {
			return nil, err
		}
	}
	result, err := r.deleteLocked(ctx, t, &pinned, &options)
	if held != nil {
		if releaseErr := held.Release(context.WithoutCancel(ctx)); err == nil {
			err = releaseErr
		}
	}
	return result, err
}

func (r *Repository) deleteLocked(ctx context.Context, t *tree, pinned *artifact.PinnedReference, options *artifact.DeleteOptions) (*artifact.DeleteResult, error) {
	v, err := r.openView(ctx, t, pinned)
	if err != nil {
		return nil, err
	}
	if options.ExpectedCurrent != nil && options.ExpectedCurrent.ResolvedDigest != v.ref.RecordDigest {
		return nil, fmt.Errorf("%w: %s:%s is %s but the delete expected %s", errUtils.ErrArtifactConflict, v.ref.Name, v.ref.Version, v.ref.RecordDigest, options.ExpectedCurrent.ResolvedDigest)
	}
	if err := t.Remove(native(layout.ReferencePath(v.ref.Key()))); err != nil {
		return nil, fmt.Errorf("%w: remove reference: %w", errUtils.ErrArtifactDeleteFailed, err)
	}
	removed, err := r.store.remove(t, v)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrArtifactDeleteFailed, err)
	}
	for _, owned := range []string{
		layout.SignatureDir(artifact.SubjectRecord, v.ref.RecordDigest),
		layout.RecordPath(v.ref.RecordDigest),
		layout.RepresentationPath(v.ref.Key()),
	} {
		if err := t.removeAll(owned); err != nil {
			return nil, fmt.Errorf("%w: remove %s: %w", errUtils.ErrArtifactDeleteFailed, owned, err)
		}
	}
	r.tidyOperation(t, v.ref.OperationID)
	t.removeEmptyParents(v.rep.Location)
	return &artifact.DeleteResult{Deleted: removed, Digest: v.ref.RecordDigest}, nil
}
