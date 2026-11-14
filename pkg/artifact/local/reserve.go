package local

import (
	"fmt"
	"slices"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact/layout"
)

// checkCollision rejects a location that another reference owns or that holds content this
// repository does not manage. A second reference with a colliding key_pattern therefore fails
// with a conflict and is never bound to the first reference's payload, and a user's own files at
// the location are never overwritten. The caller holds the claims lock, so no other publication
// can reserve between the check and the journal write.
func (r *Repository) checkCollision(t *tree, job *publishJob, plan *publishPlan) error {
	claims, err := r.collectClaims(t, job.key)
	if err != nil {
		return err
	}
	candidate := layout.Claim{
		ReferenceKey: job.key, Name: job.record.Name, Version: job.record.Version,
		OperationID: job.opID, Paths: layout.PayloadPaths(r.kind, job.location),
	}
	if other := layout.FindConflict(&candidate, claims); other != nil {
		return fmt.Errorf("%w: location %q for %s:%s is already claimed by %s:%s%s; give the repository a key_pattern that keeps references apart",
			errUtils.ErrArtifactConflict, job.location, job.record.Name, job.record.Version, other.Name, other.Version, pendingSuffix(other))
	}
	return r.checkUnmanaged(t, &candidate, plan)
}

func pendingSuffix(claim *layout.Claim) string {
	if claim.OperationID == "" {
		return ""
	}
	return " (pending operation " + claim.OperationID + ")"
}

// collectClaims lists the paths owned by every other reference: the representation of each
// completed reference and the journal of each pending one.
func (r *Repository) collectClaims(t *tree, ownKey string) ([]layout.Claim, error) {
	refs, err := t.listReferences()
	if err != nil {
		return nil, err
	}
	var claims []layout.Claim
	for _, ref := range refs {
		if ref.Key() == ownKey {
			continue
		}
		if ref.Available() {
			rep, err := t.loadRepresentation(ref.Key())
			if err != nil {
				return nil, err
			}
			if rep != nil {
				claims = append(claims, rep.Claim())
			}
			continue
		}
		op, err := t.loadOperation(ref.OperationID)
		if err != nil {
			return nil, err
		}
		if op != nil {
			claims = append(claims, op.Claim())
		}
	}
	return claims, nil
}

// checkUnmanaged fails when a path the publication would write already exists and this reference
// does not own it.
func (r *Repository) checkUnmanaged(t *tree, candidate *layout.Claim, plan *publishPlan) error {
	var owned []string
	for _, location := range plan.owned {
		for _, path := range layout.PayloadPaths(r.kind, location) {
			owned = append(owned, layout.FoldPath(path))
		}
	}
	for _, path := range candidate.Paths {
		exists, err := t.exists(path)
		if err != nil {
			return fmt.Errorf("%w: inspect %q: %w", errUtils.ErrArtifactUploadFailed, path, err)
		}
		if exists && !slices.Contains(owned, layout.FoldPath(path)) {
			return fmt.Errorf("%w: %q already exists and is not managed by this repository; remove it or choose another key_pattern", errUtils.ErrArtifactConflict, path)
		}
	}
	return nil
}
