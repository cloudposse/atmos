package artifact

import "context"

//go:generate mockgen -package artifact -destination mock_evidence.go github.com/cloudposse/atmos/pkg/artifact EvidenceReader,EvidenceWriter

// EvidenceReader is an optional Repository extension that exports the signature evidence stored
// for a pinned artifact, so a copy through the shared staging fallback can carry it along.
type EvidenceReader interface {
	// ReadEvidence returns the evidence blobs bound to the pinned artifact. The evidence is
	// untrusted until verified.
	ReadEvidence(ctx context.Context, pinned *PinnedReference) ([]EvidenceBlob, error)
}

// EvidenceWriter is an optional Repository extension that imports signature evidence for an
// artifact the repository already holds. It never changes the signed Record or native graph root.
type EvidenceWriter interface {
	// WriteEvidence stores evidence for the pinned artifact and returns its stored descriptors.
	WriteEvidence(ctx context.Context, pinned *PinnedReference, evidence []EvidenceBlob, lock LockHandle) ([]SignatureEvidence, error)
}
