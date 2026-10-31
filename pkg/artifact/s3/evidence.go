package s3

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/opencontainers/go-digest"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/layout"
	"github.com/cloudposse/atmos/pkg/perf"
)

// maxEvidenceFiles bounds how many evidence objects one verification or export reads. More than
// this is reported, not silently dropped.
const maxEvidenceFiles = 64

// loadedEvidence is one stored evidence object that parsed and matched its name.
type loadedEvidence struct {
	evidence *layout.Evidence
	locator  string
}

// loadEvidence reads the evidence stored for a Record subject. Malformed or oversize objects are
// reported as diagnostics and skipped; they can never override valid matching evidence. A failure
// to list or read the store is an error, because "no evidence" must never mean "could not look".
func (r *Repository) loadEvidence(ctx context.Context, subject digest.Digest) ([]loadedEvidence, []artifact.Diagnostic, error) {
	dir := layout.SignatureDir(artifact.SubjectRecord, subject) + "/"
	objects, err := r.listPrefix(ctx, dir)
	if err != nil {
		return nil, nil, err
	}
	var (
		loaded []loadedEvidence
		diags  []artifact.Diagnostic
	)
	for index := range objects {
		stem, ok := strings.CutSuffix(strings.TrimPrefix(objects[index].Key, dir), ".json")
		if !ok || strings.Contains(stem, "/") {
			continue
		}
		if len(loaded) >= maxEvidenceFiles {
			diags = append(diags, artifact.Diagnostic{Severity: artifact.SeverityWarning, Code: "evidence-truncated", Message: fmt.Sprintf("more than %d evidence files; the rest were not read", maxEvidenceFiles)})
			break
		}
		item, problem := r.readEvidenceObject(ctx, objects[index].Key, stem)
		switch {
		case errors.Is(problem, context.Canceled), errors.Is(problem, context.DeadlineExceeded):
			return nil, nil, problem
		case problem != nil:
			diags = append(diags, artifact.Diagnostic{Severity: artifact.SeverityWarning, Code: "malformed-evidence", Message: fmt.Sprintf("%s was ignored: %v", r.uri(objects[index].Key), problem)})
		default:
			loaded = append(loaded, loadedEvidence{evidence: item, locator: r.uri(objects[index].Key)})
		}
	}
	return loaded, diags, nil
}

func (r *Repository) readEvidenceObject(ctx context.Context, key, hexDigest string) (*layout.Evidence, error) {
	want := digest.NewDigestFromEncoded(digest.SHA256, hexDigest)
	if err := want.Validate(); err != nil {
		return nil, err
	}
	raw, _, err := r.readObject(ctx, key, layout.MaxEvidenceFileBytes)
	if err != nil {
		return nil, err
	}
	if raw == nil {
		return nil, fmt.Errorf("%w: evidence object disappeared", errUtils.ErrArtifactNotFound)
	}
	return layout.DecodeEvidence(raw, want)
}

// evidenceRefs converts loaded evidence into descriptors.
func (r *Repository) evidenceRefs(loaded []loadedEvidence) []artifact.SignatureEvidence {
	out := make([]artifact.SignatureEvidence, 0, len(loaded))
	for index := range loaded {
		out = append(out, loaded[index].evidence.Reference(loaded[index].locator))
	}
	return out
}

// requireCurrentRecord checks that digest names the current completed Record of its reference and
// returns the canonical bytes. Evidence only ever binds to a live subject.
func (r *Repository) requireCurrentRecord(ctx context.Context, recordDigest digest.Digest) ([]byte, error) {
	record, raw, err := r.loadRecord(ctx, recordDigest)
	if err != nil {
		return nil, err
	}
	if _, err := r.resolveRef(ctx, artifact.Ref{Name: record.Name, Digest: recordDigest}); err != nil {
		return nil, err
	}
	return raw, nil
}

// checkSubject validates a signature subject for this repository.
func (r *Repository) checkSubject(operation string, subject artifact.SignatureSubject) error {
	if err := subject.Validate(); err != nil {
		return err
	}
	if caps := r.Capabilities(); !caps.HasSubjectKind(subject.Kind) {
		return artifact.NewUnsupportedCapability(r.alias, operation, "subject/"+string(subject.Kind),
			"S3 repositories bind detached evidence to the canonical Record only")
	}
	return nil
}

