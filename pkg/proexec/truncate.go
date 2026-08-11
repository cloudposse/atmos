package proexec

import (
	"encoding/json"

	"github.com/cloudposse/atmos/pkg/pro"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
	"github.com/cloudposse/atmos/pkg/schema"
)

// truncationMarker is substituted for Data when it cannot be trimmed enough
// to fit under the payload limit.
const truncationMarker = "... truncated"

// maxWarningsKept bounds how many entries of a "warnings"-shaped array field
// are kept during the first-pass trim.
const maxWarningsKept = 5

// truncateIfNeeded compares the marshaled size of req against
// Settings.Pro.MaxPayloadBytes (falling back to pro.DefaultMaxPayloadBytes)
// and, when exceeded, trims large text fields inside Data. An execution
// record is one indivisible logical unit, so it is never chunked into
// multiple requests (FR-011, research.md Decision 6) — only trimmed in place.
func truncateIfNeeded(req *dtos.ExecUploadRequest, atmosConfig *schema.AtmosConfiguration) (*dtos.ExecUploadRequest, error) {
	maxBytes := pro.DefaultMaxPayloadBytes
	if atmosConfig != nil && atmosConfig.Settings.Pro.MaxPayloadBytes > 0 {
		maxBytes = atmosConfig.Settings.Pro.MaxPayloadBytes
	}

	size, err := marshaledSize(req)
	if err != nil {
		return nil, err
	}
	if size <= maxBytes || len(req.Data) == 0 {
		return req, nil
	}

	trimmed := trimDataFields(req.Data)
	req.Data = trimmed

	size, err = marshaledSize(req)
	if err != nil {
		return nil, err
	}
	if size <= maxBytes {
		return req, nil
	}

	// Still too large after trimming known text fields — replace Data
	// entirely with a truncation marker rather than chunk the request.
	marker, markerErr := json.Marshal(map[string]any{
		"truncated": true,
		"reason":    truncationMarker,
	})
	if markerErr != nil {
		return nil, markerErr
	}
	req.Data = marker

	return req, nil
}

// marshaledSize returns the byte length of req marshaled to JSON.
func marshaledSize(req *dtos.ExecUploadRequest) (int, error) {
	b, err := json.Marshal(req)
	if err != nil {
		return 0, err
	}
	return len(b), nil
}

// trimDataFields best-effort trims known large-text-shaped fields (e.g.
// "warnings") inside an opaque Data payload. Data is intentionally an `any`
// per research.md Decision 5 (no structured-data registry), so this operates
// generically on the decoded map rather than a concrete type.
func trimDataFields(data json.RawMessage) json.RawMessage {
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		// Not a JSON object we can selectively trim — leave as-is; the
		// caller will fall back to the truncation marker if still oversized.
		return data
	}

	trimmed := false
	for _, key := range []string{"warnings", "Warnings"} {
		if arr, ok := decoded[key].([]any); ok && len(arr) > maxWarningsKept {
			decoded[key] = append(arr[:maxWarningsKept], truncationMarker)
			trimmed = true
		}
	}
	if !trimmed {
		return data
	}

	out, err := json.Marshal(decoded)
	if err != nil {
		return data
	}
	return out
}
