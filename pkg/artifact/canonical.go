package artifact

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/opencontainers/go-digest"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Validate checks the Record against the V1 schema, including that ContentDigest matches the
// inventory and documents. It does not check size limits of the serialized form; Canonical does.
func (r *Record) Validate() error {
	defer perf.Track(nil, "artifact.Record.Validate")()

	if r.SchemaVersion != SchemaVersion {
		return fmt.Errorf("%w: unsupported schema_version %d (want %d)", errUtils.ErrArtifactRecordInvalid, r.SchemaVersion, SchemaVersion)
	}
	if err := ValidateName(r.Name); err != nil {
		return err
	}
	if err := ValidateVersion(r.Version); err != nil {
		return err
	}
	if err := ValidateKind(r.Kind); err != nil {
		return err
	}
	if r.CreatedAt.IsZero() {
		return fmt.Errorf("%w: created_at is required", errUtils.ErrArtifactRecordInvalid)
	}
	if err := r.Metadata.validate(); err != nil {
		return err
	}
	expected, err := ComputeContentDigest(r.Inventory, r.Documents)
	if err != nil {
		return err
	}
	if r.ContentDigest != expected {
		return fmt.Errorf("%w: content_digest %q does not match the inventory and documents (%s)", errUtils.ErrArtifactRecordInvalid, r.ContentDigest, expected)
	}
	return nil
}

func (m Metadata) validate() error {
	for key := range m.Labels {
		if key == "" {
			return fmt.Errorf("%w: label keys must not be empty", errUtils.ErrArtifactRecordInvalid)
		}
	}
	return nil
}

// Seal sets SchemaVersion (when unset) and ContentDigest from the inventory and documents, then
// validates the Record. Call it after assembling a Record from scratch.
func (r *Record) Seal() error {
	defer perf.Track(nil, "artifact.Record.Seal")()

	if r.SchemaVersion == 0 {
		r.SchemaVersion = SchemaVersion
	}
	computed, err := ComputeContentDigest(r.Inventory, r.Documents)
	if err != nil {
		return err
	}
	r.ContentDigest = computed
	return r.Validate()
}

// normalized returns a deep-enough copy with sorted arrays, non-nil arrays, normalized time, and
// empty provenance dropped, ready for serialization. The receiver is never mutated.
func (r *Record) normalized() Record {
	out := *r
	out.CreatedAt = NewTimestamp(r.CreatedAt.Time())
	out.Inventory = sortedInventory(r.Inventory)
	out.Documents = sortedDocuments(r.Documents)
	out.Metadata.Provenance = r.Metadata.Provenance.normalized()
	if len(r.Metadata.Labels) == 0 {
		out.Metadata.Labels = nil
	}
	return out
}

func (p *Provenance) normalized() *Provenance {
	if p == nil {
		return nil
	}
	out := Provenance{}
	if p.CI != nil && *p.CI != (CIProvenance{}) {
		ci := *p.CI
		out.CI = &ci
	}
	if p.Source != nil && *p.Source != (SourceProvenance{}) {
		source := *p.Source
		out.Source = &source
	}
	if out.CI == nil && out.Source == nil {
		return nil
	}
	return &out
}

// Canonical returns the canonical Record bytes: sorted keys and arrays, UTF-8, integer sizes and
// modes, UTC RFC3339 seconds, no insignificant whitespace, and no trailing newline. The same bytes
// are stored as the OCI config blob and in local and S3 record files. The Record is validated
// first, and records larger than [MaxRecordSize] are rejected.
func (r *Record) Canonical() ([]byte, error) {
	defer perf.Track(nil, "artifact.Record.Canonical")()

	if err := r.Validate(); err != nil {
		return nil, err
	}
	canonical, err := canonicalJSON(r.normalized())
	if err != nil {
		return nil, err
	}
	if int64(len(canonical)) > MaxRecordSize {
		return nil, fmt.Errorf("%w: record is %d bytes; the limit is %d", errUtils.ErrArtifactTooLarge, len(canonical), MaxRecordSize)
	}
	return canonical, nil
}

// canonicalJSON marshals v with encoding/json and re-encodes the result canonically.
func canonicalJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(v); err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrArtifactRecordInvalid, err)
	}
	return CanonicalizeJSON(buf.Bytes())
}

// RecordDigest returns the SHA-256 of the full canonical Record bytes, including created_at.
func (r *Record) RecordDigest() (digest.Digest, error) {
	defer perf.Track(nil, "artifact.Record.RecordDigest")()

	canonical, err := r.Canonical()
	if err != nil {
		return "", err
	}
	return digest.FromBytes(canonical), nil
}

// DecodeRecord strictly parses canonical Record bytes. It rejects oversize input, unknown fields,
// invalid records, and any bytes that are not exactly the canonical serialization of the decoded
// Record, so the record digest of the stored bytes is always the digest of the decoded value.
func DecodeRecord(raw []byte) (*Record, error) {
	defer perf.Track(nil, "artifact.DecodeRecord")()

	if int64(len(raw)) > MaxRecordSize {
		return nil, fmt.Errorf("%w: record is %d bytes; the limit is %d", errUtils.ErrArtifactTooLarge, len(raw), MaxRecordSize)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var record Record
	if err := decoder.Decode(&record); err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrArtifactRecordInvalid, err)
	}
	canonical, err := record.Canonical()
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(canonical, raw) {
		return nil, fmt.Errorf("%w: bytes are not the canonical serialization", errUtils.ErrArtifactRecordInvalid)
	}
	return &record, nil
}

// ReadRecord reads and strictly decodes a Record from r, buffering at most [MaxRecordSize] bytes.
// It returns the canonical bytes alongside the Record so callers can compute the record digest
// of exactly what was stored.
func ReadRecord(r io.Reader) (*Record, []byte, error) {
	defer perf.Track(nil, "artifact.ReadRecord")()

	raw, err := ReadBounded(r, MaxRecordSize)
	if err != nil {
		return nil, nil, err
	}
	record, err := DecodeRecord(raw)
	if err != nil {
		return nil, nil, err
	}
	return record, raw, nil
}
