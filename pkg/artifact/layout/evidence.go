package layout

import (
	"fmt"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Evidence is the stored form of one detached signature evidence blob. The file name is the
// SHA-256 of Data, so the evidence digest the verifier reports identifies exactly the signed
// bytes, and a stored file is rejected when its name and content disagree.
//
// Evidence is stored beside, never inside, the signed Record, so adding a signature cannot change
// the Record, its digest, or the reference.
type Evidence struct {
	// SchemaVersion is [SchemaVersion].
	SchemaVersion int `json:"schema_version"`

	// Subject is the immutable subject the evidence claims to cover.
	Subject artifact.SignatureSubject `json:"subject"`

	// Format is the evidence encoding.
	Format artifact.EvidenceFormat `json:"format"`

	// MediaType is the evidence media type.
	MediaType string `json:"media_type"`

	// Data is the evidence bytes, at most [artifact.MaxEvidenceSize].
	Data []byte `json:"data"`
}

// NewEvidence builds the stored form of a freshly produced evidence payload.
func NewEvidence(subject artifact.SignatureSubject, payload *artifact.EvidencePayload) (*Evidence, error) {
	defer perf.Track(nil, "layout.NewEvidence")()

	evidence := &Evidence{SchemaVersion: SchemaVersion, Subject: subject, Format: payload.Format, MediaType: payload.MediaType, Data: payload.Data}
	if err := evidence.Validate(); err != nil {
		return nil, err
	}
	return evidence, nil
}

// Digest returns the evidence digest: the SHA-256 of the evidence bytes.
func (e *Evidence) Digest() digest.Digest {
	defer perf.Track(nil, "layout.Evidence.Digest")()

	return digest.FromBytes(e.Data)
}

// Validate checks the evidence envelope.
func (e *Evidence) Validate() error {
	defer perf.Track(nil, "layout.Evidence.Validate")()

	if e.SchemaVersion != SchemaVersion {
		return invalid(kindEvidence, "unsupported schema_version %d", e.SchemaVersion)
	}
	if err := e.Subject.Validate(); err != nil {
		return err
	}
	if e.Format == "" || e.MediaType == "" {
		return invalid(kindEvidence, "format and media_type are required")
	}
	if len(e.Data) == 0 {
		return invalid(kindEvidence, "data is empty")
	}
	if int64(len(e.Data)) > artifact.MaxEvidenceSize {
		return fmt.Errorf("%w: evidence is %d bytes; the limit is %d", errUtils.ErrArtifactTooLarge, len(e.Data), artifact.MaxEvidenceSize)
	}
	return nil
}

// Marshal returns the canonical bytes of the envelope.
func (e *Evidence) Marshal() ([]byte, error) {
	defer perf.Track(nil, "layout.Evidence.Marshal")()

	return encode(e, MaxEvidenceFileBytes)
}

// DecodeEvidence strictly parses a stored evidence file. The wantDigest argument is the evidence
// digest taken from the file name; it must equal the digest of the stored bytes.
func DecodeEvidence(raw []byte, wantDigest digest.Digest) (*Evidence, error) {
	defer perf.Track(nil, "layout.DecodeEvidence")()

	var evidence Evidence
	if err := decode(kindEvidence, raw, &evidence, MaxEvidenceFileBytes); err != nil {
		return nil, err
	}
	if evidence.Digest() != wantDigest {
		return nil, invalid(kindEvidence, "content digest %s does not match its file name %s", evidence.Digest(), wantDigest)
	}
	return &evidence, nil
}

// Blob converts the stored evidence into an [artifact.EvidenceBlob]. The locator argument is the
// display location of the evidence file.
func (e *Evidence) Blob(locator string) artifact.EvidenceBlob {
	defer perf.Track(nil, "layout.Evidence.Blob")()

	return artifact.EvidenceBlob{Evidence: e.Reference(locator), Data: append([]byte(nil), e.Data...)}
}

// Reference returns the descriptor of the stored evidence. The locator argument is the display
// location.
func (e *Evidence) Reference(locator string) artifact.SignatureEvidence {
	defer perf.Track(nil, "layout.Evidence.Reference")()

	return artifact.SignatureEvidence{
		Subject: e.Subject,
		Format:  e.Format,
		Descriptor: ocispec.Descriptor{
			MediaType: e.MediaType, Digest: e.Digest(), Size: int64(len(e.Data)),
		},
		Locator: locator,
	}
}
