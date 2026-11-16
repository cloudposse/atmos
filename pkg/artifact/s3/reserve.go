package s3

import (
	"context"
	"fmt"
	"slices"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/blob"
	"github.com/cloudposse/atmos/pkg/artifact/layout"
)

// claimsResource is the repository-wide lock that serializes location reservations. It starts with
// a NUL byte, which no artifact reference can contain, so it can never collide with a reference
// lock. It is held only while collisions are checked and the pending state is written.
const claimsResource = "\x00claims"

// begin reserves the location, writes the operation journal, and marks the reference pending
// through a conditional write. It holds the repository-wide claims lock so two references can never
// both reserve overlapping locations, and it never changes a payload object.
func (r *Repository) begin(ctx context.Context, job *publishJob, plan *publishPlan) (*refState, error) {
	claims, err := r.acquire(ctx, claimsResource, r.settings.owner, job.opID)
	if err != nil {
		return nil, err
	}
	pending, beginErr := r.reserve(ctx, job, plan)
	beginErr = joinRelease(beginErr, claims.Release(ctx))
	if beginErr != nil {
		return nil, beginErr
	}
	if plan.oldOp != nil && plan.oldOp.OperationID != job.opID {
		r.tidyOperation(ctx, plan.oldOp.OperationID)
	}
	if plan.settled != "" && plan.settled != job.opID {
		// The new journal now names the replaced payload's location (Prior), so the journal that
		// held the only other record of it has nothing left to carry.
		r.tidyOperation(ctx, plan.settled)
	}
	return pending, r.settings.fault(faultBeforePayload)
}

// reserve checks for location collisions and unmanaged content, then records the journal and the
// pending reference. The pending reference is created with If-None-Match, or replaces the observed
// state with If-Match, so a concurrent writer to the same reference loses with a conflict.
func (r *Repository) reserve(ctx context.Context, job *publishJob, plan *publishPlan) (*refState, error) {
	if err := r.checkCollision(ctx, job, plan); err != nil {
		return nil, err
	}
	journal := &layout.Operation{
		SchemaVersion: layout.SchemaVersion, RepositoryKind: artifact.RepositoryKindS3, OperationID: job.opID,
		Name: job.record.Name, Version: job.record.Version, IntentDigest: job.prepared.IntentDigest,
		RecordDigest: job.recordDigest, CapturedAt: job.record.CreatedAt, Packaging: job.prepared.Packaging,
		Location: job.location, Prior: plan.prior, Abandoned: retiredPayloads(plan.abandoned), Payload: job.payload, Documents: job.documents, Owner: r.settings.owner,
	}
	if plan.expected != nil {
		journal.ExpectedPrior = plan.expected.ResolvedDigest
	}
	if err := r.writeOperation(ctx, journal); err != nil {
		return nil, wrapWrite(fmt.Sprintf("record operation %s", job.opID), err)
	}
	var prior *layout.Reference
	cond := blob.IfAbsent()
	if plan.current != nil {
		prior, cond = plan.current.ref, blob.IfMatch(plan.current.etag)
	}
	pending := layout.NewPendingReference(job.record.Name, job.record.Version, job.opID, job.prepared.IntentDigest, prior)
	etag, err := r.writeReference(ctx, &pending, cond)
	if err != nil {
		return nil, wrapWrite(fmt.Sprintf("mark %s:%s pending", job.record.Name, job.record.Version), err)
	}
	return &refState{ref: &pending, etag: etag}, nil
}

// checkCollision rejects a location that another reference owns or that holds objects this
// repository does not manage. A second reference with a colliding key_pattern therefore fails with
// a conflict and is never bound to the first reference's payload, and a user's own objects at the
// location are never overwritten. The caller holds the claims lock, so no other publication can
// reserve between the check and the journal write.
func (r *Repository) checkCollision(ctx context.Context, job *publishJob, plan *publishPlan) error {
	claims, err := r.collectClaims(ctx, job.key)
	if err != nil {
		return err
	}
	candidate := layout.Claim{
		ReferenceKey: job.key, Name: job.record.Name, Version: job.record.Version,
		OperationID: job.opID, Paths: claimPaths(job.prepared.Packaging, job.location),
	}
	if other := layout.FindConflict(&candidate, claims); other != nil {
		return fmt.Errorf("%w: location %q for %s:%s is already claimed by %s:%s%s; give the repository a key_pattern that keeps references apart",
			errUtils.ErrArtifactConflict, job.location, job.record.Name, job.record.Version, other.Name, other.Version, pendingSuffix(other))
	}
	return r.checkUnmanaged(ctx, job, &candidate, plan)
}

