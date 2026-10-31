//nolint:gocritic // OCI descriptors and the pinned reference are passed by value, mirroring the ORAS content contracts and the fixed artifact.Repository signatures.
package oci

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/opencontainers/go-digest"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
)

// subjectRoot is a manifest or index a signature subject is attached to.
type subjectRoot struct {
	name   string
	st     store
	loaded *root
}

// locateSubject finds the roots a subject digest names. Signature subjects carry only a digest,
// so the repository uses the roots it has resolved or inspected in this process; a layout, which
// holds every name in one store, can also search its own references. Anything else needs the
// artifact resolved first, which is what the service always does before signing or verifying.
func (r *Repository) locateSubject(ctx context.Context, subject artifact.SignatureSubject) ([]subjectRoot, error) {
	var roots []subjectRoot
	for _, known := range r.locations(subject.Digest) {
		st, err := r.be.open(ctx, known.name, false)
		if err != nil {
			return nil, err
		}
		loaded, err := r.loadCachedRoot(ctx, st, known.name, known.desc)
		if err != nil {
			return nil, err
		}
		roots = append(roots, subjectRoot{name: known.name, st: st, loaded: loaded})
	}
	if len(roots) == 0 {
		if layout, ok := r.be.(*layoutBackend); ok {
			return r.scanLayoutSubject(ctx, layout, subject)
		}
		return nil, fmt.Errorf("%w: subject %s is not a root this repository has resolved; resolve or inspect the artifact first", errUtils.ErrArtifactNotFound, subject.Digest)
	}
	return roots, nil
}

// scanLayoutSubject searches the named roots of a layout for the subject.
func (r *Repository) scanLayoutSubject(ctx context.Context, layout *layoutBackend, subject artifact.SignatureSubject) ([]subjectRoot, error) {
	refs, err := layout.references(ctx)
	if err != nil {
		return nil, err
	}
	var roots []subjectRoot
	for _, ref := range refs {
		_, loaded, err := r.resolveRoot(ctx, ref)
		if err != nil {
			if errors.Is(err, errUtils.ErrArtifactNotFound) {
				continue
			}
			return nil, err
		}
		if loaded.desc.Digest == subject.Digest || (loaded.class == classAtmos && loaded.manifest.Config.Digest == subject.Digest) {
			st, openErr := r.be.open(ctx, ref.Name, false)
			if openErr != nil {
				return nil, openErr
			}
			roots = append(roots, subjectRoot{name: ref.Name, st: st, loaded: loaded})
		}
	}
	if len(roots) == 0 {
		return nil, fmt.Errorf("%w: no reference in the layout has subject %s", errUtils.ErrArtifactNotFound, subject.Digest)
	}
	return roots, nil
}

// checkSubject validates a subject and that the repository can bind and verify evidence for it.
func (r *Repository) checkSubject(operation string, subject artifact.SignatureSubject) error {
	if err := subject.Validate(); err != nil {
		return err
	}
	caps := r.Capabilities()
	if !caps.HasSubjectKind(subject.Kind) {
		return artifact.NewUnsupportedCapability(r.alias, operation, "evidence/verify", "no signature verifier is configured, so this repository neither stores nor verifies evidence")
	}
	return nil
}

// bindSubject requires the located root to be what the subject says it is.
func bindSubject(subject artifact.SignatureSubject, loaded *root) error {
	switch subject.Kind {
	case artifact.SubjectRecord:
		if loaded.class != classAtmos || loaded.manifest.Config.Digest != subject.Digest {
			return fmt.Errorf("%w: %s is not the Record of an Atmos package in this repository", errUtils.ErrArtifactRefInvalid, subject.Digest)
		}
		if subject.Size != 0 && subject.Size != loaded.manifest.Config.Size {
			return fmt.Errorf("%w: the subject declares %d bytes but the Record is %d", errUtils.ErrArtifactRefInvalid, subject.Size, loaded.manifest.Config.Size)
		}
	case artifact.SubjectOCIRoot:
		if loaded.desc.Digest != subject.Digest {
			return fmt.Errorf("%w: %s is not the manifest or index digest of the located root", errUtils.ErrArtifactRefInvalid, subject.Digest)
		}
	default:
		return fmt.Errorf("%w: unsupported subject kind %q", errUtils.ErrArtifactRefInvalid, subject.Kind)
	}
	return nil
}

