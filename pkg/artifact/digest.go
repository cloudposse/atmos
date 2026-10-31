package artifact

import (
	"github.com/opencontainers/go-digest"

	"github.com/cloudposse/atmos/pkg/perf"
)

// contentView is the exact document hashed for content_digest.
type contentView struct {
	Documents     []Document `json:"documents"`
	Inventory     []Entry    `json:"inventory"`
	SchemaVersion int        `json:"schema_version"`
}

// userMetadata is the part of Metadata that participates in intent identity.
type userMetadata struct {
	Description string            `json:"description,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
}

// intentView is the exact document hashed for intent_digest.
type intentView struct {
	ContentDigest digest.Digest `json:"content_digest"`
	Kind          string        `json:"kind"`
	KindData      KindData      `json:"kind_data,omitempty"`
	Metadata      userMetadata  `json:"metadata"`
	Name          string        `json:"name"`
	SchemaVersion int           `json:"schema_version"`
	Version       string        `json:"version"`
}

// ComputeContentDigest returns the content_digest: the SHA-256 of the canonical JSON object
// {"documents":[...],"inventory":[...],"schema_version":1} built from the sorted inventory and
// document descriptors, including paths, modes, and link targets. It excludes storage locators
// and generated operation data. Inputs are validated first.
func ComputeContentDigest(inventory []Entry, documents []Document) (digest.Digest, error) {
	defer perf.Track(nil, "artifact.ComputeContentDigest")()

	if err := validateInventory(inventory); err != nil {
		return "", err
	}
	if err := validateDocuments(documents); err != nil {
		return "", err
	}
	canonical, err := canonicalJSON(contentView{
		Documents:     sortedDocuments(documents),
		Inventory:     sortedInventory(inventory),
		SchemaVersion: SchemaVersion,
	})
	if err != nil {
		return "", err
	}
	return digest.FromBytes(canonical), nil
}

// IntentDigest returns the intent_digest: the SHA-256 of the canonical JSON object holding
// name, version, kind, content_digest, the effective user metadata (description and labels), and
// the kind data. It excludes created_at and generated provenance so that a retry of the same
// intent yields the same digest and a no-op can be detected.
func (r *Record) IntentDigest() (digest.Digest, error) {
	defer perf.Track(nil, "artifact.Record.IntentDigest")()

	if err := r.Validate(); err != nil {
		return "", err
	}
	normalized := r.normalized()
	canonical, err := canonicalJSON(intentView{
		ContentDigest: normalized.ContentDigest,
		Kind:          normalized.Kind,
		KindData:      normalized.KindData,
		Metadata:      userMetadata{Description: normalized.Metadata.Description, Labels: normalized.Metadata.Labels},
		Name:          normalized.Name,
		SchemaVersion: normalized.SchemaVersion,
		Version:       normalized.Version,
	})
	if err != nil {
		return "", err
	}
	return digest.FromBytes(canonical), nil
}
