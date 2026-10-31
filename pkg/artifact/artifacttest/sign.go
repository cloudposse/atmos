package artifacttest

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
)

// FakeEvidenceFormat is the evidence format of [FakeSigner]; the in-memory repository stores and
// verifies it. It stands in for a real format only inside tests.
const FakeEvidenceFormat = artifact.EvidenceSigstoreBundle

// fakeEvidenceMediaType is the media type of fake evidence.
const fakeEvidenceMediaType = "application/vnd.atmos.artifacttest.evidence.v1+json"

// fakeEvidence is the payload [FakeSigner] produces.
type fakeEvidence struct {
	Subject  digest.Digest `json:"subject"`
	Identity string        `json:"identity"`
	Issuer   string        `json:"issuer"`
}

// FakeSigner signs with a fixed identity and issuer. Evidence is bound to the subject digest.
type FakeSigner struct {
	// Identity is the signer identity claim.
	Identity string

	// Issuer is the issuer claim.
	Issuer string
}

var _ artifact.Signer = (*FakeSigner)(nil)

// Formats implements [artifact.Signer].
func (f *FakeSigner) Formats() []artifact.EvidenceFormat {
	defer perf.Track(nil, "artifacttest.FakeSigner.Formats")()

	return []artifact.EvidenceFormat{FakeEvidenceFormat}
}

// Sign implements [artifact.Signer].
func (f *FakeSigner) Sign(_ context.Context, subject artifact.SignatureSubject) (*artifact.EvidencePayload, error) {
	defer perf.Track(nil, "artifacttest.FakeSigner.Sign")()

	data, err := json.Marshal(fakeEvidence{Subject: subject.Digest, Identity: f.Identity, Issuer: f.Issuer})
	if err != nil {
		return nil, err
	}
	return &artifact.EvidencePayload{Format: FakeEvidenceFormat, MediaType: fakeEvidenceMediaType, Data: data}, nil
}

// TrustedPolicy returns a policy that only [FakeSigner] with the same identity and issuer satisfies.
func (f *FakeSigner) TrustedPolicy() artifact.VerificationPolicy {
	defer perf.Track(nil, "artifacttest.FakeSigner.TrustedPolicy")()

	return artifact.VerificationPolicy{
		RequireSignature: true,
		TrustSets:        []artifact.TrustSet{{Identities: []artifact.TrustedIdentity{{Identity: f.Identity, Issuer: f.Issuer}}}},
	}
}

// UntrustedPolicy returns a policy that no evidence from [FakeSigner] satisfies.
func (f *FakeSigner) UntrustedPolicy() artifact.VerificationPolicy {
	defer perf.Track(nil, "artifacttest.FakeSigner.UntrustedPolicy")()

	return artifact.VerificationPolicy{
		RequireSignature: true,
		TrustSets:        []artifact.TrustSet{{Identities: []artifact.TrustedIdentity{{Identity: f.Identity + "-other", Issuer: f.Issuer}}}},
	}
}

