package local

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"

	"github.com/opencontainers/go-digest"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/layout"
	"github.com/cloudposse/atmos/pkg/perf"
)

// maxMalformedEvidence bounds diagnostics and reads for invalid evidence entries.
const maxMalformedEvidence = 64

var errTooManyMalformedEvidence = errors.New("too many malformed evidence files")

// loadedEvidence is one stored evidence file that parsed and matched its file name.
type loadedEvidence struct {
	evidence *layout.Evidence
	locator  string
}

// loadEvidence reads the evidence stored for a Record subject. Malformed or oversize files are
// reported as diagnostics and skipped; they can never override valid matching evidence.
func (t *tree) loadEvidence(subject digest.Digest) ([]loadedEvidence, []artifact.Diagnostic, error) {
	dir := layout.SignatureDir(artifact.SubjectRecord, subject)
	file, err := t.Open(native(dir))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("open evidence directory: %w", err)
	}
	defer file.Close()
	var loaded []loadedEvidence
	var diags []artifact.Diagnostic
	ignored := 0
	for {
		entries, readErr := file.ReadDir(maxMalformedEvidence)
		for _, entry := range entries {
			item, diag, ok := t.evidenceEntry(dir, entry)
			if ok {
				loaded = append(loaded, item)
				continue
			}
			ignored++
			if diag != nil {
				diags = append(diags, *diag)
			}
			if ignored > maxMalformedEvidence {
				diags = append(diags, artifact.Diagnostic{Severity: artifact.SeverityWarning, Code: "evidence-truncated", Message: "too many invalid evidence entries; remaining entries were not read"})
				return nil, diags, errTooManyMalformedEvidence
			}
		}
		if errors.Is(readErr, io.EOF) {
			return loaded, diags, nil
		}
		if readErr != nil {
			return nil, diags, fmt.Errorf("read evidence directory: %w", readErr)
		}
	}
}

// evidenceEntry reads one directory entry of an evidence directory. It reports ok for valid
// evidence; otherwise the entry is ignored, with a diagnostic when it looked like evidence.
func (t *tree) evidenceEntry(dir string, entry fs.DirEntry) (loadedEvidence, *artifact.Diagnostic, bool) {
	stem, ok := strings.CutSuffix(entry.Name(), ".json")
	if entry.IsDir() || !ok {
		return loadedEvidence{}, nil, false
	}
	locator := dir + "/" + entry.Name()
	item, problem := t.readEvidenceFile(locator, stem)
	if problem != nil {
		return loadedEvidence{}, &artifact.Diagnostic{Severity: artifact.SeverityWarning, Code: "malformed-evidence", Message: fmt.Sprintf("%s was ignored: %v", locator, problem)}, false
	}
	return loadedEvidence{evidence: item, locator: locator}, nil, true
}

func (t *tree) readEvidenceFile(locator, hexDigest string) (*layout.Evidence, error) {
	want := digest.NewDigestFromEncoded(digest.SHA256, hexDigest)
	if err := want.Validate(); err != nil {
		return nil, err
	}
	raw, err := t.readBounded(locator, layout.MaxEvidenceFileBytes)
	if err != nil {
		return nil, err
	}
	return layout.DecodeEvidence(raw, want)
}

// listEvidence returns the stored evidence descriptors for a subject, skipping malformed files.
func (t *tree) listEvidence(subject digest.Digest) []artifact.SignatureEvidence {
	loaded, _, _ := t.loadEvidence(subject)
	out := make([]artifact.SignatureEvidence, 0, len(loaded))
	for index := range loaded {
		out = append(out, loaded[index].evidence.Reference(loaded[index].locator))
	}
	return out
}

// requireCurrentRecord checks that digest names the current completed Record of its reference and
// returns the canonical bytes. Evidence only ever binds to a live subject.
func (r *Repository) requireCurrentRecord(t *tree, recordDigest digest.Digest) ([]byte, error) {
	record, raw, err := t.loadRecord(recordDigest)
	if err != nil {
		return nil, err
	}
	if _, err := r.resolveRef(t, artifact.Ref{Name: record.Name, Digest: recordDigest}); err != nil {
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
			"local repositories bind detached evidence to the canonical Record only")
	}
	return nil
}

