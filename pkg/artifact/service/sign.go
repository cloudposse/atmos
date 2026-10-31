package service

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Sign implements [artifact.Service]. It accepts digest references only, binds evidence to the
// resolved immutable subject, and reports that the subject is signed, not policy-verified.
func (s *Service) Sign(ctx context.Context, request artifact.SignRequest) (*artifact.SignResult, error) {
	defer perf.Track(nil, "service.Service.Sign")()

	op := s.begin(artifact.ActionSign, request.Repository, request.Ref.String())
	result := &artifact.SignResult{}
	err := s.sign(ctx, op, &request, result)
	result.Operation = op.done(err)
	return result, err
}

func (s *Service) sign(ctx context.Context, op *operation, request *artifact.SignRequest, result *artifact.SignResult) error {
	if err := s.validateSign(request); err != nil {
		return err
	}
	info, repo, err := s.openForRead(ctx, request.Repository, opSign)
	if err != nil {
		return err
	}
	pinned, inspection, err := s.pinAndInspect(ctx, op, repo, request.Repository, request.Ref)
	if err != nil {
		return err
	}
	subject, err := signSubject(&pinned, inspection, request.Ref)
	if err != nil {
		return err
	}
	if err := checkSigningCapability(request.Repository, &info, s.signer, subject); err != nil {
		return err
	}
	locks, err := s.acquireLocks(ctx, op, opSign, []lockTarget{{alias: request.Repository, repo: repo, info: &info, resource: request.Ref.String()}})
	if err != nil {
		return err
	}
	defer func() { _ = locks.release(ctx, op) }()

	evidence, err := repo.Sign(ctx, subject, s.signer, artifact.SignOptions{OperationID: op.id, Lock: locks.handleFor(request.Repository)})
	if err != nil {
		return fmt.Errorf("sign %s in repository %q: %w", request.Ref, request.Repository, err)
	}
	result.Subject, result.Evidence = subject, evidence
	op.result.Outcome = artifact.OutcomeCreated
	op.result.LockScope = locks.scope()
	op.info("signed-not-verified", "the subject is signed; signing alone does not verify it against trust policy, use verify")
	return nil
}

// validateSign requires a repository, a valid digest reference, and a configured signer.
func (s *Service) validateSign(request *artifact.SignRequest) error {
	if err := validateRepositoryRef(request.Repository, request.Ref); err != nil {
		return err
	}
	if !request.Ref.IsDigest() {
		return fmt.Errorf("%w: sign accepts digest references only (%s@sha256:...), not %s", errUtils.ErrArtifactRefInvalid, request.Ref.Name, request.Ref)
	}
	if s.signer == nil {
		return artifact.NewUnsupportedCapability(request.Repository, opSign, "signer", "no signer is configured")
	}
	return nil
}

// signSubject binds the digest reference to the immutable subject: the portable Record when the
// digest names the Record (or the resolved local/S3 digest), the original OCI root otherwise.
func signSubject(pinned *artifact.PinnedReference, inspection *artifact.Inspection, ref artifact.Ref) (artifact.SignatureSubject, error) {
	subject, err := subjectOf(inspection)
	if err != nil {
		return subject, err
	}
	if ref.Digest != subject.Digest {
		return subject, fmt.Errorf("%w: %s does not name the signable subject %s of the resolved artifact (resolved %s)", errUtils.ErrArtifactRefInvalid, ref, subject.Digest, pinned.ResolvedDigest)
	}
	return subject, subject.Validate()
}

func checkSigningCapability(alias string, info *RepositoryInfo, signer artifact.Signer, subject artifact.SignatureSubject) error {
	if !info.Capabilities.Write {
		return artifact.NewUnsupportedCapability(alias, opSign, "write", "the repository cannot store signature evidence")
	}
	if !info.Capabilities.HasSubjectKind(subject.Kind) {
		return artifact.NewUnsupportedCapability(alias, opSign, "subject/"+string(subject.Kind), "the repository cannot bind evidence to this subject kind")
	}
	for _, format := range signer.Formats() {
		if info.Capabilities.HasEvidenceFormat(format) {
			return nil
		}
	}
	return artifact.NewUnsupportedCapability(alias, opSign, "evidence/format", fmt.Sprintf("the signer produces %v but the repository stores %v", signer.Formats(), info.Capabilities.EvidenceFormats))
}

// Verify implements [artifact.Service]. An explicit verify always requires a trusted valid
// signature: with no usable trust configuration the result is unchecked and the call fails rather
// than returning a green result.
func (s *Service) Verify(ctx context.Context, request artifact.VerifyRequest) (*artifact.VerifyResult, error) {
	defer perf.Track(nil, "service.Service.Verify")()

	op := s.begin(artifact.ActionVerify, request.Repository, request.Ref.String())
	result := &artifact.VerifyResult{}
	err := s.verify(ctx, op, &request, result)
	result.Operation = op.done(err)
	return result, err
}

func (s *Service) verify(ctx context.Context, op *operation, request *artifact.VerifyRequest, result *artifact.VerifyResult) error {
	if err := validateRepositoryRef(request.Repository, request.Ref); err != nil {
		return err
	}
	info, repo, err := s.openForRead(ctx, request.Repository, opVerify)
	if err != nil {
		return err
	}
	_, inspection, err := s.pinAndInspect(ctx, op, repo, request.Repository, request.Ref)
	if err != nil {
		return err
	}
	subject, err := subjectOf(inspection)
	if err != nil {
		return err
	}
	policy := info.Policy
	policy.RequireSignature = true
	if !policy.HasTrustConstraints() {
		result.Verification = artifact.VerificationResult{Subject: subject, Status: artifact.VerificationUnchecked}
		return fmt.Errorf("%w: repository %q has no trusted identities or keys configured, so nothing can be verified; configure artifacts.verification", errUtils.ErrArtifactConfigInvalid, request.Repository)
	}
	verification, err := verifySubject(ctx, request.Repository, repo, subject, policy)
	if verification != nil {
		result.Verification = *verification
	}
	if err != nil {
		return err
	}
	op.result.Outcome = artifact.OutcomeRead
	return nil
}
