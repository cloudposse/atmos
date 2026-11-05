package service

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/opencontainers/go-digest"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
)

// verificationRequired reports whether a policy requires, or constrains, verification. Without
// either, consumption reports "unchecked" and never "verified".
func verificationRequired(policy artifact.VerificationPolicy) bool {
	return policy.RequireSignature || policy.HasTrustConstraints()
}

// mergePolicies combines policies so that every trust set of every policy must be satisfied
// (global and repository sets are never unioned into a weaker policy).
func mergePolicies(policies ...artifact.VerificationPolicy) artifact.VerificationPolicy {
	var merged artifact.VerificationPolicy
	for _, policy := range policies {
		merged.RequireSignature = merged.RequireSignature || policy.RequireSignature
		merged.Offline = merged.Offline || policy.Offline
		merged.TrustSets = append(merged.TrustSets, policy.TrustSets...)
	}
	return merged
}

// subjectOf derives the immutable signature subject from an inspection: the original OCI root when
// a native graph is carried, otherwise the portable Record. Metadata claims never choose it.
func subjectOf(inspection *artifact.Inspection) (artifact.SignatureSubject, error) {
	if root := inspection.Representation.OriginalRoot; root != nil {
		return artifact.SignatureSubject{Kind: artifact.SubjectOCIRoot, Digest: root.Digest, MediaType: root.MediaType, Size: root.Size}, nil
	}
	if inspection.Record == nil {
		return artifact.SignatureSubject{}, fmt.Errorf("%w: the artifact has neither a Record nor an OCI root to sign or verify", errUtils.ErrArtifactRecordInvalid)
	}
	canonical, err := inspection.Record.Canonical()
	if err != nil {
		return artifact.SignatureSubject{}, err
	}
	return artifact.SignatureSubject{
		Kind:      artifact.SubjectRecord,
		Digest:    digest.FromBytes(canonical),
		MediaType: artifact.MediaTypeConfig,
		Size:      int64(len(canonical)),
	}, nil
}

// verifySubject requires every trust set of policy to be satisfied by a valid permitted signature
// on subject. Each set is checked through its own repository Verify call, so the "both global and
// repository sets" rule holds regardless of the backend. Unsupported evidence fails closed.
func verifySubject(ctx context.Context, alias string, repo artifact.Repository, subject artifact.SignatureSubject, policy artifact.VerificationPolicy) (*artifact.VerificationResult, error) {
	caps := repo.Capabilities()
	if !caps.HasSubjectKind(subject.Kind) {
		unsupported := artifact.NewUnsupportedCapability(alias, opVerify, "subject/"+string(subject.Kind), "the repository cannot bind or verify evidence for this subject kind")
		return failedResult(subject, policy, unsupported), errors.Join(errUtils.ErrArtifactVerificationFailed, unsupported)
	}
	fingerprint, err := policy.Fingerprint()
	if err != nil {
		return nil, err
	}
	combined := &artifact.VerificationResult{Subject: subject, PolicyFingerprint: fingerprint, Status: artifact.VerificationVerified}
	for _, call := range splitPolicy(policy) {
		result, verifyErr := repo.Verify(ctx, subject, call)
		if failure := checkVerifyResult(subject, result, verifyErr); failure != nil {
			combined.Status = artifact.VerificationFailed
			if result != nil {
				combined.Diagnostics = append(combined.Diagnostics, result.Diagnostics...)
			}
			combined.Diagnostics = append(combined.Diagnostics, artifact.Diagnostic{Severity: artifact.SeverityError, Code: "verification-failed", Message: failure.Error()})
			return combined, errors.Join(errUtils.ErrArtifactVerificationFailed, failure)
		}
		mergeResult(combined, result)
	}
	return combined, nil
}

// mergeResult folds a verified per-set result into the combined one without duplicating claims,
// evidence digests, or diagnostics that several sets share.
func mergeResult(combined, result *artifact.VerificationResult) {
	for _, claim := range result.Claims {
		if !slices.Contains(combined.Claims, claim) {
			combined.Claims = append(combined.Claims, claim)
		}
	}
	for _, evidence := range result.EvidenceDigests {
		if !slices.Contains(combined.EvidenceDigests, evidence) {
			combined.EvidenceDigests = append(combined.EvidenceDigests, evidence)
		}
	}
	for _, diagnostic := range result.Diagnostics {
		if !slices.Contains(combined.Diagnostics, diagnostic) {
			combined.Diagnostics = append(combined.Diagnostics, diagnostic)
		}
	}
}

// splitPolicy returns one single-trust-set policy per set, or the policy itself when it has none.
func splitPolicy(policy artifact.VerificationPolicy) []artifact.VerificationPolicy {
	if len(policy.TrustSets) == 0 {
		policy.RequireSignature = true
		return []artifact.VerificationPolicy{policy}
	}
	calls := make([]artifact.VerificationPolicy, 0, len(policy.TrustSets))
	for _, set := range policy.TrustSets {
		calls = append(calls, artifact.VerificationPolicy{RequireSignature: true, TrustSets: []artifact.TrustSet{set}, Offline: policy.Offline})
	}
	return calls
}

// checkVerifyResult returns a failure when the repository errored, returned nothing, returned a
// result for another subject, or did not report the verified status.
func checkVerifyResult(subject artifact.SignatureSubject, result *artifact.VerificationResult, err error) error {
	switch {
	case err != nil:
		return err
	case result == nil:
		return fmt.Errorf("%w: the repository returned no verification result", errUtils.ErrArtifactVerificationFailed)
	case result.Subject.Digest != subject.Digest || result.Subject.Kind != subject.Kind:
		return fmt.Errorf("%w: the verification result is bound to %s, not the requested subject %s", errUtils.ErrArtifactVerificationFailed, result.Subject.Digest, subject.Digest)
	case result.Status != artifact.VerificationVerified:
		return fmt.Errorf("%w: verification status is %q", errUtils.ErrArtifactVerificationFailed, result.Status)
	}
	return nil
}

func failedResult(subject artifact.SignatureSubject, policy artifact.VerificationPolicy, cause error) *artifact.VerificationResult {
	fingerprint, _ := policy.Fingerprint()
	return &artifact.VerificationResult{
		Subject:           subject,
		PolicyFingerprint: fingerprint,
		Status:            artifact.VerificationFailed,
		Diagnostics:       []artifact.Diagnostic{{Severity: artifact.SeverityError, Code: "verification-failed", Message: cause.Error()}},
	}
}

// gate enforces the consumption policy. It returns an unchecked result when no policy requires or
// constrains verification (never "verified"), a verified result when every trust set is satisfied,
// and a failed result plus ErrArtifactVerificationFailed otherwise. Force never reaches this
// function.
func gate(ctx context.Context, alias string, repo artifact.Repository, inspection *artifact.Inspection, policies ...artifact.VerificationPolicy) (*artifact.VerificationResult, error) {
	merged := mergePolicies(policies...)
	if !verificationRequired(merged) {
		return &artifact.VerificationResult{Status: artifact.VerificationUnchecked}, nil
	}
	subject, err := subjectOf(inspection)
	if err != nil {
		return nil, err
	}
	return verifySubject(ctx, alias, repo, subject, merged)
}
