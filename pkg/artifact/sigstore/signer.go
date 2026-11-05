package sigstore

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/signature"
)

// Signer implements [artifact.Signer] over an injected [signature.BlobSigner], typically the
// managed Cosign authoring path. It owns no cryptography and no registry credentials.
type Signer struct {
	blob signature.BlobSigner
}

var _ artifact.Signer = (*Signer)(nil)

// NewSigner returns a Signer that signs subject statements with blob.
func NewSigner(blob signature.BlobSigner) *Signer {
	defer perf.Track(nil, "sigstore.NewSigner")()

	return &Signer{blob: blob}
}

// Formats implements [artifact.Signer]. The produced format follows the bundle the injected signer
// returns: a certificate yields a sigstore bundle and a configured key yields a public-key bundle.
func (s *Signer) Formats() []artifact.EvidenceFormat {
	defer perf.Track(nil, "sigstore.Signer.Formats")()

	return []artifact.EvidenceFormat{artifact.EvidenceSigstoreBundle, artifact.EvidencePublicKeyBundle}
}

// Sign implements [artifact.Signer]. It signs [SubjectStatement] for the subject and refuses
// output that is oversized, malformed, or bound to different bytes.
func (s *Signer) Sign(ctx context.Context, subject artifact.SignatureSubject) (*artifact.EvidencePayload, error) {
	defer perf.Track(nil, "sigstore.Signer.Sign")()

	if s == nil || s.blob == nil {
		return nil, artifact.NewUnsupportedCapability("", "sign", "signer", "no Sigstore blob signer is configured")
	}
	if err := subject.Validate(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	bundle, err := signature.SignBlob(ctx, s.blob, SubjectStatement(subject))
	if err != nil {
		return nil, fmt.Errorf("%w: sign %s %s: %w", errUtils.ErrArtifactVerificationFailed, subject.Kind, subject.Digest, err)
	}
	format, mediaType, err := EvidenceFormatOf(bundle)
	if err != nil {
		return nil, err
	}
	return &artifact.EvidencePayload{Format: format, MediaType: mediaType, Data: bundle}, nil
}
