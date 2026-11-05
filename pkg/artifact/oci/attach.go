//nolint:gocritic // OCI descriptors and the pinned reference are passed by value, mirroring the ORAS content contracts and the fixed artifact.Repository signatures.
package oci

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	atmosoci "github.com/cloudposse/atmos/pkg/oci"
)

// attachment is one signature referrer of a root: a manifest whose subject is the root and whose
// single layer is a Sigstore bundle. It is evidence candidate only; nothing here is trusted.
type attachment struct {
	manifestDesc ocispec.Descriptor
	layer        ocispec.Descriptor
	kind         artifact.SubjectKind
	subject      digest.Digest
	configDigest digest.Digest
}

// attachmentSet is the attachments found for a root, split by whether they are well formed.
// Malformed and over-limit referrers are reported, never silently dropped and never able to
// override valid matching evidence.
type attachmentSet struct {
	valid       []attachment
	diagnostics []artifact.Diagnostic
}

// attachments lists the Sigstore bundle referrers of a root, in digest order, inspecting at most
// [maxAttachments] of them. Referrers of other types (SBOMs, attestations) and manifests that
// merely contain the root as a child are not evidence and are skipped. A failure to list
// referrers is an error: required signature material must never be silently omitted.
func (r *Repository) attachments(ctx context.Context, st store, name string, rootDesc ocispec.Descriptor) (*attachmentSet, error) {
	predecessors, err := st.Predecessors(ctx, rootDesc)
	if err != nil {
		if isNotFound(err) {
			return &attachmentSet{}, nil
		}
		return nil, fmt.Errorf("%w: list signature referrers of %s: %w", errUtils.ErrArtifactDownloadFailed, rootDesc.Digest, err)
	}
	sort.Slice(predecessors, func(i, j int) bool { return predecessors[i].Digest < predecessors[j].Digest })
	set := &attachmentSet{}
	inspected := 0
	for index := range predecessors {
		if !isManifestMediaType(predecessors[index].MediaType) {
			continue
		}
		if inspected >= maxAttachments {
			set.warn("evidence-truncated", fmt.Sprintf("more than %d referrers of %s; the rest were not inspected", maxAttachments, rootDesc.Digest))
			break
		}
		inspected++
		item, problem, err := r.readAttachment(ctx, st, name, rootDesc, predecessors[index])
		switch {
		case err != nil:
			return nil, err
		case problem != "":
			set.warn("malformed-evidence", problem)
		case item != nil:
			set.valid = append(set.valid, *item)
		}
	}
	return set, nil
}

func (a *attachmentSet) warn(code, message string) {
	a.diagnostics = append(a.diagnostics, artifact.Diagnostic{Severity: artifact.SeverityWarning, Code: code, Message: message})
}

// readAttachment parses one referrer. It returns a nil attachment and no problem for a referrer
// that is simply not signature evidence, and a problem string for a malformed bundle referrer.
func (r *Repository) readAttachment(ctx context.Context, st store, name string, rootDesc, desc ocispec.Descriptor) (*attachment, string, error) {
	raw, err := r.cachedBytes(ctx, st, name, desc)
	if err != nil {
		if ctx.Err() != nil {
			return nil, "", ctx.Err()
		}
		return nil, fmt.Sprintf("referrer %s could not be read: %v", desc.Digest, err), nil
	}
	var manifest ocispec.Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, fmt.Sprintf("referrer %s is not a manifest: %v", desc.Digest, err), nil
	}
	if !isBundleReferrer(&manifest, rootDesc.Digest) {
		return nil, "", nil
	}
	if len(manifest.Layers) != 1 {
		return nil, fmt.Sprintf("signature referrer %s has %d layers; a bundle attachment has exactly one", desc.Digest, len(manifest.Layers)), nil
	}
	layer := manifest.Layers[0]
	if layer.Size <= 0 || layer.Size > artifact.MaxEvidenceSize {
		return nil, fmt.Sprintf("signature referrer %s has a %d byte bundle; the limit is %d", desc.Digest, layer.Size, artifact.MaxEvidenceSize), nil
	}
	return newAttachment(&manifest, desc, layer, rootDesc.Digest), "", nil
}

// isBundleReferrer reports whether a manifest is a Sigstore bundle attached to the subject digest.
// Referrers of other types and manifests that merely contain the root as a child are not evidence.
func isBundleReferrer(manifest *ocispec.Manifest, subject digest.Digest) bool {
	if manifest.Subject == nil || manifest.Subject.Digest != subject {
		return false
	}
	artifactType := manifest.ArtifactType
	if artifactType == "" {
		artifactType = manifest.Config.MediaType
	}
	return strings.HasPrefix(artifactType, bundleMediaTypePrefix)
}

// newAttachment reads the Record binding from the attachment's annotations. An attachment without
// one covers the root itself, like a signature made by any other Sigstore tool.
func newAttachment(manifest *ocispec.Manifest, desc, layer ocispec.Descriptor, rootDigest digest.Digest) *attachment {
	found := &attachment{manifestDesc: desc, layer: layer, kind: artifact.SubjectOCIRoot, subject: rootDigest}
	if manifest.Annotations[annotationSubjectKind] == string(artifact.SubjectRecord) {
		found.kind = artifact.SubjectRecord
		found.subject = digest.Digest(manifest.Annotations[annotationSubjectDigest])
		found.configDigest = digest.Digest(manifest.Annotations[annotationConfigDigest])
	}
	return found
}

