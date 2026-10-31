package artifacttest

import (
	"context"
	"encoding/json"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
)

// FakeVerifier is an [artifact.Verifier] for the evidence [FakeSigner] produces. Repositories that
// delegate cryptography to an injected verifier (the local repositories) use it in conformance
// tests, so those tests exercise the real storage and policy flow without Sigstore. It is a
// harness: it checks subject binding and the trust set, nothing cryptographic.
type FakeVerifier struct{}

var _ artifact.Verifier = (*FakeVerifier)(nil)

// Formats implements [artifact.Verifier].
func (*FakeVerifier) Formats() []artifact.EvidenceFormat {
	defer perf.Track(nil, "artifacttest.FakeVerifier.Formats")()

	return []artifact.EvidenceFormat{FakeEvidenceFormat}
}

// Verify implements [artifact.Verifier]. Every trust set must be satisfied by at least one
// evidence item bound to the subject. Malformed, unsupported, and unrelated evidence is reported
// and ignored; it can never override matching evidence.
func (*FakeVerifier) Verify(_ context.Context, subject artifact.SignatureSubject, evidence []artifact.EvidenceBlob, policy artifact.VerificationPolicy) (*artifact.VerificationResult, error) {
	defer perf.Track(nil, "artifacttest.FakeVerifier.Verify")()

	result := &artifact.VerificationResult{Subject: subject, Status: artifact.VerificationFailed}
	if fingerprint, err := policy.Fingerprint(); err == nil {
		result.PolicyFingerprint = fingerprint
	}
	if !policy.HasTrustConstraints() {
		result.Status = artifact.VerificationUnchecked
		return result, nil
	}
	var claims []artifact.SignerClaim
	for index := range evidence {
		claim, ok := authenticateFake(subject, &evidence[index], result)
		if ok {
			claims = append(claims, claim)
			result.EvidenceDigests = append(result.EvidenceDigests, evidence[index].Evidence.Descriptor.Digest)
		}
	}
	if !satisfies(policy, claims) {
		result.Diagnostics = append(result.Diagnostics, artifact.Diagnostic{Severity: artifact.SeverityError, Code: "no-permitted-signature", Message: "no valid permitted signature satisfies the trust set"})
		return result, fmt.Errorf("%w: no valid permitted signature for %s", errUtils.ErrArtifactVerificationFailed, subject.Digest)
	}
	result.Claims = claims
	result.Status = artifact.VerificationVerified
	return result, nil
}

func authenticateFake(subject artifact.SignatureSubject, held *artifact.EvidenceBlob, result *artifact.VerificationResult) (artifact.SignerClaim, bool) {
	skip := func(code, message string) (artifact.SignerClaim, bool) {
		result.Diagnostics = append(result.Diagnostics, artifact.Diagnostic{Severity: artifact.SeverityWarning, Code: code, Message: message})
		return artifact.SignerClaim{}, false
	}
	if held.Evidence.Format != FakeEvidenceFormat {
		return skip("unsupported-evidence", "evidence in an unsupported format was ignored")
	}
	var parsed fakeEvidence
	if err := json.Unmarshal(held.Data, &parsed); err != nil {
		return skip("malformed-evidence", "malformed evidence was ignored")
	}
	if parsed.Subject != subject.Digest || held.Evidence.Subject.Digest != subject.Digest {
		return skip("subject-mismatch", "evidence bound to a different subject was ignored")
	}
	return artifact.SignerClaim{Identity: parsed.Identity, Issuer: parsed.Issuer}, true
}
