package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
)

// target is the repository a mutation writes to.
type target struct {
	alias string
	repo  artifact.Repository
	info  *RepositoryInfo
}

// incoming describes the artifact being written, for comparison and reconciliation.
type incoming struct {
	ref          artifact.Ref
	record       *artifact.Record
	intent       digest.Digest
	recordDigest digest.Digest
	payload      []ocispec.Descriptor
	size         int64
	prepared     *artifact.PreparedArtifact
	inspection   *artifact.Inspection
}

func newIncoming(prepared *artifact.PreparedArtifact) (*incoming, error) {
	recordDigest, err := prepared.Record.RecordDigest()
	if err != nil {
		return nil, err
	}
	in := &incoming{ref: artifact.Ref{Name: prepared.Record.Name, Version: prepared.Record.Version}, record: &prepared.Record, intent: prepared.IntentDigest, recordDigest: recordDigest, prepared: prepared}
	for index := range prepared.Blobs {
		if prepared.Blobs[index].Role == artifact.BlobPayload {
			in.payload = append(in.payload, prepared.Blobs[index].Descriptor)
			in.size += prepared.Blobs[index].Descriptor.Size
		}
	}
	return in, nil
}

// current is what a reference holds right now.
type current struct {
	pinned      *artifact.PinnedReference
	inspection  *artifact.Inspection
	unavailable bool
}

type verdict int

const (
	verdictCreate verdict = iota
	verdictReplace
	verdictUnchanged
)

// writer performs one repository write attempt.
type writer func(ctx context.Context, options artifact.PublishOptions) (*artifact.PublishResult, error)

// publishState is filled by commit for the caller's outcome.
type publishState struct {
	record   artifact.Record
	existing bool
}

// publication bundles everything one write needs: the operation, the target repository, the
// incoming artifact, the force flag, the write closure, and the state commit fills in.
type publication struct {
	op    *operation
	tgt   *target
	in    *incoming
	force bool
	write writer
	state *publishState
}

// commit runs the shared publication sequence under the writer lock: read the current reference,
// compare intent, apply conflict and immutability rules, then write with reconciliation.
func (s *Service) commit(ctx context.Context, pub *publication) (err error) {
	ref := pub.in.ref
	pub.op.result.Ref = ref.String()
	locks, err := s.acquireLocks(ctx, pub.op, opPublish, []lockTarget{{alias: pub.tgt.alias, repo: pub.tgt.repo, info: pub.tgt.info, resource: ref.String()}})
	if err != nil {
		return err
	}
	s.event(ctx, pub, PhaseLocked, 0, nil)
	defer func() {
		_ = locks.release(ctx, pub.op)
		s.event(ctx, pub, PhaseReleased, 0, nil)
		if err != nil {
			s.event(ctx, pub, PhaseFailed, 0, err)
		}
	}()

	cur, err := s.readCurrent(ctx, pub.tgt, ref)
	if err != nil {
		return err
	}
	decision, err := s.decide(pub.tgt, cur, pub.in, pub.force)
	if err != nil {
		return err
	}
	if decision == verdictUnchanged {
		s.reportUnchanged(pub, cur)
		return nil
	}
	result, err := s.execute(ctx, pub, cur, decision, locks.handleFor(pub.tgt.alias))
	if err != nil {
		return err
	}
	s.reportWritten(pub, result, locks, decision, cur)
	return nil
}

func (s *Service) event(ctx context.Context, pub *publication, phase JournalPhase, attempt int, err error) {
	s.record(ctx, &JournalEntry{
		OperationID: pub.op.id, Phase: phase, Repository: pub.tgt.alias, Ref: pub.in.ref.String(),
		IntentDigest: pub.in.intent.String(), Attempt: attempt, Err: err,
	})
}

// readCurrent resolves and inspects the reference once. A missing reference is not an error; a
// reference that exists but is unavailable (pending or interrupted) is reported so the same-intent
// retry can repair it.
func (s *Service) readCurrent(ctx context.Context, tgt *target, ref artifact.Ref) (*current, error) {
	pinned, err := tgt.repo.Resolve(ctx, ref)
	switch {
	case errors.Is(err, errUtils.ErrArtifactNotFound):
		return &current{}, nil
	case errors.Is(err, errUtils.ErrArtifactPublicationUnavailable):
		return &current{unavailable: true}, nil
	case err != nil:
		return nil, fmt.Errorf("resolve %s in repository %q: %w", ref, tgt.alias, err)
	}
	inspection, err := tgt.repo.Inspect(ctx, pinned)
	if err != nil {
		return nil, fmt.Errorf("inspect %s in repository %q: %w", ref, tgt.alias, err)
	}
	return &current{pinned: &pinned, inspection: inspection}, nil
}

// decide applies the intent comparison, conflict, force, and immutability rules.
func (s *Service) decide(tgt *target, cur *current, in *incoming, force bool) (verdict, error) {
	if cur.pinned == nil {
		return verdictCreate, nil
	}
	existing := cur.inspection.Record
	if existing != nil {
		if intent, err := existing.IntentDigest(); err == nil && intent == in.intent {
			return verdictUnchanged, nil
		}
	}
	if immutable, reason := s.immutableCandidate(tgt, cur, in); immutable {
		return verdictCreate, fmt.Errorf("%w: %s stays immutable even with force", errUtils.ErrArtifactConflict, reason)
	}
	if !force {
		return verdictCreate, fmt.Errorf("%w: %s already exists in repository %q with different %s; use force to replace it",
			errUtils.ErrArtifactConflict, in.ref, tgt.alias, describeDifferences(existing, in.record))
	}
	return verdictReplace, nil
}

// immutableCandidate asks the admission policy about both the stored and the incoming artifact.
func (s *Service) immutableCandidate(tgt *target, cur *current, in *incoming) (bool, string) {
	if s.immutable == nil {
		return false, ""
	}
	if immutable, why := s.immutable.Immutable(Candidate{Existing: true, Repository: tgt.alias, Inspection: cur.inspection}); immutable {
		return true, "the existing release (" + why + ")"
	}
	if immutable, why := s.immutable.Immutable(Candidate{Repository: tgt.alias, Inspection: in.inspection, Prepared: in.prepared}); immutable {
		return true, "the incoming release (" + why + ")"
	}
	return false, ""
}

// describeDifferences names which parts of identity changed.
func describeDifferences(existing, incoming *artifact.Record) string {
	if existing == nil || incoming == nil {
		return "content (the existing artifact carries no portable record)"
	}
	var differences []string
	if existing.ContentDigest != incoming.ContentDigest {
		differences = append(differences, "content (files, modes, or documents)")
	}
	if existing.Metadata.Description != incoming.Metadata.Description {
		differences = append(differences, "description")
	}
	if !labelsEqual(existing.Metadata.Labels, incoming.Metadata.Labels) {
		differences = append(differences, "labels")
	}
	if existing.Kind != incoming.Kind || string(existing.KindData) != string(incoming.KindData) {
		differences = append(differences, "kind data")
	}
	if len(differences) == 0 {
		return "intent"
	}
	return strings.Join(differences, ", ")
}

func labelsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for key, value := range a {
		if other, ok := b[key]; !ok || other != value {
			return false
		}
	}
	return true
}