// cachedBytes reads a small manifest through the verified cache.
func (r *Repository) cachedBytes(ctx context.Context, st store, name string, desc ocispec.Descriptor) ([]byte, error) {
	if raw, ok := r.cached(name, desc); ok {
		return raw, nil
	}
	raw, err := fetchBounded(ctx, st, desc, maxManifestSize)
	if err != nil {
		return nil, err
	}
	r.remember(name, desc, raw)
	return raw, nil
}

// bindsTo reports whether the attachment covers the subject: a Record signature must name the
// Record and the config descriptor of the addressed manifest; an image signature must carry no
// Record binding and cover the manifest or index digest itself.
func (a *attachment) bindsTo(subject artifact.SignatureSubject, loaded *root) bool {
	switch subject.Kind {
	case artifact.SubjectRecord:
		return a.kind == artifact.SubjectRecord && loaded.class == classAtmos &&
			a.subject == subject.Digest && a.configDigest == subject.Digest && loaded.manifest.Config.Digest == subject.Digest
	case artifact.SubjectOCIRoot:
		return a.kind == artifact.SubjectOCIRoot && loaded.desc.Digest == subject.Digest
	default:
		return false
	}
}

// bundleFormat classifies a bundle by its verification material: a bare public key means a
// public-key bundle, everything else is treated as a certificate bundle. The verifier still
// enforces the structure; this only routes the evidence.
func bundleFormat(data []byte) artifact.EvidenceFormat {
	var probe struct {
		VerificationMaterial struct {
			PublicKey json.RawMessage `json:"publicKey"`
		} `json:"verificationMaterial"`
	}
	if json.Unmarshal(data, &probe) == nil && len(probe.VerificationMaterial.PublicKey) > 0 {
		return artifact.EvidencePublicKeyBundle
	}
	return artifact.EvidenceSigstoreBundle
}

// attachRequest is one signature bundle to store as a referrer of a root.
type attachRequest struct {
	root         ocispec.Descriptor
	subject      artifact.SignatureSubject
	payload      *artifact.EvidencePayload
	configDigest digest.Digest
}

// storeAttachment writes a signature bundle as an artifact manifest whose subject is the root,
// so registries with the referrers API list it and layouts keep the relationship. A Record
// signature additionally carries the Record digest and the config descriptor digest of the
// addressed manifest. Writing is idempotent: identical evidence yields an identical manifest.
func (r *Repository) storeAttachment(ctx context.Context, st store, request *attachRequest) (artifact.SignatureEvidence, string, error) {
	rootDesc, subject, payload, configDigest := request.root, request.subject, request.payload, request.configDigest
	layer := ocispec.Descriptor{
		MediaType:   payload.MediaType,
		Digest:      digest.FromBytes(payload.Data),
		Size:        int64(len(payload.Data)),
		Annotations: map[string]string{annotationTitle: bundleFileTitle},
	}
	annotations := map[string]string{
		annotationSubjectKind:    string(subject.Kind),
		annotationSubjectDigest:  subject.Digest.String(),
		annotationEvidenceFormat: string(payload.Format),
	}
	if subject.Kind == artifact.SubjectRecord {
		annotations[annotationConfigDigest] = configDigest.String()
	}
	manifest := ocispec.Manifest{
		Versioned:    specs.Versioned{SchemaVersion: ociSchemaVersion},
		MediaType:    ocispec.MediaTypeImageManifest,
		ArtifactType: payload.MediaType,
		Config:       ocispec.DescriptorEmptyJSON,
		Layers:       []ocispec.Descriptor{layer},
		Subject:      &ocispec.Descriptor{MediaType: rootDesc.MediaType, Digest: rootDesc.Digest, Size: rootDesc.Size},
		Annotations:  annotations,
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		return artifact.SignatureEvidence{}, "", fmt.Errorf("%w: signature manifest: %w", errUtils.ErrArtifactPackagingInvalid, err)
	}
	desc := ocispec.Descriptor{MediaType: ocispec.MediaTypeImageManifest, Digest: digest.FromBytes(raw), Size: int64(len(raw))}
	for _, blob := range []struct {
		desc ocispec.Descriptor
		data []byte
	}{{layer, payload.Data}, {ocispec.DescriptorEmptyJSON, []byte("{}")}, {desc, raw}} {
		if err := pushIfAbsent(ctx, st, blob.desc, blob.data); err != nil {
			return artifact.SignatureEvidence{}, "", fmt.Errorf("%w: store signature evidence: %w", errUtils.ErrArtifactUploadFailed, err)
		}
	}
	evidence := artifact.SignatureEvidence{Subject: subject, Format: payload.Format, Descriptor: layer}
	return evidence, desc.Digest.String(), nil
}

// pushIfAbsent uploads a blob unless it is already present; a concurrent identical upload is
// success because content addressing makes it the same object.
func pushIfAbsent(ctx context.Context, st store, desc ocispec.Descriptor, data []byte) error {
	exists, err := st.Exists(ctx, desc)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	err = st.Push(ctx, desc, bytesReader(data))
	if err != nil && !atmosoci.IsAlreadyExists(err) {
		return err
	}
	return nil
}