func pendingSuffix(claim *layout.Claim) string {
	if claim.OperationID == "" {
		return ""
	}
	return " (pending operation " + claim.OperationID + ")"
}

// collectClaims lists the paths owned by every other reference: the representation of each
// completed reference and the journal of each pending one.
func (r *Repository) collectClaims(ctx context.Context, ownKey string) ([]layout.Claim, error) {
	refs, err := r.listReferences(ctx)
	if err != nil {
		return nil, err
	}
	var others []*layout.Reference
	for _, state := range refs {
		if state.ref.Key() != ownKey {
			others = append(others, state.ref)
		}
	}
	found := make([]*layout.Claim, len(others))
	err = r.forEach(ctx, len(others), func(ctx context.Context, index int) error {
		claim, claimErr := r.claimOf(ctx, others[index])
		found[index] = claim
		return claimErr
	})
	if err != nil {
		return nil, err
	}
	var claims []layout.Claim
	for _, claim := range found {
		if claim != nil {
			claims = append(claims, *claim)
		}
	}
	return claims, nil
}

// claimOf returns the claim a reference holds: its representation when completed (or its journal,
// when that is the only record of the live payload), its journal when pending, and in both states
// the locations its journal still awaits removing. A reference whose state is missing holds no claim.
func (r *Repository) claimOf(ctx context.Context, ref *layout.Reference) (*layout.Claim, error) {
	base := layout.Claim{ReferenceKey: ref.Key(), Name: ref.Name, Version: ref.Version}
	if ref.Available() {
		return r.completedClaim(ctx, ref, &base)
	}
	op, err := r.loadOperation(ctx, ref.OperationID)
	if err != nil {
		return nil, err
	}
	if op == nil {
		// The reference was listed as pending, but a missing journal is also what a completed reference
		// leaves behind once its cleanup finished. Its current state decides: a completed reference still
		// claims its location, so an overlapping reservation cannot take it.
		return r.claimOfCurrent(ctx, ref, &base)
	}
	base.OperationID = op.OperationID
	base.Paths = append(claimPaths(op.Packaging, op.Location), journalClaimPaths(op)...)
	return &base, nil
}

// claimOfCurrent reloads a reference whose journal is missing and derives its claim from the state it has
// now. A reference that is gone, or still pending without a journal, holds no claim.
func (r *Repository) claimOfCurrent(ctx context.Context, listed *layout.Reference, base *layout.Claim) (*layout.Claim, error) {
	current, err := r.loadReference(ctx, listed.Key())
	if err != nil || current == nil || !current.ref.Available() {
		return nil, err
	}
	return r.completedClaim(ctx, current.ref, base)
}

// completedClaim returns the claim of a completed reference: the location its representation names,
// the location its journal records when the representation does not name the live payload (the
// journal is then the only record of it), and the locations the journal still awaits removing. A
// completed reference with none of them holds no claim.
func (r *Repository) completedClaim(ctx context.Context, ref *layout.Reference, base *layout.Claim) (*layout.Claim, error) {
	rep, err := r.loadRepresentation(ctx, ref.Key())
	if err != nil {
		return nil, err
	}
	// A completed reference keeps its journal until the cleanup that follows completion has
	// removed everything the journal still lists.
	journal, err := r.loadOperation(ctx, ref.OperationID)
	if err != nil {
		return nil, err
	}
	if rep != nil {
		base.Paths = claimPaths(rep.Packaging, rep.Location)
	}
	if recovered := recoveredRepresentation(ref, journal); recovered != nil && (rep == nil || rep.RecordDigest != ref.RecordDigest) {
		base.Paths = append(base.Paths, claimPaths(recovered.Packaging, recovered.Location)...)
	}
	base.Paths = append(base.Paths, journalClaimPaths(journal)...)
	if len(base.Paths) == 0 {
		return nil, nil
	}
	return base, nil
}

