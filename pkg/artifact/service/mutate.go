package service

import (
	"context"
	"fmt"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Delete implements [artifact.Service]. The reference is resolved once; deletion is scoped to the
// pinned subject within the repository's declared delete capability and guarded by the writer lock.
func (s *Service) Delete(ctx context.Context, request artifact.DeleteRequest) (*artifact.OperationResult, error) {
	defer perf.Track(nil, "service.Service.Delete")()

	op := s.begin(artifact.ActionDelete, request.Repository, request.Ref.String())
	err := s.deleteArtifact(ctx, op, &request)
	result := op.done(err)
	return &result, err
}

func (s *Service) deleteArtifact(ctx context.Context, op *operation, request *artifact.DeleteRequest) error {
	if err := validateRepositoryRef(request.Repository, request.Ref); err != nil {
		return err
	}
	info, repo, err := s.openForRead(ctx, request.Repository, opDelete)
	if err != nil {
		return err
	}
	if caps := info.Capabilities; caps.Delete == artifact.DeleteNone || caps.Delete == "" || !caps.Write {
		return artifact.NewUnsupportedCapability(request.Repository, opDelete, opDelete, "the repository cannot delete artifacts")
	}
	return s.runDelete(ctx, op, request, &info, repo)
}

// validateRepositoryRef checks the two fields every single-artifact operation needs.
func validateRepositoryRef(repository string, ref artifact.Ref) error {
	if err := requireRepository(repository); err != nil {
		return err
	}
	return ref.Validate()
}

func (s *Service) runDelete(ctx context.Context, op *operation, request *artifact.DeleteRequest, info *RepositoryInfo, repo artifact.Repository) error {
	resource := lockTarget{alias: request.Repository, repo: repo, info: info, resource: request.Ref.String()}
	locks, err := s.acquireLocks(ctx, op, opDelete, []lockTarget{resource})
	if err != nil {
		return err
	}
	defer func() { _ = locks.release(ctx, op) }()

	pinned, inspection, err := s.pinAndInspect(ctx, op, repo, request.Repository, request.Ref)
	if err != nil {
		return err
	}
	result, err := repo.Delete(ctx, pinned, artifact.DeleteOptions{ExpectedCurrent: &pinned, Scope: info.Capabilities.Delete, Lock: locks.handleFor(request.Repository)})
	if err != nil {
		return fmt.Errorf("delete %s in repository %q: %w", request.Ref, request.Repository, err)
	}
	op.result.Outcome = artifact.OutcomeDeleted
	op.result.LockScope = locks.scope()
	op.result.Size = payloadSize(inspection)
	if len(result.Deleted) > 0 {
		op.result.Location = result.Deleted[0]
		op.result.Aliases = result.Deleted[1:]
	}
	for _, affected := range result.Affected {
		op.info("affected-reference", fmt.Sprintf("the deletion also affected %s", affected))
	}
	return nil
}

// Pin implements [artifact.Service]. A pin is immutable reference data for consumer lockfiles; it
// acquires no write lock and is not a substitute for signature verification.
func (s *Service) Pin(ctx context.Context, request artifact.PinRequest) (*artifact.PinResult, error) {
	defer perf.Track(nil, "service.Service.Pin")()

	op := s.begin(artifact.ActionPin, request.Repository, request.Ref.String())
	result := &artifact.PinResult{}
	err := s.pin(ctx, op, &request, result)
	result.Operation = op.done(err)
	return result, err
}

func (s *Service) pin(ctx context.Context, op *operation, request *artifact.PinRequest, result *artifact.PinResult) error {
	if err := validateRepositoryRef(request.Repository, request.Ref); err != nil {
		return err
	}
	_, repo, err := s.openForRead(ctx, request.Repository, opPin)
	if err != nil {
		return err
	}
	pinned, err := repo.Pin(ctx, request.Ref)
	if err != nil {
		return fmt.Errorf("pin %s in repository %q: %w", request.Ref, request.Repository, err)
	}
	result.Pinned = pinned
	op.result.Outcome = artifact.OutcomeRead
	op.result.ResolvedDigest = pinned.ResolvedDigest
	op.result.RecordDigest = pinned.RecordDigest
	op.result.ContentDigest = pinned.ContentDigest
	op.info("pin-not-trust", "a pin fixes the immutable reference; it does not verify signatures, and trust policy is reevaluated when it is used")
	return nil
}