// Sign implements [artifact.Repository]: it asks the signer for evidence over the Record subject
// and stores it as a detached file named by the evidence digest. Evidence lives beside the Record,
// so signing never changes the Record, the reference, or the payload. A signature added after
// publication covers the Record that describes the archive, not the original archive bytes.
func (r *Repository) Sign(ctx context.Context, subject artifact.SignatureSubject, signer artifact.Signer, options artifact.SignOptions) (result []artifact.SignatureEvidence, err error) {
	defer perf.Track(nil, "local.Repository.Sign")()

	if err := r.checkSignRequest(subject, signer); err != nil {
		return nil, err
	}
	t, err := r.openTree(false)
	if err != nil {
		return nil, r.missingRoot(err, artifact.Ref{Name: "(subject)", Digest: subject.Digest})
	}
	defer t.Close()
	held, err := r.lockEvidenceReference(ctx, t, &artifact.PinnedReference{RecordDigest: subject.Digest}, options.Lock)
	if err != nil {
		return nil, err
	}
	if held != nil {
		defer func() {
			if releaseErr := held.Release(context.WithoutCancel(ctx)); err == nil {
				err = releaseErr
			}
		}()
	}
	return r.signLocked(ctx, t, subject, signer)
}

// signLocked writes evidence while the reference lock is held.
func (r *Repository) signLocked(ctx context.Context, t *tree, subject artifact.SignatureSubject, signer artifact.Signer) ([]artifact.SignatureEvidence, error) {
	raw, err := r.requireCurrentRecord(t, subject.Digest)
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
	stored, err := r.storeEvidence(t, subject, payload)
	if err != nil {
		return nil, err
	}
	return []artifact.SignatureEvidence{stored}, nil
}

// checkSignRequest refuses an unsupported subject, a missing signer, and a signer whose evidence
// formats the repository cannot store, before anything is opened or signed.
func (r *Repository) checkSignRequest(subject artifact.SignatureSubject, signer artifact.Signer) error {
	if err := r.checkSubject("sign", subject); err != nil {
		return err
	}
	if signer == nil {
		return fmt.Errorf("%w: no signer supplied", errUtils.ErrArtifactConfigInvalid)
	}
	caps := r.Capabilities()
	return caps.CheckSignerFormats(r.alias, signer)
}

// storeEvidence validates and writes one evidence payload.
func (r *Repository) storeEvidence(t *tree, subject artifact.SignatureSubject, payload *artifact.EvidencePayload) (artifact.SignatureEvidence, error) {
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
	locator := layout.SignaturePath(subject.Kind, subject.Digest, evidence.Digest())
	if err := t.writeAtomic(locator, raw); err != nil {
		return artifact.SignatureEvidence{}, fmt.Errorf("%w: store evidence: %w", errUtils.ErrArtifactUploadFailed, err)
	}
	return evidence.Reference(locator), nil
}

// Verify implements [artifact.Repository]. It authenticates the stored evidence with the injected
// verifier; it never trusts a cached verdict. No evidence, an unverifiable format, or an
// unsatisfied policy is a failure.
func (r *Repository) Verify(ctx context.Context, subject artifact.SignatureSubject, policy artifact.VerificationPolicy) (*artifact.VerificationResult, error) {
	defer perf.Track(nil, "local.Repository.Verify")()

	if err := r.checkSubject("verify", subject); err != nil {
		return nil, err
	}
	verifier := r.settings.verifier
	if verifier == nil {
		return nil, artifact.NewUnsupportedCapability(r.alias, "verify", "evidence/verify", "no signature verifier is configured for this repository")
	}
	t, err := r.openTree(false)
	if err != nil {
		return nil, r.missingRoot(err, artifact.Ref{Name: "(subject)", Digest: subject.Digest})
	}
	defer t.Close()
	if _, err := r.requireCurrentRecord(t, subject.Digest); err != nil {
		return nil, err
	}
	loaded, diags, loadErr := t.loadEvidence(subject.Digest)
	if loadErr != nil {
		diags = append(diags, artifact.Diagnostic{Severity: artifact.SeverityWarning, Code: "evidence-read-failed", Message: loadErr.Error()})
	}
	blobs := evidenceBlobs(subject, loaded, &diags)
	result := &artifact.VerificationResult{Subject: subject, Status: artifact.VerificationFailed, Diagnostics: diags}
	if loadErr != nil {
		return result, fmt.Errorf("%w: %w", errUtils.ErrArtifactVerificationFailed, loadErr)
	}
	if fingerprint, fingerprintErr := policy.Fingerprint(); fingerprintErr == nil {
		result.PolicyFingerprint = fingerprint
	}
	if len(blobs) == 0 {
		result.Diagnostics = append(result.Diagnostics, artifact.Diagnostic{Severity: artifact.SeverityError, Code: "no-evidence", Message: "no signature evidence is stored for the subject"})
		return result, fmt.Errorf("%w: no signature evidence is stored for %s", errUtils.ErrArtifactVerificationFailed, subject.Digest)
	}
	verified, verifyErr := verifier.Verify(ctx, subject, blobs, policy)
	return r.finishVerification(result, verified, verifyErr)
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
func (r *Repository) finishVerification(base, verified *artifact.VerificationResult, verifyErr error) (*artifact.VerificationResult, error) {
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
	case result.Status != artifact.VerificationVerified:
		return result, fmt.Errorf("%w: no valid permitted signature satisfies the policy for %s", errUtils.ErrArtifactVerificationFailed, result.Subject.Digest)
	}
	return result, nil
}