// Sign implements [artifact.Repository]: it asks the signer for evidence over the Record subject
// and stores it as an object named by the evidence digest below the subject's directory. Evidence
// lives beside the Record, so signing never changes the Record, the reference, or the payload.
func (r *Repository) Sign(ctx context.Context, subject artifact.SignatureSubject, signer artifact.Signer, _ artifact.SignOptions) ([]artifact.SignatureEvidence, error) {
	defer perf.Track(nil, "s3.Repository.Sign")()

	if err := r.checkSubject("sign", subject); err != nil {
		return nil, err
	}
	if signer == nil {
		return nil, fmt.Errorf("%w: no signer supplied", errUtils.ErrArtifactConfigInvalid)
	}
	raw, err := r.requireCurrentRecord(ctx, subject.Digest)
	if err != nil {
		return nil, err
	}
	if subject.Size != 0 && subject.Size != int64(len(raw)) {
		return nil, fmt.Errorf("%w: the subject declares %d bytes but the Record is %d", errUtils.ErrArtifactRefInvalid, subject.Size, len(raw))
	}
	payload, err := signer.Sign(ctx, subject)
	if err != nil {
		return nil, err
	}
	if payload == nil {
		return nil, fmt.Errorf("%w: the signer returned no evidence", errUtils.ErrArtifactVerificationFailed)
	}
	stored, err := r.storeEvidence(ctx, subject, payload)
	if err != nil {
		return nil, err
	}
	return []artifact.SignatureEvidence{stored}, nil
}

// storeEvidence validates and writes one evidence payload.
func (r *Repository) storeEvidence(ctx context.Context, subject artifact.SignatureSubject, payload *artifact.EvidencePayload) (artifact.SignatureEvidence, error) {
	if caps := r.Capabilities(); !caps.HasEvidenceFormat(payload.Format) {
		return artifact.SignatureEvidence{}, artifact.NewUnsupportedCapability(r.alias, "sign", "evidence/"+string(payload.Format),
			fmt.Sprintf("the repository stores and verifies %v evidence; configure a verifier for this format", caps.EvidenceFormats))
	}
	evidence, err := layout.NewEvidence(subject, payload)
	if err != nil {
		return artifact.SignatureEvidence{}, err
	}
	raw, err := evidence.Marshal()
	if err != nil {
		return artifact.SignatureEvidence{}, err
	}
	key := layout.SignaturePath(subject.Kind, subject.Digest, evidence.Digest())
	if err := r.putImmutable(ctx, key, raw, contentTypeJSON, layout.MaxEvidenceFileBytes); err != nil {
		return artifact.SignatureEvidence{}, wrapWrite("store evidence", err)
	}
	return evidence.Reference(r.uri(key)), nil
}

// deleteEvidence removes every evidence object stored for a Record subject.
func (r *Repository) deleteEvidence(ctx context.Context, subject digest.Digest) error {
	objects, err := r.listPrefix(ctx, layout.SignatureDir(artifact.SubjectRecord, subject)+"/")
	if err != nil {
		return err
	}
	keys := make([]string, 0, len(objects))
	for index := range objects {
		keys = append(keys, objects[index].Key)
	}
	return r.deleteAll(ctx, keys)
}

// Verify implements [artifact.Repository]. It authenticates the stored evidence with the injected
// verifier; it never trusts a cached verdict. No evidence, an unverifiable format, or an
// unsatisfied policy is a failure.
func (r *Repository) Verify(ctx context.Context, subject artifact.SignatureSubject, policy artifact.VerificationPolicy) (*artifact.VerificationResult, error) {
	defer perf.Track(nil, "s3.Repository.Verify")()

	if err := r.checkSubject("verify", subject); err != nil {
		return nil, err
	}
	verifier := r.settings.verifier
	if verifier == nil {
		return nil, artifact.NewUnsupportedCapability(r.alias, "verify", "evidence/verify", "no signature verifier is configured for this repository")
	}
	if _, err := r.requireCurrentRecord(ctx, subject.Digest); err != nil {
		return nil, err
	}
	loaded, diags, err := r.loadEvidence(ctx, subject.Digest)
	if err != nil {
		return nil, err
	}
	blobs := evidenceBlobs(subject, loaded, &diags)
	result := &artifact.VerificationResult{Subject: subject, Status: artifact.VerificationFailed, Diagnostics: diags}
	if fingerprint, fingerprintErr := policy.Fingerprint(); fingerprintErr == nil {
		result.PolicyFingerprint = fingerprint
	}
	if len(blobs) == 0 {
		result.Diagnostics = append(result.Diagnostics, artifact.Diagnostic{Severity: artifact.SeverityError, Code: "no-evidence", Message: "no signature evidence is stored for the subject"})
		return result, fmt.Errorf("%w: no signature evidence is stored for %s", errUtils.ErrArtifactVerificationFailed, subject.Digest)
	}
	verified, verifyErr := verifier.Verify(ctx, subject, blobs, policy)
	return finishVerification(result, verified, verifyErr)
}