// Sign implements [artifact.Repository]. The signer produces evidence over the subject; the
// repository stores it as a referrer of the addressed manifest. A Record signature is bound to
// the manifest's config descriptor; an image or plugin signature is bound to the original
// manifest or index digest. Signing never changes the Record, the manifest, or any tag.
func (r *Repository) Sign(ctx context.Context, subject artifact.SignatureSubject, signer artifact.Signer, _ artifact.SignOptions) ([]artifact.SignatureEvidence, error) {
	defer perf.Track(nil, "oci.Repository.Sign")()

	if err := r.checkSubject("sign", subject); err != nil {
		return nil, err
	}
	if signer == nil {
		return nil, fmt.Errorf("%w: no signer supplied", errUtils.ErrArtifactConfigInvalid)
	}
	roots, err := r.locateSubject(ctx, subject)
	if err != nil {
		return nil, err
	}
	for index := range roots {
		if err := bindSubject(subject, roots[index].loaded); err != nil {
			return nil, err
		}
	}
	payload, err := signer.Sign(ctx, subject)
	if err != nil {
		return nil, err
	}
	if err := r.checkPayload(payload); err != nil {
		return nil, err
	}
	var stored []artifact.SignatureEvidence
	for index := range roots {
		target := &roots[index]
		evidence, manifestDigest, storeErr := r.storeAttachment(ctx, target.st, &attachRequest{root: target.loaded.desc, subject: subject, payload: payload, configDigest: target.loaded.manifest.Config.Digest})
		if storeErr != nil {
			return stored, storeErr
		}
		evidence.Locator = locatorDigest(r.be.locator(target.name), target.loaded.desc) + "#" + manifestDigest
		stored = append(stored, evidence)
	}
	return stored, nil
}

// checkPayload validates fresh evidence before it is stored.
func (r *Repository) checkPayload(payload *artifact.EvidencePayload) error {
	caps := r.Capabilities()
	switch {
	case payload == nil:
		return fmt.Errorf("%w: the signer returned no evidence", errUtils.ErrArtifactVerificationFailed)
	case !caps.HasEvidenceFormat(payload.Format):
		return artifact.NewUnsupportedCapability(r.alias, "sign", "evidence/"+string(payload.Format),
			fmt.Sprintf("the repository stores and verifies %v evidence", caps.EvidenceFormats))
	case !strings.HasPrefix(payload.MediaType, bundleMediaTypePrefix):
		return artifact.NewUnsupportedCapability(r.alias, "sign", "evidence/"+payload.MediaType,
			"only Sigstore bundle attachments are stored; other formats need a dedicated tested provider")
	case len(payload.Data) == 0 || int64(len(payload.Data)) > artifact.MaxEvidenceSize:
		return fmt.Errorf("%w: evidence is %d bytes; it must be between 1 and %d", errUtils.ErrArtifactTooLarge, len(payload.Data), artifact.MaxEvidenceSize)
	}
	return nil
}