// ReadEvidence implements [artifact.EvidenceReader]: it exports the stored evidence blobs of a
// pinned artifact so a copy through the shared staging path can carry them along. The evidence is
// untrusted until verified.
func (r *Repository) ReadEvidence(ctx context.Context, pinned *artifact.PinnedReference) ([]artifact.EvidenceBlob, error) {
	defer perf.Track(nil, "local.Repository.ReadEvidence")()

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	t, err := r.openTree(false)
	if err != nil {
		return nil, r.missingRoot(err, pinned.Ref)
	}
	defer t.Close()
	if _, err := r.requireCurrentRecord(t, pinned.RecordDigest); err != nil {
		return nil, err
	}
	loaded, _, err := t.loadEvidence(pinned.RecordDigest)
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
func (r *Repository) WriteEvidence(ctx context.Context, pinned *artifact.PinnedReference, evidence []artifact.EvidenceBlob, lock artifact.LockHandle) (stored []artifact.SignatureEvidence, err error) {
	defer perf.Track(nil, "local.Repository.WriteEvidence")()

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	t, err := r.openTree(false)
	if err != nil {
		return nil, r.missingRoot(err, pinned.Ref)
	}
	defer t.Close()
	held, err := r.lockEvidenceReference(ctx, t, pinned, lock)
	if err != nil {
		return nil, err
	}
	if held != nil {
		defer func() {
			if releaseErr := held.Release(context.WithoutCancel(ctx)); err == nil {
				err = releaseErr
			}
		}()
	}
	if _, err := r.requireCurrentRecord(t, pinned.RecordDigest); err != nil {
		return nil, err
	}
	return r.writeEvidenceLocked(t, pinned, evidence)
}

// writeEvidenceLocked validates and stores each evidence blob under the reference lock.
func (r *Repository) writeEvidenceLocked(t *tree, pinned *artifact.PinnedReference, evidence []artifact.EvidenceBlob) ([]artifact.SignatureEvidence, error) {
	stored := make([]artifact.SignatureEvidence, 0, len(evidence))
	for index := range evidence {
		blob := &evidence[index]
		subject := blob.Evidence.Subject
		if err := r.checkSubject("write evidence", subject); err != nil {
			return stored, err
		}
		if subject.Digest != pinned.RecordDigest {
			return stored, fmt.Errorf("%w: evidence is bound to %s, not to the pinned Record %s", errUtils.ErrArtifactRefInvalid, subject.Digest, pinned.RecordDigest)
		}
		if blob.Evidence.Descriptor.Digest != digest.FromBytes(blob.Data) {
			return stored, fmt.Errorf("%w: evidence bytes do not match their descriptor %s", errUtils.ErrArtifactIntegrityFailed, blob.Evidence.Descriptor.Digest)
		}
		item, err := r.storeEvidence(t, subject, &artifact.EvidencePayload{Format: blob.Evidence.Format, MediaType: blob.Evidence.Descriptor.MediaType, Data: blob.Data})
		if err != nil {
			return stored, err
		}
		stored = append(stored, item)
	}
	return stored, nil
}

// lockEvidenceReference reuses a matching caller lock or acquires the reference lock.
func (r *Repository) lockEvidenceReference(ctx context.Context, t *tree, pinned *artifact.PinnedReference, lock artifact.LockHandle) (*fileLock, error) {
	record, _, err := t.loadRecord(pinned.RecordDigest)
	if err != nil {
		return nil, err
	}
	resource := record.Name + ":" + record.Version
	if localLock, ok := lock.(*fileLock); ok {
		if !r.coversReference(localLock, resource) {
			return nil, fmt.Errorf("%w: supplied lock does not cover reference %s", errUtils.ErrArtifactConflict, resource)
		}
		return nil, nil
	}
	return r.acquire(ctx, resource, r.settings.owner, "")
}
