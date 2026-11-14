package s3

import (
	"context"
	"fmt"
	"slices"

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
	if releaseErr := claims.Release(ctx); beginErr == nil {
		beginErr = releaseErr
	}
	if beginErr != nil {
		return nil, beginErr
	}
	if plan.oldOp != nil && plan.oldOp.OperationID != job.opID {
		r.tidyOperation(ctx, plan.oldOp.OperationID)
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
		Location: job.location, Prior: plan.prior, Payload: job.payload, Documents: job.documents, Owner: r.settings.owner,
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
	return r.checkUnmanaged(ctx, &candidate, plan)
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

// claimOf returns the claim a reference holds: its representation when completed, its journal when
// pending. A reference whose state is missing holds no claim.
func (r *Repository) claimOf(ctx context.Context, ref *layout.Reference) (*layout.Claim, error) {
	base := layout.Claim{ReferenceKey: ref.Key(), Name: ref.Name, Version: ref.Version}
	if ref.Available() {
		rep, err := r.loadRepresentation(ctx, ref.Key())
		if err != nil || rep == nil {
			return nil, err
		}
		base.Paths = claimPaths(rep.Packaging, rep.Location)
		return &base, nil
	}
	op, err := r.loadOperation(ctx, ref.OperationID)
	if err != nil || op == nil {
		return nil, err
	}
	base.OperationID = op.OperationID
	base.Paths = claimPaths(op.Packaging, op.Location)
	return &base, nil
}

// checkUnmanaged fails when a path the publication would write already holds objects and this
// reference does not own it.
func (r *Repository) checkUnmanaged(ctx context.Context, candidate *layout.Claim, plan *publishPlan) error {
	var owned []string
	for index := range plan.owned {
		for _, path := range claimPaths(plan.owned[index].packaging, plan.owned[index].location) {
			owned = append(owned, layout.FoldPath(path))
		}
		// A prior written before packaging was recorded may be either shape.
		if plan.owned[index].packaging == "" {
			for _, packaging := range []artifact.Packaging{artifact.PackagingNone, artifact.PackagingTgz} {
				for _, path := range claimPaths(packaging, plan.owned[index].location) {
					owned = append(owned, layout.FoldPath(path))
				}
			}
		}
	}
	for _, path := range candidate.Paths {
		if slices.Contains(owned, layout.FoldPath(path)) {
			continue
		}
		occupied, err := r.occupied(ctx, path)
		if err != nil {
			return err
		}
		if occupied {
			return fmt.Errorf("%w: %q already holds objects that are not managed by this repository; remove them or choose another key_pattern", errUtils.ErrArtifactConflict, path)
		}
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
