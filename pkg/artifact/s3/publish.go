package s3

import (
	"context"
	"fmt"

	"github.com/opencontainers/go-digest"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/layout"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ownedPayload is a payload location a reference already owns, with the packaging that decides
// which object keys belong to it.
type ownedPayload struct {
	packaging artifact.Packaging
	location  string
	set       objectSet
}

// publishPlan is what the admission decision and the existing state say about one publication.
type publishPlan struct {
	admission layout.Admission
	current   *refState
	prior     *layout.Prior
	owned     []ownedPayload
	abandoned *ownedPayload
	oldOp     *layout.Operation
	expected  *artifact.PinnedReference
	scope     artifact.LockScope
}

// Publish implements [artifact.Repository] with the publication sequence of the contracts:
//
//  1. Hold the reference lock (the service's, or one taken here).
//  2. Read the current reference and admit the publication: conflict, force, unchanged, or resume.
//  3. Reserve the location and record the pending operation journal and pending reference before
//     any payload object is written. Reference ownership is a conditional write.
//  4. Upload the payload and documents, verifying every byte, then write the immutable Record and
//     the representation.
//  5. Complete the reference last, with a conditional write on the pending state's ETag.
//  6. Clean up only the objects this operation owns.
//
// A failure after step 3 leaves the reference pending: unavailable to readers, repaired by a retry
// with the same intent. A response lost after the request was sent is reconciled by reading the
// reference back.
func (r *Repository) Publish(ctx context.Context, prepared *artifact.PreparedArtifact, options artifact.PublishOptions) (result *artifact.PublishResult, err error) {
	defer perf.Track(nil, "s3.Repository.Publish")()

	job, err := r.newJob(prepared)
	if err != nil {
		return nil, err
	}
	scope := artifact.LockScopeRepository
	if options.Lock != nil {
		scope = options.Lock.Scope()
	} else {
		lock, lockErr := r.acquire(ctx, prepared.Record.Name+":"+prepared.Record.Version, r.settings.owner, job.opID)
		if lockErr != nil {
			return nil, lockErr
		}
		defer func() {
			if releaseErr := lock.Release(ctx); err == nil {
				err = releaseErr
			}
		}()
	}
	return r.publishLocked(ctx, job, &options, scope)
}

// publishLocked runs the publication while the reference lock is held.
func (r *Repository) publishLocked(ctx context.Context, job *publishJob, options *artifact.PublishOptions, scope artifact.LockScope) (*artifact.PublishResult, error) {
	plan, err := r.admit(ctx, job, options)
	if err != nil {
		return nil, err
	}
	plan.scope = scope
	if plan.admission == layout.AdmitUnchanged {
		return r.unchanged(ctx, job, plan)
	}
	if err := r.resolveLocation(ctx, job, plan); err != nil {
		return nil, err
	}
	pending, err := r.begin(ctx, job, plan)
	if err != nil {
		return nil, err
	}
	return r.build(ctx, job, plan, pending)
}

// admit reads the current reference and applies the admission rules.
func (r *Repository) admit(ctx context.Context, job *publishJob, options *artifact.PublishOptions) (*publishPlan, error) {
	current, err := r.loadReference(ctx, job.key)
	if err != nil {
		return nil, err
	}
	input := layout.AdmitInput{
		Current: refOf(current), IncomingRecord: job.recordDigest, IncomingIntent: job.prepared.IntentDigest,
		ExpectedPrior: options.ExpectedPrior, Force: options.Force,
	}
	if input.Current != nil && input.Current.Available() {
		if input.CurrentIntent, err = r.completedIntent(ctx, input.Current); err != nil {
			return nil, err
		}
	}
	admission, err := layout.Admit(&input)
	if err != nil {
		return nil, err
	}
	return &publishPlan{admission: admission, current: current, expected: options.ExpectedPrior}, nil
}

// refOf returns the reference of a stored state, or nil for none.
func refOf(state *refState) *layout.Reference {
	if state == nil {
		return nil
	}
	return state.ref
}

// completedIntent reads the intent digest of a completed reference's Record.
func (r *Repository) completedIntent(ctx context.Context, current *layout.Reference) (digest.Digest, error) {
	record, _, err := r.loadRecord(ctx, current.RecordDigest)
	if err != nil {
		return "", err
	}
	return intentOf(record), nil
}

// intentOf returns the intent digest of a Record. An intent that cannot be computed is empty, which
// never counts as unchanged.
func intentOf(record *artifact.Record) digest.Digest {
	intent, err := record.IntentDigest()
	if err != nil {
		return ""
	}
	return intent
}

// unchanged returns the existing artifact for an identical republication and tidies the journal of
// the operation that completed it.
func (r *Repository) unchanged(ctx context.Context, job *publishJob, plan *publishPlan) (*artifact.PublishResult, error) {
	current := plan.current.ref
	record, _, err := r.loadRecord(ctx, current.RecordDigest)
	if err != nil {
		return nil, err
	}
	rep, err := r.loadRepresentation(ctx, job.key)
	if err != nil {
		return nil, err
	}
	if rep == nil || rep.RecordDigest != current.RecordDigest {
		return nil, fmt.Errorf("%w: %s:%s is completed but has no matching representation", errUtils.ErrArtifactIntegrityFailed, current.Name, current.Version)
	}
	r.tidyOperation(ctx, current.OperationID)
	return r.result(record, current.RecordDigest, rep, layout.AdmitUnchanged.Outcome(), plan.scope), nil
}

// tidyOperation removes the journal of a finished operation. It is best-effort: a leftover is
// harmless and is retried by the next publication.
func (r *Repository) tidyOperation(ctx context.Context, operationID string) {
	if operationID == "" {
		return
	}
	cleanup, cancel := detached(ctx)
	defer cancel()
	_ = r.deleteObject(cleanup, layout.OperationPath(operationID))
}

// resolveLocation decides where the payload lives. A resumed publication reuses the location in
// its journal; otherwise the key_pattern is rendered exactly once, here, from the Record's
// captured creation time. The result is persisted; it is never rendered again.
func (r *Repository) resolveLocation(ctx context.Context, job *publishJob, plan *publishPlan) error {
	rep, err := r.loadRepresentation(ctx, job.key)
	if err != nil {
		return err
	}
	if err := r.loadPendingState(ctx, plan, rep); err != nil {
		return err
	}
	if plan.admission.Resuming() && plan.oldOp != nil && plan.oldOp.IntentDigest == job.prepared.IntentDigest && plan.oldOp.Packaging == job.prepared.Packaging {
		job.location = plan.oldOp.Location
		return nil
	}
	if plan.oldOp != nil {
		plan.abandoned = &ownedPayload{
			packaging: plan.oldOp.Packaging, location: plan.oldOp.Location,
			set: setOfDescriptors(plan.oldOp.Payload, plan.oldOp.Documents),
		}
	}
	job.location, err = layout.RenderLocation(r.keyPattern, &layout.PatternInput{
		Name: job.record.Name, Version: job.record.Version, Kind: job.record.Kind, CreatedAt: job.record.CreatedAt.Time(),
	}, job.prepared.Packaging)
	return err
}

// loadPendingState records the locations this reference already owns and the content a
// publication replaces. The interrupted operation's journal is read when one is pending.
func (r *Repository) loadPendingState(ctx context.Context, plan *publishPlan, rep *layout.Representation) error {
	if rep != nil {
		plan.owned = append(plan.owned, ownedPayload{packaging: rep.Packaging, location: rep.Location})
	}
	if plan.current != nil && !plan.current.ref.Available() {
		oldOp, err := r.loadOperation(ctx, plan.current.ref.OperationID)
		if err != nil {
			return err
		}
		plan.oldOp = oldOp
	}
	plan.prior = priorState(plan, rep)
	if plan.oldOp != nil {
		plan.owned = append(plan.owned, ownedPayload{packaging: plan.oldOp.Packaging, location: plan.oldOp.Location})
		if prior := plan.oldOp.Prior; prior != nil && prior.Location != "" {
			plan.owned = append(plan.owned, ownedPayload{packaging: prior.Packaging, location: prior.Location})
		}
	}
	return nil
}

// priorState describes the content a publication replaces, so a later cleanup can retire it.
func priorState(plan *publishPlan, rep *layout.Representation) *layout.Prior {
	if plan.admission == layout.AdmitReplace {
		return priorFrom(plan.current.ref.RecordDigest, rep)
	}
	if !plan.admission.Resuming() {
		return nil
	}
	if plan.oldOp != nil && plan.oldOp.Prior != nil {
		prior := *plan.oldOp.Prior
		return &prior
	}
	if plan.current.ref.PriorRecordDigest == "" {
		return nil
	}
	return priorFrom(plan.current.ref.PriorRecordDigest, rep)
}

// priorFrom builds the prior state for a Record digest, taking the payload location from the
// representation when it still describes that Record.
func priorFrom(recordDigest digest.Digest, rep *layout.Representation) *layout.Prior {
	prior := &layout.Prior{RecordDigest: recordDigest}
	if rep != nil && rep.RecordDigest == recordDigest {
		prior.Location, prior.Packaging = rep.Location, rep.Packaging
	}
	return prior
}
