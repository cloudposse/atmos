package local

import (
	"context"
	"errors"
	"fmt"

	"github.com/opencontainers/go-digest"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/layout"
	"github.com/cloudposse/atmos/pkg/perf"
)

// publishPlan is what the admission decision and the existing state say about one publication.
type publishPlan struct {
	admission layout.Admission
	current   *layout.Reference
	prior     *layout.Prior
	owned     []string
	abandoned string
	oldOp     *layout.Operation
	expected  *artifact.PinnedReference
}

// Publish implements [artifact.Repository] with the publication sequence of the contracts:
//
//  1. Hold the reference lock (the service's, or one taken here) and the resolved-location lock.
//  2. Read the current reference and admit the publication: conflict, force, unchanged, or resume.
//  3. Reserve the location and record the pending operation journal and pending reference before
//     any payload path changes.
//  4. Stage the payload and sidecars on the same filesystem, write the immutable Record, install
//     the payload, and write the representation.
//  5. Complete the reference last, with an atomic replacement.
//  6. Clean up only the temporary resources this operation owns.
//
// A failure after step 3 leaves the reference pending: unavailable to readers, repaired by a retry
// with the same intent. The caller reconciles an ambiguous response by reading the reference.
func (r *Repository) Publish(ctx context.Context, prepared *artifact.PreparedArtifact, options artifact.PublishOptions) (result *artifact.PublishResult, err error) {
	defer perf.Track(nil, "local.Repository.Publish")()

	job, err := r.newJob(prepared)
	if err != nil {
		return nil, err
	}
	t, err := r.openTree(true)
	if err != nil {
		return nil, err
	}
	defer t.Close()
	var held []*fileLock
	defer func() {
		if releaseErr := releaseAll(ctx, held...); err == nil {
			err = releaseErr
		}
	}()
	if options.Lock == nil {
		lock, lockErr := r.acquire(ctx, prepared.Record.Name+":"+prepared.Record.Version, r.settings.owner, job.opID)
		if lockErr != nil {
			return nil, lockErr
		}
		held = append(held, lock)
	}
	return r.publishLocked(ctx, t, job, &options, &held)
}

// publishLocked runs the publication while the reference lock is held.
func (r *Repository) publishLocked(ctx context.Context, t *tree, job *publishJob, options *artifact.PublishOptions, held *[]*fileLock) (*artifact.PublishResult, error) {
	plan, err := r.admit(t, job, options)
	if err != nil {
		return nil, err
	}
	if plan.admission == layout.AdmitUnchanged {
		return r.unchanged(t, job, plan.current)
	}
	if err := r.resolveLocation(t, job, plan); err != nil {
		return nil, err
	}
	locationLock, err := r.acquire(ctx, locationResource+layout.FoldPath(job.location), r.settings.owner, job.opID)
	if err != nil {
		return nil, err
	}
	*held = append(*held, locationLock)
	pending, err := r.begin(ctx, t, job, plan)
	if err != nil {
		return nil, err
	}
	result, err := r.build(ctx, t, job, plan, pending)
	if err != nil && !isCrash(err) && !job.installing {
		_ = t.removeAll(job.staging)
	}
	return result, err
}

// admit reads the current reference and applies the admission rules.
func (r *Repository) admit(t *tree, job *publishJob, options *artifact.PublishOptions) (*publishPlan, error) {
	current, err := t.loadReference(job.key)
	if err != nil {
		return nil, err
	}
	input := layout.AdmitInput{
		Current: current, IncomingRecord: job.recordDigest, IncomingIntent: job.prepared.IntentDigest,
		ExpectedPrior: options.ExpectedPrior, Force: options.Force,
	}
	if current != nil && current.Available() {
		record, _, loadErr := t.loadRecord(current.RecordDigest)
		if loadErr != nil {
			return nil, loadErr
		}
		if intent, intentErr := record.IntentDigest(); intentErr == nil {
			input.CurrentIntent = intent
		}
	}
	admission, err := layout.Admit(&input)
	if err != nil {
		return nil, err
	}
	return &publishPlan{admission: admission, current: current, expected: options.ExpectedPrior}, nil
}

// unchanged returns the existing artifact for an identical republication and tidies leftovers of
// the operation that completed it.
func (r *Repository) unchanged(t *tree, job *publishJob, current *layout.Reference) (*artifact.PublishResult, error) {
	record, _, err := t.loadRecord(current.RecordDigest)
	if err != nil {
		return nil, err
	}
	rep, err := t.loadRepresentation(job.key)
	if err != nil {
		return nil, err
	}
	if rep == nil || rep.RecordDigest != current.RecordDigest {
		return nil, fmt.Errorf("%w: %s:%s is completed but has no matching representation", errUtils.ErrArtifactIntegrityFailed, current.Name, current.Version)
	}
	r.tidyOperation(t, current.OperationID)
	return r.result(t, record, current.RecordDigest, rep, layout.AdmitUnchanged.Outcome()), nil
}

