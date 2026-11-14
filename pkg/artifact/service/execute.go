package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
)

// reconcileState is what a read of the completed reference proves.
type reconcileState int

const (
	// The reference could not be proven to hold or lack the expected content.
	reconcileUnknown reconcileState = iota

	// The completed reference holds exactly the expected content.
	reconcileCommitted

	// The reference still equals the observed prior state.
	reconcileNotCommitted
)

// execute writes with reconciliation. It never blind-retries: after an ambiguous failure it reads
// the completed reference and verifies the expected digests; only a proven not-committed state
// with a temporary error is retried, reusing the prepared artifact, operation ID, and time.
func (s *Service) execute(ctx context.Context, pub *publication, cur *current, decision verdict, lock artifact.LockHandle) (*artifact.PublishResult, error) {
	for attempt := 1; ; attempt++ {
		s.event(ctx, pub, PhasePending, attempt, nil)
		options := artifact.PublishOptions{ExpectedPrior: cur.pinned, Force: decision == verdictReplace, Lock: lock}
		result, err := pub.write(ctx, options)
		if err == nil {
			s.event(ctx, pub, PhaseCommitted, attempt, nil)
			return result, nil
		}
		if isDefinitive(err) {
			return nil, err
		}
		recovered, retry, failure := s.afterFailure(ctx, pub, cur, err, attempt)
		if !retry {
			return recovered, failure
		}
	}
}

// afterFailure decides what an ambiguous write failure means by reading the completed reference.
// It returns a recovered result when the write is proven committed, retry when it is proven
// uncommitted and worth repeating, and otherwise the error to report.
func (s *Service) afterFailure(ctx context.Context, pub *publication, cur *current, writeErr error, attempt int) (*artifact.PublishResult, bool, error) {
	reconcileCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	found := reconcile(reconcileCtx, pub.tgt, pub.in, cur)
	cancel()
	switch found.state {
	case reconcileCommitted:
		s.event(ctx, pub, PhaseReconciled, attempt, nil)
		pub.op.info("reconciled", fmt.Sprintf("the repository reported %q but the completed reference holds the expected digests; success was confirmed by reading it", writeErr))
		return syntheticResult(&found.pinned, found.inspection), false, nil
	case reconcileNotCommitted:
		if ctx.Err() != nil {
			return nil, false, errors.Join(writeErr, ctx.Err())
		}
		if !isTemporary(writeErr) || attempt >= s.attempts {
			return nil, false, writeErr
		}
		if sleepErr := s.sleep(ctx, s.backoff<<(attempt-1)); sleepErr != nil {
			return nil, false, errors.Join(writeErr, sleepErr)
		}
		return nil, true, nil
	case reconcileUnknown:
		fallthrough
	default:
		ambiguous := fmt.Errorf("%w: operation %s on %s in repository %q failed and could not be reconciled; check the reference before retrying", errUtils.ErrArtifactPublicationAmbiguous, pub.op.id, pub.in.ref, pub.tgt.alias)
		return nil, false, errors.Join(ambiguous, writeErr, found.err)
	}
}

// isDefinitive reports errors that prove nothing was committed, so reconciliation is unnecessary. A
// publication-unavailable rejection (a reference pending for another operation) is definitive even
// when the error also carries the ambiguous sentinel.
func isDefinitive(err error) bool {
	for _, sentinel := range []error{
		errUtils.ErrArtifactPublicationUnavailable,
		errUtils.ErrArtifactConflict, errUtils.ErrArtifactUnsupportedCapability, errUtils.ErrArtifactRecordInvalid,
		errUtils.ErrArtifactTooLarge, errUtils.ErrArtifactRefInvalid, errUtils.ErrArtifactUnsafePath,
		errUtils.ErrArtifactPackagingInvalid, errUtils.ErrArtifactConfigInvalid, errUtils.ErrArtifactVerificationFailed,
	} {
		if errors.Is(err, sentinel) {
			return true
		}
	}
	return false
}

// isTemporary reports errors that advertise themselves as retryable.
func isTemporary(err error) bool {
	var temporary interface{ Temporary() bool }
	return errors.As(err, &temporary) && temporary.Temporary()
}

// reconciliation is what reading the completed reference proved.
type reconciliation struct {
	state      reconcileState
	pinned     artifact.PinnedReference
	inspection *artifact.Inspection
	err        error
}