// evidenceBlobs converts the evidence bound to the subject, reporting any bound to another subject.
func evidenceBlobs(subject artifact.SignatureSubject, loaded []loadedEvidence, diags *[]artifact.Diagnostic) []artifact.EvidenceBlob {
	blobs := make([]artifact.EvidenceBlob, 0, len(loaded))
	for index := range loaded {
		stored := loaded[index].evidence
		if stored.Subject.Kind != subject.Kind || stored.Subject.Digest != subject.Digest {
			*diags = append(*diags, artifact.Diagnostic{Severity: artifact.SeverityWarning, Code: "subject-mismatch", Message: loaded[index].locator + " is bound to a different subject and was ignored"})
			continue
		}
		blobs = append(blobs, stored.Blob(loaded[index].locator))
	}
	return blobs
}

// finishVerification merges the verifier's result with the storage diagnostics and turns anything
// but a verified status into an error.
func finishVerification(base, verified *artifact.VerificationResult, verifyErr error) (*artifact.VerificationResult, error) {
	result := base
	if verified != nil {
		merged := *verified
		merged.Subject = base.Subject
		merged.Diagnostics = append(append([]artifact.Diagnostic(nil), base.Diagnostics...), verified.Diagnostics...)
		if merged.PolicyFingerprint == "" {
			merged.PolicyFingerprint = base.PolicyFingerprint
		}
		result = &merged
	}
	switch {
	case verifyErr != nil && errors.Is(verifyErr, errUtils.ErrArtifactVerificationFailed):
		return result, verifyErr
	case verifyErr != nil:
		return result, fmt.Errorf("%w: %w", errUtils.ErrArtifactVerificationFailed, verifyErr)
	case result.Status == artifact.VerificationFailed:
		return result, fmt.Errorf("%w: no valid permitted signature satisfies the policy for %s", errUtils.ErrArtifactVerificationFailed, result.Subject.Digest)
	}
	return result, nil
}

// ReadEvidence implements [artifact.EvidenceReader]: it exports the stored evidence blobs of a
// pinned artifact so a copy through the shared staging path can carry them along. The evidence is
// untrusted until verified.
func (r *Repository) ReadEvidence(ctx context.Context, pinned *artifact.PinnedReference) ([]artifact.EvidenceBlob, error) {
	defer perf.Track(nil, "s3.Repository.ReadEvidence")()

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, err := r.requireCurrentRecord(ctx, pinned.RecordDigest); err != nil {
		return nil, err
	}
	loaded, _, err := r.loadEvidence(ctx, pinned.RecordDigest)
	if err != nil {
		return nil, err
	}
	blobs := make([]artifact.EvidenceBlob, 0, len(loaded))
	for index := range loaded {
		blobs = append(blobs, loaded[index].evidence.Blob(loaded[index].locator))
	}
	return blobs, nil
}

// WriteEvidence implements [artifact.EvidenceWriter]: it imports evidence for an artifact this
// repository already holds. The evidence must bind to the pinned Record and carry matching bytes;
// importing never changes the Record or the reference.
func (r *Repository) WriteEvidence(ctx context.Context, pinned *artifact.PinnedReference, evidence []artifact.EvidenceBlob, _ artifact.LockHandle) ([]artifact.SignatureEvidence, error) {
	defer perf.Track(nil, "s3.Repository.WriteEvidence")()

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, err := r.requireCurrentRecord(ctx, pinned.RecordDigest); err != nil {
		return nil, err
	}
	stored := make([]artifact.SignatureEvidence, 0, len(evidence))
	for index := range evidence {
		blobEvidence := &evidence[index]
		subject := blobEvidence.Evidence.Subject
		if err := r.checkSubject("write evidence", subject); err != nil {
			return stored, err
		}
		if subject.Digest != pinned.RecordDigest {
			return stored, fmt.Errorf("%w: evidence is bound to %s, not to the pinned Record %s", errUtils.ErrArtifactRefInvalid, subject.Digest, pinned.RecordDigest)
		}
		if blobEvidence.Evidence.Descriptor.Digest != digest.FromBytes(blobEvidence.Data) {
			return stored, fmt.Errorf("%w: evidence bytes do not match their descriptor %s", errUtils.ErrArtifactIntegrityFailed, blobEvidence.Evidence.Descriptor.Digest)
		}
		item, err := r.storeEvidence(ctx, subject, &artifact.EvidencePayload{Format: blobEvidence.Evidence.Format, MediaType: blobEvidence.Evidence.Descriptor.MediaType, Data: blobEvidence.Data})
		if err != nil {
			return stored, err
		}
		stored = append(stored, item)
	}
	return stored, nil
}