// tidyOperation removes the journal and staging directory of a finished operation. It is
// best-effort: a leftover is harmless and is retried by the next publication.
func (r *Repository) tidyOperation(t *tree, operationID string) {
	if operationID == "" {
		return
	}
	_ = t.removeAll(layout.StagingPath(operationID))
	_ = t.removeAll(layout.OperationPath(operationID))
}

// resolveLocation decides where the payload lives. A resumed publication reuses the location in
// its journal; otherwise the key_pattern is rendered exactly once, here, from the Record's
// captured creation time. The result is persisted; it is never rendered again.
func (r *Repository) resolveLocation(t *tree, job *publishJob, plan *publishPlan) error {
	rep, err := t.loadRepresentation(job.key)
	if err != nil {
		return err
	}
	if err := r.loadPendingState(t, plan, rep); err != nil {
		return err
	}
	if plan.admission.Resuming() && plan.oldOp != nil && plan.oldOp.IntentDigest == job.prepared.IntentDigest && plan.oldOp.Packaging == job.prepared.Packaging {
		job.location = plan.oldOp.Location
		return nil
	}
	if plan.oldOp != nil {
		plan.abandoned = plan.oldOp.Location
	}
	job.location, err = layout.RenderLocation(r.keyPattern, &layout.PatternInput{
		Name: job.record.Name, Version: job.record.Version, Kind: job.record.Kind, CreatedAt: job.record.CreatedAt.Time(),
	}, job.prepared.Packaging)
	return err
}

// loadPendingState records the locations this reference already owns and the content a
// publication replaces. The interrupted operation's journal is read when one is pending.
func (r *Repository) loadPendingState(t *tree, plan *publishPlan, rep *layout.Representation) error {
	if rep != nil {
		plan.owned = append(plan.owned, rep.Location)
	}
	if plan.current != nil && !plan.current.Available() {
		oldOp, err := t.loadOperation(plan.current.OperationID)
		if err != nil {
			return err
		}
		plan.oldOp = oldOp
	}
	plan.prior = priorState(plan, rep)
	if plan.oldOp != nil {
		plan.owned = append(plan.owned, plan.oldOp.Location)
		if plan.oldOp.Prior != nil {
			plan.owned = append(plan.owned, plan.oldOp.Prior.Location)
		}
	}
	return nil
}

// priorState describes the content a publication replaces, so a later cleanup can retire it.
func priorState(plan *publishPlan, rep *layout.Representation) *layout.Prior {
	if plan.admission == layout.AdmitReplace {
		prior := &layout.Prior{RecordDigest: plan.current.RecordDigest}
		if rep != nil && rep.RecordDigest == prior.RecordDigest {
			prior.Location = rep.Location
		}
		return prior
	}
	if !plan.admission.Resuming() {
		return nil
	}
	if plan.oldOp != nil && plan.oldOp.Prior != nil {
		prior := *plan.oldOp.Prior
		return &prior
	}
	if plan.current.PriorRecordDigest == "" {
		return nil
	}
	prior := &layout.Prior{RecordDigest: plan.current.PriorRecordDigest}
	if rep != nil && rep.RecordDigest == prior.RecordDigest {
		prior.Location = rep.Location
	}
	return prior
}

// begin reserves the location, writes the operation journal, and marks the reference pending.
// It holds the repository-wide claims lock so two references can never both reserve overlapping
// locations, and it never changes a payload path.
func (r *Repository) begin(ctx context.Context, t *tree, job *publishJob, plan *publishPlan) (*layout.Reference, error) {
	claims, err := r.acquire(ctx, claimsResource, r.settings.owner, job.opID)
	if err != nil {
		return nil, err
	}
	pending, beginErr := r.reserve(t, job, plan)
	if releaseErr := claims.Release(context.WithoutCancel(ctx)); beginErr == nil {
		beginErr = releaseErr
	}
	if beginErr != nil {
		return nil, beginErr
	}
	if plan.oldOp != nil && plan.oldOp.OperationID != job.opID {
		r.tidyOperation(t, plan.oldOp.OperationID)
	}
	return pending, r.settings.fault(faultBeforePayload)
}

// reserve checks for location collisions and unmanaged content, then records the journal and the
// pending reference.
func (r *Repository) reserve(t *tree, job *publishJob, plan *publishPlan) (*layout.Reference, error) {
	if err := r.checkCollision(t, job, plan); err != nil {
		return nil, err
	}
	journal := &layout.Operation{
		SchemaVersion: layout.SchemaVersion, RepositoryKind: r.kind, OperationID: job.opID,
		Name: job.record.Name, Version: job.record.Version, IntentDigest: job.prepared.IntentDigest,
		RecordDigest: job.recordDigest, CapturedAt: job.record.CreatedAt, Packaging: job.prepared.Packaging,
		Location: job.location, Prior: plan.prior, Payload: job.payload, Documents: job.documents, Owner: r.settings.owner,
	}
	if plan.expected != nil {
		journal.ExpectedPrior = plan.expected.ResolvedDigest
	}
	if err := t.writeOperation(journal); err != nil {
		return nil, fmt.Errorf("%w: record operation %s: %w", errUtils.ErrArtifactUploadFailed, job.opID, err)
	}
	pending := layout.NewPendingReference(job.record.Name, job.record.Version, job.opID, job.prepared.IntentDigest, plan.current)
	if err := t.writeReference(&pending); err != nil {
		return nil, fmt.Errorf("%w: mark %s:%s pending: %w", errUtils.ErrArtifactUploadFailed, job.record.Name, job.record.Version, err)
	}
	return &pending, nil
}

