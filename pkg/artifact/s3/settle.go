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
	if err := r.retireOwed(ctx, journal, record, rep, protected...); err != nil {
		return err
	}
	r.tidyOperation(ctx, journal.OperationID)
	return nil
}

// retireOwed retires what the journal owes, keeping the live payload and never retiring the Records
// listed. It leaves the journal in place.
func (r *Repository) retireOwed(ctx context.Context, journal *layout.Operation, record *artifact.Record, rep *layout.Representation, protected ...digest.Digest) error {
	keep, err := setOfRecord(record).keys(rep.Packaging, rep.Location)
	if err != nil {
		return err
	}
	return r.retireJournaled(ctx, journal, keep, protected...)
}

// settleReplaced settles the journal of the completed reference a forced replacement is about to
// replace. The reference may be what a repair replaces, so a representation that no longer matches
// is not an obstacle: the journal then names the live location.
//
// When the journal is the only record of the live location, it is the one thing the settlement must
// not remove: the representation that would name the location is missing or describes another
// Record. The settlement then retires what the journal owes, keeps the journal, and returns the
// location it recovered. The replacement carries it into its own journal (see resolveLocation) and
// removes the settled journal only once that journal is durable (see begin). A crash anywhere in
// between leaves the journal, and so the location, where the retry finds it. A representation that
// names the live payload needs no recovery: the settlement returns nil and removes the journal.
func (r *Repository) settleReplaced(ctx context.Context, current *layout.Reference) (*layout.Representation, error) {
	if current.OperationID == "" {
		return nil, nil
	}
	journal, err := r.loadOperation(ctx, current.OperationID)
	if err != nil {
		return nil, err
	}
	live, recovered, err := r.locateLive(ctx, current, journal)
	if err != nil {
		return nil, err
	}
	if owes(journal) {
		if err := r.retireLive(ctx, current, journal, live); err != nil {
			return nil, err
		}
	}
	if recovered == nil {
		r.tidyOperation(ctx, current.OperationID)
	}
	return recovered, nil
}

// locateLive finds where the completed reference's payload lives. The stored representation names
// it when it describes the reference's Record; otherwise the journal that wrote the payload does,
// and that representation is also returned as recovered. Neither locates it when both are nil.
func (r *Repository) locateLive(ctx context.Context, current *layout.Reference, journal *layout.Operation) (live, recovered *layout.Representation, err error) {
	rep, err := r.loadRepresentation(ctx, current.Key())
	if err != nil {
		return nil, nil, err
	}
	if rep != nil && rep.RecordDigest == current.RecordDigest {
		return rep, nil, nil
	}
	recovered = recoveredRepresentation(current, journal)
	return recovered, recovered, nil
}

// retireLive retires what the journal owes, keeping the live payload.
func (r *Repository) retireLive(ctx context.Context, current *layout.Reference, journal *layout.Operation, live *layout.Representation) error {
	record, _, err := r.loadRecord(ctx, current.RecordDigest)
	if err != nil {
		return err
	}
	if live == nil {
		return unknownLocation(current)
	}
	if err := r.retireOwed(ctx, journal, record, live, current.RecordDigest); err != nil {
		return fmt.Errorf("finish the cleanup owed by operation %s before replacing %s:%s: %w", current.OperationID, current.Name, current.Version, err)
	}
	return nil
}

// recoveredRepresentation describes the payload a completed reference points at from the journal
// that wrote it, or returns nil when the journal is missing or wrote another Record. The journal
// lists the payload and the documents, so the recovered representation owns exactly the keys the
// stored one did.
func recoveredRepresentation(current *layout.Reference, journal *layout.Operation) *layout.Representation {
	if journal == nil || journal.RecordDigest != current.RecordDigest || journal.Location == "" {
		return nil
	}
	return &layout.Representation{
		Name: current.Name, Version: current.Version, RecordDigest: current.RecordDigest, OperationID: journal.OperationID,
		Packaging: journal.Packaging, Location: journal.Location, Payload: journal.Payload, Documents: journal.Documents,
	}
}

// unknownLocation is the failure for a completed reference whose payload location no record names.
func unknownLocation(current *layout.Reference) error {
	return fmt.Errorf("%w: %s:%s is completed but its payload location is unknown", errUtils.ErrArtifactIntegrityFailed, current.Name, current.Version)
}