// journalClaimPaths lists the paths of every payload a journal still awaits removing: the replaced
// payload (Prior) and the payloads of abandoned attempts. They stay claimed until the journal is
// retired, even while they hold no objects, because the retirement deletes by the journaled keys
// and does not check who owns the location by then.
func journalClaimPaths(op *layout.Operation) []string {
	if op == nil {
		return nil
	}
	var paths []string
	if op.Prior != nil && op.Prior.Location != "" {
		paths = append(paths, anyShapeClaimPaths(op.Prior.Packaging, op.Prior.Location)...)
	}
	for index := range op.Abandoned {
		paths = append(paths, anyShapeClaimPaths(op.Abandoned[index].Packaging, op.Abandoned[index].Location)...)
	}
	return paths
}

// checkUnmanaged fails when an object the publication would overwrite exists and this reference
// does not own it. A location no reference owns must hold nothing at all. An owned location is owned
// only through the exact keys its payloads consist of, so a user's object below it is never
// overwritten: every key the publication writes there must be one the reference already owns, or
// be absent. It runs before the journal and any payload object are written.
func (r *Repository) checkUnmanaged(ctx context.Context, job *publishJob, candidate *layout.Claim, plan *publishPlan) error {
	ownedPaths, ownedKeys, err := ownership(plan.owned)
	if err != nil {
		return err
	}
	for _, path := range candidate.Paths {
		if slices.Contains(ownedPaths, layout.FoldPath(path)) {
			continue
		}
		if err := r.requireUnoccupied(ctx, path); err != nil {
			return err
		}
	}
	return r.checkWriteKeys(ctx, job, ownedPaths, ownedKeys)
}

// ownership lists the paths the owned payloads claim, in folded form, and the exact object keys they
// consist of.
func ownership(owned []ownedPayload) (paths, keys []string, err error) {
	for index := range owned {
		for _, path := range anyShapeClaimPaths(owned[index].packaging, owned[index].location) {
			paths = append(paths, layout.FoldPath(path))
		}
		payloadKeys, keyErr := owned[index].set.anyShapeKeys(owned[index].packaging, owned[index].location)
		if keyErr != nil {
			return nil, nil, keyErr
		}
		keys = append(keys, payloadKeys...)
	}
	return paths, keys, nil
}

// checkWriteKeys requires every key the publication writes below an owned path to be one the
// reference owns or to be absent. Keys below a path no reference owns are covered by the whole-path
// check.
func (r *Repository) checkWriteKeys(ctx context.Context, job *publishJob, ownedPaths, ownedKeys []string) error {
	items, err := job.plan()
	if err != nil {
		return err
	}
	for index := range items {
		key := items[index].place(job.location)
		if slices.Contains(ownedKeys, key) || !coveredBy(ownedPaths, key) {
			continue
		}
		if err := r.requireUnoccupied(ctx, key); err != nil {
			return err
		}
	}
	return nil
}

// coveredBy reports whether an owned path contains the key. Owned paths are in folded form,
// matching how claims are compared.
func coveredBy(ownedPaths []string, key string) bool {
	folded := layout.FoldPath(key)
	return slices.ContainsFunc(ownedPaths, func(path string) bool { return folded == path || strings.HasPrefix(folded, path+"/") })
}

// requireUnoccupied fails with a conflict when the path holds an object, or objects below it.
func (r *Repository) requireUnoccupied(ctx context.Context, path string) error {
	occupied, err := r.occupied(ctx, path)
	if err != nil {
		return err
	}
	if occupied {
		return fmt.Errorf("%w: %q already holds objects that are not managed by this repository; remove them or choose another key_pattern", errUtils.ErrArtifactConflict, path)
	}
	return nil
}

// occupied reports whether an object, or any object below the path as a prefix, exists.
func (r *Repository) occupied(ctx context.Context, path string) (bool, error) {
	info, err := r.stat(ctx, path)
	if err != nil || info != nil {
		return info != nil, err
	}
	return r.hasAny(ctx, path+"/")
}
