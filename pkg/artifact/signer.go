package artifact

import "context"

//go:generate mockgen -package artifact -destination mock_signer.go github.com/cloudposse/atmos/pkg/artifact Signer

// EvidencePayload is freshly produced signature evidence, bounded by [MaxEvidenceSize].
type EvidencePayload struct {
	// Format is the evidence encoding.
	Format EvidenceFormat

	// MediaType is the evidence media type.
	MediaType string

	// Data holds the evidence bytes.
	Data []byte
}

// Signer produces signature evidence for a subject. V1 reuses the managed Cosign authoring path
// behind this interface; registry authentication stays separate from signing identity, and no
// custom cryptography is implemented.
type Signer interface {
	// Formats lists the evidence formats the signer can produce.
	Formats() []EvidenceFormat

	// Sign signs the immutable subject and returns evidence for the repository to store.
	Sign(ctx context.Context, subject SignatureSubject) (*EvidencePayload, error)
}
