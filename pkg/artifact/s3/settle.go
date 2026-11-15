package s3

import (
	"context"
	"fmt"

	"github.com/opencontainers/go-digest"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/layout"
)

// A completed publication keeps its journal until the cleanup that follows completion has removed
// everything the journal still lists: the replaced Record with its payload and evidence (Prior),
// and the payloads of abandoned attempts. Until then the journal is the only record of that work,
// so whatever ends the journal must first finish the work or carry it on:
//
//   - the journal's locations stay claimed (see claimOf), so no other reference can reserve one;
//   - a same-intent republication retires it (see unchanged);
//   - a forced replacement and a deletion settle it before they write anything (see settle).
//
// Every removal is by exact journaled key and is idempotent, so a retirement that stops part way is
// finished by the next settlement, which removes each object once.

// retireJournaled removes what a completed operation's journal still lists for removal: the
// replaced Record and its payload, and the payloads of abandoned attempts. The keys of the live
// payload are kept, and the Records listed as protected are never retired.
func (r *Repository) retireJournaled(ctx context.Context, journal *layout.Operation, keep []string, protected ...digest.Digest) error {
	if journal == nil {
		return nil
	}
	if journal.Prior != nil {
		if err := r.retirePrior(ctx, journal.Prior, keep, protected...); err != nil {
			return err
		}
	}
	return r.removePayloads(ctx, carriedPayloads(journal), keep)
}

// owes reports whether a journal still lists cleanup.
func owes(journal *layout.Operation) bool {
	return journal != nil && (journal.Prior != nil || len(journal.Abandoned) > 0)
}

// settle finishes the cleanup a completed reference's journal still owes and then removes the
// journal. The live payload of the reference is kept; the Records it and the incoming publication
// use are never retired. A journal that owes nothing is left for its caller to tidy.
func (r *Repository) settle(ctx context.Context, current *layout.Reference, record *artifact.Record, rep *layout.Representation, incoming digest.Digest) error {
	if current.OperationID == "" {
		return nil
	}
	journal, err := r.loadOperation(ctx, current.OperationID)
	if err != nil || !owes(journal) {
		return err
	}
	return r.retireAndTidy(ctx, journal, record, rep, current.RecordDigest, incoming)
}

// retireAndTidy retires what the journal owes, keeping the live payload and never retiring the
// Records listed, and then removes the journal.
func (r *Repository) retireAndTidy(ctx context.Context, journal *layout.Operation, record *artifact.Record, rep *layout.Representation, protected ...digest.Digest) error {
	keep, err := setOfRecord(record).keys(rep.Packaging, rep.Location)
	if err != nil {
		return err
	}
	if err := r.retireJournaled(ctx, journal, keep, protected...); err != nil {
		return err
	}
	r.tidyOperation(ctx, journal.OperationID)
	return nil
}

// settleReplaced settles the journal of the completed reference a forced replacement is about to
// replace. The reference may be what a repair replaces, so a representation that no longer matches
// is not an obstacle: the journal then names the live location.
func (r *Repository) settleReplaced(ctx context.Context, current *layout.Reference) error {
	if current.OperationID == "" {
		return nil
	}
	journal, err := r.loadOperation(ctx, current.OperationID)
	if err != nil {
		return err
	}
	if !owes(journal) {
		r.tidyOperation(ctx, current.OperationID)
		return nil
	}
	record, _, err := r.loadRecord(ctx, current.RecordDigest)
	if err != nil {
		return err
	}
	rep, err := r.liveRepresentation(ctx, current, journal)
	if err != nil {
		return err
	}
	if err := r.retireAndTidy(ctx, journal, record, rep, current.RecordDigest); err != nil {
		return fmt.Errorf("finish the cleanup owed by operation %s before replacing %s:%s: %w", current.OperationID, current.Name, current.Version, err)
	}
	return nil
}

// liveRepresentation returns where the completed reference's payload lives: its representation, or
// the journal that wrote it when the representation is missing or describes another Record.
func (r *Repository) liveRepresentation(ctx context.Context, current *layout.Reference, journal *layout.Operation) (*layout.Representation, error) {
	rep, err := r.loadRepresentation(ctx, current.Key())
	if err != nil {
		return nil, err
	}
	if rep != nil && rep.RecordDigest == current.RecordDigest {
		return rep, nil
	}
	if journal.RecordDigest == current.RecordDigest && journal.Location != "" {
		return &layout.Representation{Packaging: journal.Packaging, Location: journal.Location}, nil
	}
	return nil, fmt.Errorf("%w: %s:%s is completed but its payload location is unknown", errUtils.ErrArtifactIntegrityFailed, current.Name, current.Version)
}