// reconcile reads the completed reference and classifies it against the expected content.
func reconcile(ctx context.Context, tgt *target, in *incoming, cur *current) reconciliation {
	ref := in.ref
	pinned, err := tgt.repo.Resolve(ctx, ref)
	switch {
	case errors.Is(err, errUtils.ErrArtifactNotFound):
		if cur.pinned == nil && !cur.unavailable {
			return reconciliation{state: reconcileNotCommitted}
		}
		return reconciliation{err: fmt.Errorf("the previously present reference %s is now absent: %w", ref, err)}
	case err != nil:
		return reconciliation{err: fmt.Errorf("reconcile %s: %w", ref, err)}
	}
	if in.recordDigest == "" {
		return reconcileGraph(ref, &pinned, in, cur)
	}
	if pinned.RecordDigest == in.recordDigest {
		return reconcileRecord(ctx, tgt, &pinned, in)
	}
	if cur.pinned != nil && pinned.ResolvedDigest == cur.pinned.ResolvedDigest {
		return reconciliation{state: reconcileNotCommitted, pinned: pinned}
	}
	return reconciliation{pinned: pinned, err: fmt.Errorf("%w: %s now resolves to %s, which is neither the prior state nor the expected content", errUtils.ErrArtifactConflict, ref, pinned.ResolvedDigest)}
}

// reconcileRecord inspects a reference whose Record digest matches the expected one and confirms
// that the stored content is the expected content.
func reconcileRecord(ctx context.Context, tgt *target, pinned *artifact.PinnedReference, in *incoming) reconciliation {
	inspection, err := tgt.repo.Inspect(ctx, *pinned)
	if err != nil {
		return reconciliation{pinned: *pinned, err: fmt.Errorf("reconcile %s: %w", in.ref, err)}
	}
	if matchErr := matchesExpected(in, inspection); matchErr != nil {
		return reconciliation{pinned: *pinned, inspection: inspection, err: matchErr}
	}
	return reconciliation{state: reconcileCommitted, pinned: *pinned, inspection: inspection}
}

// reconcileGraph classifies a native OCI graph without a Record. Empty record digests prove nothing,
// so success requires the destination root to be exactly the pinned source root.
func reconcileGraph(ref artifact.Ref, pinned *artifact.PinnedReference, in *incoming, cur *current) reconciliation {
	if in.rootDigest != "" && pinned.ResolvedDigest == in.rootDigest {
		return reconciliation{state: reconcileCommitted, pinned: *pinned, inspection: in.inspection}
	}
	if cur.pinned != nil && pinned.ResolvedDigest == cur.pinned.ResolvedDigest {
		return reconciliation{state: reconcileNotCommitted, pinned: *pinned}
	}
	return reconciliation{pinned: *pinned, err: fmt.Errorf("%w: %s now resolves to %s, not the source graph root %s", errUtils.ErrArtifactConflict, ref, pinned.ResolvedDigest, in.rootDigest)}
}

// matchesExpected verifies the stored Record and, when both sides describe them, the payload
// descriptors against what this operation prepared.
func matchesExpected(in *incoming, inspection *artifact.Inspection) error {
	if inspection.Record != nil {
		stored, err := inspection.Record.RecordDigest()
		if err != nil || stored != in.recordDigest {
			return fmt.Errorf("%w: the stored record does not match the prepared record", errUtils.ErrArtifactIntegrityFailed)
		}
	}
	if len(in.payload) == 0 || len(inspection.Representation.Payload) == 0 {
		return nil
	}
	want := descriptorDigests(in.payload)
	got := descriptorDigests(inspection.Representation.Payload)
	if !slices.Equal(want, got) {
		return fmt.Errorf("%w: the stored payload descriptors do not match the prepared payload", errUtils.ErrArtifactIntegrityFailed)
	}
	return nil
}

func descriptorDigests(descriptors []ocispec.Descriptor) []string {
	digests := make([]string, 0, len(descriptors))
	for index := range descriptors {
		digests = append(digests, descriptors[index].Digest.String())
	}
	slices.Sort(digests)
	return digests
}

func syntheticResult(pinned *artifact.PinnedReference, inspection *artifact.Inspection) *artifact.PublishResult {
	return &artifact.PublishResult{Pinned: *pinned, Representation: inspection.Representation, Location: inspection.Representation.Locator}
}
