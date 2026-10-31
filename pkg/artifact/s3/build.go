package s3

import (
	"context"
	"slices"

	"github.com/opencontainers/go-digest"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/blob"
	"github.com/cloudposse/atmos/pkg/artifact/layout"
)

// build uploads, writes, and completes. It is the part of the sequence after the pending state is
// durable.
func (r *Repository) build(ctx context.Context, job *publishJob, plan *publishPlan, pending *refState) (*artifact.PublishResult, error) {
	items, err := job.plan()
	if err != nil {
		return nil, err
	}
	if err := r.uploadAll(ctx, job, items); err != nil {
		return nil, wrapWrite("upload payload", err)
	}
	if err := r.writeRecord(ctx, job.recordDigest, job.recordBytes); err != nil {
		return nil, wrapWrite("write record", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rep := r.representation(job)
	if err := r.writeRepresentation(ctx, rep); err != nil {
		return nil, wrapWrite("write representation", err)
	}
	if err := r.settings.fault(faultAfterMetadata); err != nil {
		return nil, err
	}
	completed := pending.ref.Completed(job.recordDigest)
	if _, err := r.writeReference(ctx, &completed, blob.IfMatch(pending.etag)); err != nil {
		return nil, wrapWrite("complete reference", err)
	}
	if err := r.settings.fault(faultAfterCommit); err != nil {
		return nil, err
	}
	r.retire(ctx, job, plan)
	return r.result(job.record, job.recordDigest, rep, plan.admission.Outcome(), plan.scope), nil
}

// representation builds the representation record for the job.
func (r *Repository) representation(job *publishJob) *layout.Representation {
	return &layout.Representation{
		SchemaVersion: layout.SchemaVersion, RepositoryKind: artifact.RepositoryKindS3, Name: job.record.Name, Version: job.record.Version,
		RecordDigest: job.recordDigest, OperationID: job.opID, Packaging: job.prepared.Packaging, Location: job.location,
		Payload: job.payload, Documents: job.documents, RepresentationDigest: job.representationDigest(),
	}
}

// result builds the publish result for a stored artifact.
func (r *Repository) result(record *artifact.Record, recordDigest digest.Digest, rep *layout.Representation, outcome artifact.Outcome, scope artifact.LockScope) *artifact.PublishResult {
	locator := r.uri(rep.Location)
	return &artifact.PublishResult{
		Outcome:        outcome,
		Pinned:         r.pinned(artifact.Ref{Name: record.Name, Version: record.Version}, record, recordDigest),
		Representation: rep.Artifact(locator),
		Location:       locator,
		LockScope:      scope,
	}
}

// retire removes what the completed publication replaced and the journal it owned. Everything here
// is best-effort: the reference is already complete, and a leftover is garbage, not an
// inconsistency. Only exact keys the retired state owned are removed, and never a key the new
// payload uses.
func (r *Repository) retire(ctx context.Context, job *publishJob, plan *publishPlan) {
	cleanup, cancel := detached(ctx)
	defer cancel()
	keep, err := setOfRecord(job.record).keys(job.prepared.Packaging, job.location)
	if err != nil {
		return
	}
	if plan.abandoned != nil {
		r.removePayload(cleanup, plan.abandoned, keep)
	}
	if plan.prior != nil {
		r.retirePrior(cleanup, plan.prior, job, keep)
	}
	_ = r.deleteObject(cleanup, layout.OperationPath(job.opID))
}

// retirePrior removes the payload, Record, and evidence of the replaced publication.
func (r *Repository) retirePrior(ctx context.Context, prior *layout.Prior, job *publishJob, keep []string) {
	if prior.RecordDigest == "" || prior.RecordDigest == job.recordDigest {
		return
	}
	if prior.Location != "" {
		if record, _, err := r.loadRecord(ctx, prior.RecordDigest); err == nil {
			r.removePayload(ctx, &ownedPayload{packaging: prior.Packaging, location: prior.Location, set: setOfRecord(record)}, keep)
		}
	}
	_ = r.deleteEvidence(ctx, prior.RecordDigest)
	_ = r.deleteObject(ctx, layout.RecordPath(prior.RecordDigest))
}

// removePayload deletes the objects of a payload that the new publication does not reuse.
func (r *Repository) removePayload(ctx context.Context, owned *ownedPayload, keep []string) {
	keys, err := owned.set.keys(owned.packaging, owned.location)
	if err != nil {
		return
	}
	keys = slices.DeleteFunc(keys, func(key string) bool { return slices.Contains(keep, key) })
	_ = r.deleteAll(ctx, keys)
}
