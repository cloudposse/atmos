package artifact

import (
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// timestampLayout is the canonical UTC RFC3339 layout with second precision.
const timestampLayout = "2006-01-02T15:04:05Z"

// Only the canonical text is admitted. The standard time parser alone would also accept fractional
// seconds even though the layout has none.
var timestampPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`)

// Timestamp is an instant serialized as a UTC RFC3339 string with second precision.
// Sub-second precision is discarded on serialization so that an in-memory value and the value
// decoded from its canonical bytes always produce identical digests.
type Timestamp time.Time

// NewTimestamp converts t to a Timestamp normalized to UTC and truncated to whole seconds.
func NewTimestamp(t time.Time) Timestamp {
	defer perf.Track(nil, "artifact.NewTimestamp")()

	return Timestamp(t.UTC().Truncate(time.Second))
}

// Time returns the instant as a time.Time.
//
//nolint:lintroller // Trivial conversion with no I/O.
func (t Timestamp) Time() time.Time { return time.Time(t) }

// IsZero reports whether the timestamp is the zero instant.
//
//nolint:lintroller // Trivial getter with no I/O.
func (t Timestamp) IsZero() bool { return time.Time(t).IsZero() }

// String returns the canonical representation.
//
//nolint:lintroller // Trivial formatter with no I/O.
func (t Timestamp) String() string {
	return time.Time(t).UTC().Truncate(time.Second).Format(timestampLayout)
}

// MarshalJSON implements json.Marshaler using the canonical layout.
func (t Timestamp) MarshalJSON() ([]byte, error) {
	defer perf.Track(nil, "artifact.Timestamp.MarshalJSON")()

	return json.Marshal(t.String())
}

// UnmarshalJSON implements json.Unmarshaler. Only the canonical layout is accepted.
func (t *Timestamp) UnmarshalJSON(data []byte) error {
	defer perf.Track(nil, "artifact.Timestamp.UnmarshalJSON")()

	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("%w: timestamp must be a string: %w", errUtils.ErrArtifactRecordInvalid, err)
	}
	if !timestampPattern.MatchString(raw) {
		return fmt.Errorf("%w: timestamp %q must use the form 2006-01-02T15:04:05Z", errUtils.ErrArtifactRecordInvalid, raw)
	}
	parsed, err := time.Parse(timestampLayout, raw)
	if err != nil {
		return fmt.Errorf("%w: timestamp %q is not a UTC RFC3339 time: %w", errUtils.ErrArtifactRecordInvalid, raw, err)
	}
	*t = Timestamp(parsed)
	return nil
}