// Verify implements [artifact.Repository]. It collects the subject's bundle referrers, drops
// any not bound to the subject (reporting them), and hands the rest to the injected verifier
// together with the policy. Invalid or unrelated attachments cannot override valid matching
// evidence; malformed ones are bounded and reported. No evidence, an unverifiable format, or an
// unsatisfied policy is a failure.
func (r *Repository) Verify(ctx context.Context, subject artifact.SignatureSubject, policy artifact.VerificationPolicy) (*artifact.VerificationResult, error) {
	defer perf.Track(nil, "oci.Repository.Verify")()

	if err := r.checkSubject("verify", subject); err != nil {
		return nil, err
	}
	roots, err := r.locateSubject(ctx, subject)
	if err != nil {
		return nil, err
	}
	result := &artifact.VerificationResult{Subject: subject, Status: artifact.VerificationFailed}
	if fingerprint, fingerprintErr := policy.Fingerprint(); fingerprintErr == nil {
		result.PolicyFingerprint = fingerprint
	}
	blobs, diagnostics, err := r.collectEvidence(ctx, subject, roots)
	result.Diagnostics = diagnostics
	if err != nil {
		return result, err
	}
	if len(blobs) == 0 {
		result.Diagnostics = append(result.Diagnostics, artifact.Diagnostic{Severity: artifact.SeverityError, Code: "no-evidence", Message: "no signature evidence is attached to the subject"})
		return result, fmt.Errorf("%w: no signature evidence is attached to %s", errUtils.ErrArtifactVerificationFailed, subject.Digest)
	}
	verified, verifyErr := r.settings.verifier.Verify(ctx, subject, blobs, policy)
	return finishVerification(result, verified, verifyErr)
}

