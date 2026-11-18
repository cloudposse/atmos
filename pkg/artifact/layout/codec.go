package layout

import (
	"bytes"
	"encoding/json"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
)

// MaxDocumentBytes bounds every layout document (reference, representation, operation journal).
const MaxDocumentBytes int64 = 1 << 20

// MaxEvidenceFileBytes bounds one stored evidence file: the base64 form of the largest evidence
// blob plus its envelope.
const MaxEvidenceFileBytes = 2*artifact.MaxEvidenceSize + 4096

// Document kinds, used in error messages.
const (
	kindReference      = "reference"
	kindRepresentation = "representation"
	kindOperation      = "operation"
	kindEvidence       = "evidence"
)

// invalid wraps a malformed or inconsistent managed document. It matches both the integrity and
// the record-invalid sentinels, because a stored managed document that does not parse is, to the
// reader, corrupted state.
func invalid(kind, format string, args ...any) error {
	return fmt.Errorf("%w: %w: %s: %s", errUtils.ErrArtifactIntegrityFailed, errUtils.ErrArtifactRecordInvalid, kind, fmt.Sprintf(format, args...))
}

// validator is implemented by every layout document.
type validator interface {
	Validate() error
}

// encode canonicalizes v: sorted keys, no insignificant whitespace, no trailing newline. Stored
// bytes are therefore a function of the document alone.
func encode(v validator, limit int64) ([]byte, error) {
	if err := v.Validate(); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(v); err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrArtifactRecordInvalid, err)
	}
	canonical, err := artifact.CanonicalizeJSON(buf.Bytes())
	if err != nil {
		return nil, err
	}
	if int64(len(canonical)) > limit {
		return nil, fmt.Errorf("%w: layout document is %d bytes; the limit is %d", errUtils.ErrArtifactTooLarge, len(canonical), limit)
	}
	return canonical, nil
}

// decode strictly parses raw into v: it rejects oversize input, unknown fields, invalid
// documents, and any bytes that are not exactly the canonical serialization of the result.
func decode(kind string, raw []byte, v validator, limit int64) error {
	if int64(len(raw)) > limit {
		return invalid(kind, "%d bytes exceeds the %d byte limit", len(raw), limit)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(v); err != nil {
		return invalid(kind, "%v", err)
	}
	if err := v.Validate(); err != nil {
		return invalid(kind, "%v", err)
	}
	canonical, err := encode(v, limit)
	if err != nil {
		return invalid(kind, "%v", err)
	}
	if !bytes.Equal(canonical, raw) {
		return invalid(kind, "bytes are not the canonical serialization")
	}
	return nil
}