// Sign implements [artifact.Repository]: the evidence is stored keyed by the subject digest.
func (r *Repository) Sign(ctx context.Context, subject artifact.SignatureSubject, signer artifact.Signer, _ artifact.SignOptions) ([]artifact.SignatureEvidence, error) {
	defer perf.Track(nil, "artifacttest.Repository.Sign")()

	r.mu.Lock()
	r.count("Sign")
	if err := r.checkSubject(subject); err != nil {
		r.mu.Unlock()
		return nil, err
	}
	r.mu.Unlock()
	payload, err := signer.Sign(ctx, subject)
	if err != nil {
		return nil, err
	}
	if !r.caps.HasEvidenceFormat(payload.Format) {
		return nil, artifact.NewUnsupportedCapability(r.opts.Alias, "sign", "evidence/"+string(payload.Format), "the repository cannot store this evidence format")
	}
	evidence := artifact.SignatureEvidence{
		Subject: subject,
		Format:  payload.Format,
		Descriptor: ocispec.Descriptor{
			MediaType: payload.MediaType, Digest: digest.FromBytes(payload.Data), Size: int64(len(payload.Data)),
		},
		Locator: fmt.Sprintf("mem://%s/signatures/%s/%s", r.opts.Alias, subject.Digest.Encoded(), digest.FromBytes(payload.Data).Encoded()),
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.evidence[subject.Digest] = append(r.evidence[subject.Digest], artifact.EvidenceBlob{Evidence: evidence, Data: payload.Data})
	return []artifact.SignatureEvidence{evidence}, nil
}

// checkSubject requires the subject to be a stored record; callers hold r.mu.
func (r *Repository) checkSubject(subject artifact.SignatureSubject) error {
	if !r.caps.HasSubjectKind(subject.Kind) {
		return artifact.NewUnsupportedCapability(r.opts.Alias, "sign", "subject/"+string(subject.Kind), "unsupported subject kind")
	}
	if _, ok := r.records[subject.Digest]; !ok {
		return fmt.Errorf("%w: subject %s", errUtils.ErrArtifactNotFound, subject.Digest)
	}
	return nil
}

// Verify implements [artifact.Repository]. It authenticates stored evidence against the policy:
// every trust set must be satisfied by at least one evidence item bound to the subject. Unrelated
// or unsupported evidence is reported and ignored; it can never override matching evidence.
func (r *Repository) Verify(_ context.Context, subject artifact.SignatureSubject, policy artifact.VerificationPolicy) (*artifact.VerificationResult, error) {
	defer perf.Track(nil, "artifacttest.Repository.Verify")()

	r.mu.Lock()
	defer r.mu.Unlock()
	r.count("Verify")
	result := &artifact.VerificationResult{Subject: subject, Status: artifact.VerificationFailed}
	if fingerprint, err := policy.Fingerprint(); err == nil {
		result.PolicyFingerprint = fingerprint
	}
	var claims []artifact.SignerClaim
	held := r.evidence[subject.Digest]
	for index := range held {
		claim, ok := r.authenticate(subject, &held[index], result)
		if ok {
			claims = append(claims, claim)
			result.EvidenceDigests = append(result.EvidenceDigests, held[index].Evidence.Descriptor.Digest)
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

// authenticate checks one evidence item; callers hold r.mu.
func (r *Repository) authenticate(subject artifact.SignatureSubject, held *artifact.EvidenceBlob, result *artifact.VerificationResult) (artifact.SignerClaim, bool) {
	skip := func(code, message string) (artifact.SignerClaim, bool) {
		result.Diagnostics = append(result.Diagnostics, artifact.Diagnostic{Severity: artifact.SeverityWarning, Code: code, Message: message})
		return artifact.SignerClaim{}, false
	}
	if !r.caps.HasEvidenceFormat(held.Evidence.Format) {
		return skip("unsupported-evidence", "evidence in an unsupported format was ignored")
	}
	var parsed fakeEvidence
	if err := json.Unmarshal(held.Data, &parsed); err != nil {
		return skip("malformed-evidence", "malformed evidence was ignored")
	}
	if parsed.Subject != subject.Digest {
		return skip("subject-mismatch", "evidence bound to a different subject was ignored")
	}
	return artifact.SignerClaim{Identity: parsed.Identity, Issuer: parsed.Issuer}, true
}

// satisfies reports whether the claims meet every trust set; without sets any valid claim suffices.
func satisfies(policy artifact.VerificationPolicy, claims []artifact.SignerClaim) bool {
	if len(claims) == 0 {
		return false
	}
	for _, set := range policy.TrustSets {
		if !setSatisfied(set, claims) {
			return false
		}
	}
	return true
}

func setSatisfied(set artifact.TrustSet, claims []artifact.SignerClaim) bool {
	for _, claim := range claims {
		for _, trusted := range set.Identities {
			if trusted.Identity == claim.Identity && trusted.Issuer == claim.Issuer {
				return true
			}
		}
	}
	return false
}

// ReadEvidence implements [artifact.EvidenceReader].
func (r *Repository) ReadEvidence(_ context.Context, pinned *artifact.PinnedReference) ([]artifact.EvidenceBlob, error) {
	defer perf.Track(nil, "artifacttest.Repository.ReadEvidence")()

	r.mu.Lock()
	defer r.mu.Unlock()
	r.count("ReadEvidence")
	return append([]artifact.EvidenceBlob(nil), r.evidence[pinned.RecordDigest]...), nil
}

// WriteEvidence implements [artifact.EvidenceWriter].
func (r *Repository) WriteEvidence(_ context.Context, pinned *artifact.PinnedReference, evidence []artifact.EvidenceBlob, _ artifact.LockHandle) ([]artifact.SignatureEvidence, error) {
	defer perf.Track(nil, "artifacttest.Repository.WriteEvidence")()

	r.mu.Lock()
	defer r.mu.Unlock()
	r.count("WriteEvidence")
	if _, ok := r.records[pinned.RecordDigest]; !ok {
		return nil, fmt.Errorf("%w: record %s", errUtils.ErrArtifactNotFound, pinned.RecordDigest)
	}
	stored := make([]artifact.SignatureEvidence, 0, len(evidence))
	for index := range evidence {
		r.evidence[pinned.RecordDigest] = append(r.evidence[pinned.RecordDigest], evidence[index])
		stored = append(stored, evidence[index].Evidence)
	}
	return stored, nil
}