// collectEvidence gathers the evidence bound to a subject across its located roots.
func (r *Repository) collectEvidence(ctx context.Context, subject artifact.SignatureSubject, roots []subjectRoot) ([]artifact.EvidenceBlob, []artifact.Diagnostic, error) {
	var (
		blobs       []artifact.EvidenceBlob
		diagnostics []artifact.Diagnostic
		seen        = map[digest.Digest]bool{}
	)
	for index := range roots {
		target := &roots[index]
		set, err := r.attachments(ctx, target.st, target.name, target.loaded.desc)
		if err != nil {
			return nil, diagnostics, fmt.Errorf("%w: %w", errUtils.ErrArtifactVerificationFailed, err)
		}
		diagnostics = append(diagnostics, set.diagnostics...)
		for item := range set.valid {
			att := &set.valid[item]
			if !att.bindsTo(subject, target.loaded) {
				diagnostics = append(diagnostics, artifact.Diagnostic{Severity: artifact.SeverityWarning, Code: "subject-mismatch", Message: fmt.Sprintf("referrer %s is bound to a different subject and was ignored", att.manifestDesc.Digest)})
				continue
			}
			if seen[att.layer.Digest] {
				continue
			}
			data, fetchErr := fetchBounded(ctx, target.st, att.layer, artifact.MaxEvidenceSize)
			if fetchErr != nil {
				if ctx.Err() != nil {
					return nil, diagnostics, ctx.Err()
				}
				diagnostics = append(diagnostics, artifact.Diagnostic{Severity: artifact.SeverityWarning, Code: "malformed-evidence", Message: fmt.Sprintf("referrer %s was ignored: %v", att.manifestDesc.Digest, fetchErr)})
				continue
			}
			seen[att.layer.Digest] = true
			blobs = append(blobs, artifact.EvidenceBlob{
				Evidence: artifact.SignatureEvidence{
					Subject: subject, Format: bundleFormat(data), Descriptor: att.layer,
					Locator: locatorDigest(r.be.locator(target.name), att.manifestDesc),
				},
				Data: data,
			})
		}
	}
	return blobs, diagnostics, nil
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

// subjectsOf returns the signature subjects an Atmos package or foreign root offers.
func subjectsOf(loaded *root) []artifact.SignatureSubject {
	subjects := []artifact.SignatureSubject{{Kind: artifact.SubjectOCIRoot, Digest: loaded.desc.Digest, MediaType: loaded.desc.MediaType, Size: loaded.desc.Size}}
	if loaded.class == classAtmos {
		config := loaded.manifest.Config
		subjects = append([]artifact.SignatureSubject{{Kind: artifact.SubjectRecord, Digest: config.Digest, MediaType: artifact.MediaTypeConfig, Size: config.Size}}, subjects...)
	}
	return subjects
}

// ReadEvidence implements [artifact.EvidenceReader]: it exports the bundle referrers bound to the
// pinned artifact, so a copy through the shared staging path can carry them along. The evidence
// is untrusted until verified.
func (r *Repository) ReadEvidence(ctx context.Context, pinned *artifact.PinnedReference) ([]artifact.EvidenceBlob, error) {
	defer perf.Track(nil, "oci.Repository.ReadEvidence")()

	st, loaded, err := r.rootOf(ctx, pinned)
	if err != nil {
		return nil, err
	}
	set, err := r.attachments(ctx, st, pinned.Ref.Name, loaded.desc)
	if err != nil {
		return nil, err
	}
	var blobs []artifact.EvidenceBlob
	for item := range set.valid {
		att := &set.valid[item]
		for _, subject := range subjectsOf(loaded) {
			if !att.bindsTo(subject, loaded) {
				continue
			}
			data, fetchErr := fetchBounded(ctx, st, att.layer, artifact.MaxEvidenceSize)
			if fetchErr != nil {
				return blobs, fetchErr
			}
			blobs = append(blobs, artifact.EvidenceBlob{
				Evidence: artifact.SignatureEvidence{Subject: subject, Format: bundleFormat(data), Descriptor: att.layer, Locator: locatorDigest(r.be.locator(pinned.Ref.Name), att.manifestDesc)},
				Data:     data,
			})
		}
	}
	return blobs, nil
}

// WriteEvidence implements [artifact.EvidenceWriter]: it imports evidence for an artifact this
// repository already holds. Each item must bind to the pinned manifest (a Record signature to its
// config descriptor, an image signature to its digest) and carry bytes matching their descriptor.
// Evidence the manifest already carries is not duplicated.
func (r *Repository) WriteEvidence(ctx context.Context, pinned *artifact.PinnedReference, evidence []artifact.EvidenceBlob, _ artifact.LockHandle) ([]artifact.SignatureEvidence, error) {
	defer perf.Track(nil, "oci.Repository.WriteEvidence")()

	st, loaded, err := r.rootOf(ctx, pinned)
	if err != nil {
		return nil, err
	}
	existing, err := r.attachments(ctx, st, pinned.Ref.Name, loaded.desc)
	if err != nil {
		return nil, err
	}
	stored := make([]artifact.SignatureEvidence, 0, len(evidence))
	for index := range evidence {
		blob := &evidence[index]
		subject := blob.Evidence.Subject
		if err := r.checkSubject("write evidence", subject); err != nil {
			return stored, err
		}
		if err := bindSubject(subject, loaded); err != nil {
			return stored, err
		}
		if blob.Evidence.Descriptor.Digest != digest.FromBytes(blob.Data) {
			return stored, fmt.Errorf("%w: evidence bytes do not match their descriptor %s", errUtils.ErrArtifactIntegrityFailed, blob.Evidence.Descriptor.Digest)
		}
		if holds(existing, subject, loaded, blob.Evidence.Descriptor.Digest) {
			stored = append(stored, blob.Evidence)
			continue
		}
		payload := &artifact.EvidencePayload{Format: blob.Evidence.Format, MediaType: blob.Evidence.Descriptor.MediaType, Data: blob.Data}
		if err := r.checkPayload(payload); err != nil {
			return stored, err
		}
		item, manifestDigest, storeErr := r.storeAttachment(ctx, st, &attachRequest{root: loaded.desc, subject: subject, payload: payload, configDigest: loaded.manifest.Config.Digest})
		if storeErr != nil {
			return stored, storeErr
		}
		item.Locator = locatorDigest(r.be.locator(pinned.Ref.Name), loaded.desc) + "#" + manifestDigest
		stored = append(stored, item)
	}
	return stored, nil
}

// holds reports whether the set already carries the bundle bound to the subject.
func holds(set *attachmentSet, subject artifact.SignatureSubject, loaded *root, bundle digest.Digest) bool {
	for index := range set.valid {
		if set.valid[index].layer.Digest == bundle && set.valid[index].bindsTo(subject, loaded) {
			return true
		}
	}
	return false
}
