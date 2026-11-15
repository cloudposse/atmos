package s3

import (
	"context"
	"errors"
	"slices"

	"github.com/opencontainers/go-digest"

	errUtils "github.com/cloudposse/atmos/errors"
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
	if err := r.removePayloads(cleanup, plan.abandoned, keep); err != nil {
		return
	}
	if plan.prior != nil {
		if err := r.retirePrior(cleanup, plan.prior, keep, job.recordDigest); err != nil {
			return
		}
	}
	_ = r.deleteObject(cleanup, layout.OperationPath(job.opID))
}

// retirePrior removes the payload, Record, and evidence of the replaced publication, never a Record
// listed as protected. The Record goes last, so a Record that is already gone proves the payload
// and the evidence are too: a retry after a stopped retirement then has nothing left to remove
// instead of failing on the missing Record forever.
func (r *Repository) retirePrior(ctx context.Context, prior *layout.Prior, keep []string, protected ...digest.Digest) error {
	if prior.RecordDigest == "" || slices.Contains(protected, prior.RecordDigest) {
		return nil
	}
	if prior.Location != "" {
		set, err := r.priorSet(ctx, prior)
		if err != nil {
			return err
		}
		if err := r.removePayload(ctx, &ownedPayload{packaging: prior.Packaging, location: prior.Location, set: set}, keep); err != nil {
			return err
		}
	}
	if err := r.deleteEvidence(ctx, prior.RecordDigest); err != nil {
		return err
	}
	return r.deleteObject(ctx, layout.RecordPath(prior.RecordDigest))
}

// priorSet returns the objects the replaced publication wrote, from its Record. A Record that no
// longer exists was retired together with its payload, so nothing is left to name.
func (r *Repository) priorSet(ctx context.Context, prior *layout.Prior) (objectSet, error) {
	record, _, err := r.loadRecord(ctx, prior.RecordDigest)
	if errors.Is(err, errUtils.ErrArtifactNotFound) {
		return objectSet{}, nil
	}
	if err != nil {
		return objectSet{}, err
	}
	return setOfRecord(record), nil
}

// removePayloads deletes the objects of every listed payload that the new publication does not reuse.
func (r *Repository) removePayloads(ctx context.Context, payloads []ownedPayload, keep []string) error {
	for index := range payloads {
		if err := r.removePayload(ctx, &payloads[index], keep); err != nil {
			return err
		}
	}
	return nil
}

// removePayload deletes the objects of a payload that the new publication does not reuse.
func (r *Repository) removePayload(ctx context.Context, owned *ownedPayload, keep []string) error {
	keys, err := owned.set.keys(owned.packaging, owned.location)
	if err != nil {
		return err
	}
	keys = slices.DeleteFunc(keys, func(key string) bool { return slices.Contains(keep, key) })
	return r.deleteAll(ctx, keys)
}
