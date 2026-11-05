package artifact

import (
	"encoding/json"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// KindData is optional, kind-specific record data held as canonical JSON bytes.
//
// Design: a Record cannot carry map[string]any or float64 in its canonical path, yet kinds (for
// example legacy CI planfiles) need typed extra data. KindData is therefore a typed raw-JSON
// holder. [NewKindData] marshals a caller-owned typed value and immediately re-encodes it with
// [CanonicalizeJSON], which sorts object keys, writes stable string escapes, and rejects floats,
// duplicate keys, and invalid UTF-8. A KindData value is always canonical, so it embeds into the
// Record bytes unchanged and participates in the intent and record digests byte-for-byte.
// Consumers read it back into their own typed struct with [KindData.Decode].
type KindData []byte

// NewKindData canonicalizes v, which should be a typed struct, into KindData.
func NewKindData(v any) (KindData, error) {
	defer perf.Track(nil, "artifact.NewKindData")()

	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("%w: kind data: %w", errUtils.ErrArtifactRecordInvalid, err)
	}
	canonical, err := CanonicalizeJSON(raw)
	if err != nil {
		return nil, err
	}
	return KindData(canonical), nil
}

// Decode unmarshals the kind data into v, a pointer to the kind's typed struct.
func (k KindData) Decode(v any) error {
	defer perf.Track(nil, "artifact.KindData.Decode")()

	if len(k) == 0 {
		return fmt.Errorf("%w: record has no kind data", errUtils.ErrArtifactRecordInvalid)
	}
	return json.Unmarshal(k, v)
}

// MarshalJSON implements json.Marshaler, emitting the canonical bytes.
func (k KindData) MarshalJSON() ([]byte, error) {
	defer perf.Track(nil, "artifact.KindData.MarshalJSON")()

	if len(k) == 0 {
		return []byte("null"), nil
	}
	return CanonicalizeJSON(k)
}

// UnmarshalJSON implements json.Unmarshaler, canonicalizing the input.
func (k *KindData) UnmarshalJSON(data []byte) error {
	defer perf.Track(nil, "artifact.KindData.UnmarshalJSON")()

	canonical, err := CanonicalizeJSON(data)
	if err != nil {
		return err
	}
	if string(canonical) == "null" {
		*k = nil
		return nil
	}
	*k = KindData(canonical)
	return nil
}