// build stages, writes, installs, and completes. It is the part of the sequence after the pending
// state is durable.
func (r *Repository) build(ctx context.Context, t *tree, job *publishJob, plan *publishPlan, pending *layout.Reference) (*artifact.PublishResult, error) {
	if err := t.removeAll(job.staging); err != nil {
		return nil, fmt.Errorf("%w: clear staging: %w", errUtils.ErrArtifactUploadFailed, err)
	}
	if err := r.store.stage(ctx, t, job); err != nil {
		return nil, wrapWrite("stage", err)
	}
	if err := t.writeRecord(job.recordDigest, job.recordBytes); err != nil {
		return nil, wrapWrite("write record", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	job.installing = true
	if err := r.store.install(t, job); err != nil {
		return nil, wrapWrite("install payload", err)
	}
	rep := r.representation(job)
	if err := t.writeRepresentation(rep); err != nil {
		return nil, wrapWrite("write representation", err)
	}
	if err := r.settings.fault(faultAfterMetadata); err != nil {
		return nil, err
	}
	completed := pending.Completed(job.recordDigest)
	if err := t.writeReference(&completed); err != nil {
		return nil, wrapWrite("complete reference", err)
	}
	if err := r.settings.fault(faultAfterCommit); err != nil {
		return nil, err
	}
	r.retire(t, job, plan)
	return r.result(t, job.record, job.recordDigest, rep, plan.admission.Outcome()), nil
}

// wrapWrite classifies a storage failure. Context errors and already-typed artifact errors pass
// through so callers keep matching on them.
func wrapWrite(step string, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || isArtifactError(err) {
		return err
	}
	return fmt.Errorf("%w: %s: %w", errUtils.ErrArtifactUploadFailed, step, err)
}

func isArtifactError(err error) bool {
	for _, sentinel := range []error{
		errUtils.ErrArtifactConflict, errUtils.ErrArtifactUnsupportedCapability, errUtils.ErrArtifactIntegrityFailed,
		errUtils.ErrArtifactPackagingInvalid, errUtils.ErrArtifactUnsafePath, errUtils.ErrArtifactTooLarge,
		errUtils.ErrArtifactPublicationUnavailable, errUtils.ErrArtifactRecordInvalid,
	} {
		if errors.Is(err, sentinel) {
			return true
		}
	}
	return isCrash(err)
}

// representation builds the representation record for the job.
func (r *Repository) representation(job *publishJob) *layout.Representation {
	return &layout.Representation{
		SchemaVersion: layout.SchemaVersion, RepositoryKind: r.kind, Name: job.record.Name, Version: job.record.Version,
		RecordDigest: job.recordDigest, OperationID: job.opID, Packaging: job.prepared.Packaging, Location: job.location,
		Payload: job.payload, Documents: job.documents, RepresentationDigest: job.representationDigest(job.prepared.Packaging),
	}
}

// retire removes what the completed publication replaced and the temporary resources it owned.
// Everything here is best-effort: the reference is already complete, and a leftover is garbage,
// not an inconsistency.
func (r *Repository) retire(t *tree, job *publishJob, plan *publishPlan) {
	_ = t.removeAll(job.staging)
	if plan.abandoned != "" && layout.FoldPath(plan.abandoned) != layout.FoldPath(job.location) {
		r.removePayload(t, plan.abandoned)
	}
	if plan.prior != nil {
		if plan.prior.Location != "" && layout.FoldPath(plan.prior.Location) != layout.FoldPath(job.location) {
			r.removePayload(t, plan.prior.Location)
		}
		if plan.prior.RecordDigest != "" && plan.prior.RecordDigest != job.recordDigest {
			_ = t.removeAll(layout.RecordPath(plan.prior.RecordDigest))
			_ = t.removeAll(layout.SignatureDir(artifact.SubjectRecord, plan.prior.RecordDigest))
		}
	}
	_ = t.removeAll(layout.OperationPath(job.opID))
}

// removePayload deletes the payload paths this repository owns at a location it no longer uses.
func (r *Repository) removePayload(t *tree, location string) {
	for _, owned := range layout.PayloadPaths(r.kind, location) {
		_ = t.removeAll(owned)
	}
	t.removeEmptyParents(location)
}

// result builds the publish result for a stored artifact.
func (r *Repository) result(t *tree, record *artifact.Record, recordDigest digest.Digest, rep *layout.Representation, outcome artifact.Outcome) *artifact.PublishResult {
	locator := t.absolute(rep.Location)
	return &artifact.PublishResult{
		Outcome:        outcome,
		Pinned:         r.pinned(artifact.Ref{Name: record.Name, Version: record.Version}, record, recordDigest),
		Representation: rep.Artifact(locator),
		Location:       locator,
		LockScope:      artifact.LockScopeHost,
	}
}
