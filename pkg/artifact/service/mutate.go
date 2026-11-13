package service

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Delete implements [artifact.Service]. Deletion is scoped to the pinned subject within the
// repository's declared delete capability and guarded by the writer locks. Digest references are
// inspected before locking to identify the canonical reference, then resolved again under lock.
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
	targets, canonical, err := deleteLockTargets(ctx, request, info, repo)
	if err != nil {
		return err
	}
	locks, err := s.acquireLocks(ctx, op, opDelete, targets)
	if err != nil {
		return err
	}
	defer func() { _ = locks.release(ctx, op) }()

	pinned, inspection, err := s.pinAndInspect(ctx, op, repo, request.Repository, request.Ref)
	if err != nil {
		return err
	}
	if request.Ref.IsDigest() && inspection.Record != nil && canonical != inspection.Record.Name+":"+inspection.Record.Version {
		return fmt.Errorf("%w: reference changed while acquiring the delete locks", errUtils.ErrArtifactConflict)
	}
	result, err := repo.Delete(ctx, pinned, artifact.DeleteOptions{ExpectedCurrent: &pinned, Scope: info.Capabilities.Delete, Lock: locks.handleForResource(request.Repository, canonical)})
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

// deleteLockTargets returns the locks a delete takes and the canonical reference it deletes. A
// digest reference is inspected before locking so the canonical name:version reference is also
// locked; it is resolved again under the locks.
func deleteLockTargets(ctx context.Context, request *artifact.DeleteRequest, info *RepositoryInfo, repo artifact.Repository) ([]lockTarget, string, error) {
	resource := lockTarget{alias: request.Repository, repo: repo, info: info, resource: request.Ref.String()}
	subject := lockTarget{alias: request.Repository, repo: repo, info: info, resource: subjectLockResource(request.Ref.Name)}
	targets := []lockTarget{resource, subject}
	canonical := resource.resource
	if !request.Ref.IsDigest() {
		return targets, canonical, nil
	}
	prePinned, err := repo.Resolve(ctx, request.Ref)
	if err != nil {
		return nil, "", err
	}
	preInspection, err := repo.Inspect(ctx, prePinned)
	if err != nil {
		return nil, "", err
	}
	if preInspection.Record != nil {
		canonical = preInspection.Record.Name + ":" + preInspection.Record.Version
		targets = append(targets, lockTarget{alias: request.Repository, repo: repo, info: info, resource: canonical})
	}
	return targets, canonical, nil
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
