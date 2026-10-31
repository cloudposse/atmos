package artifact

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Label limits. Labels must be expressible and selectable with `--labels 'key=value,key2=value2'`,
// which splits pairs on ',' and each pair on the first '=' or ':'.
const (
	// MaxLabelKeyLength is the longest label key, in bytes.
	MaxLabelKeyLength = 128

	// MaxLabelValueLength is the longest label value, in bytes.
	MaxLabelValueLength = 1024

	// MaxLabels is the largest number of labels on one artifact version or repository default.
	MaxLabels = 64
)

// ValidateLabel checks one exact key/value label. The key must be non-empty, free of control
// characters and surrounding whitespace, and free of '=', ':' and ',' so that it can be selected
// with `--labels`; the value must be free of control characters and ','.
func ValidateLabel(key, value string) error {
	defer perf.Track(nil, "artifact.ValidateLabel")()

	switch {
	case key == "":
		return fmt.Errorf("%w: label key must not be empty", errUtils.ErrArtifactConfigInvalid)
	case len(key) > MaxLabelKeyLength:
		return fmt.Errorf("%w: label key %q exceeds %d bytes", errUtils.ErrArtifactConfigInvalid, key, MaxLabelKeyLength)
	case strings.TrimSpace(key) != key:
		return fmt.Errorf("%w: label key %q must not have leading or trailing whitespace", errUtils.ErrArtifactConfigInvalid, key)
	case strings.ContainsAny(key, "=:,"):
		return fmt.Errorf("%w: label key %q must not contain '=', ':' or ',' (they delimit --labels selectors)", errUtils.ErrArtifactConfigInvalid, key)
	case strings.IndexFunc(key, unicode.IsControl) >= 0:
		return fmt.Errorf("%w: label key %q must not contain control characters", errUtils.ErrArtifactConfigInvalid, key)
	case len(value) > MaxLabelValueLength:
		return fmt.Errorf("%w: label %q value exceeds %d bytes", errUtils.ErrArtifactConfigInvalid, key, MaxLabelValueLength)
	case strings.Contains(value, ","):
		return fmt.Errorf("%w: label %q value must not contain ',' (it delimits --labels selectors)", errUtils.ErrArtifactConfigInvalid, key)
	case strings.IndexFunc(value, unicode.IsControl) >= 0:
		return fmt.Errorf("%w: label %q value must not contain control characters", errUtils.ErrArtifactConfigInvalid, key)
	}
	return nil
}

// ValidateLabels checks a label set, returning every malformed label joined in key order.
func ValidateLabels(labels map[string]string) error {
	defer perf.Track(nil, "artifact.ValidateLabels")()

	var errs []error
	if len(labels) > MaxLabels {
		errs = append(errs, fmt.Errorf("%w: %d labels exceed the limit of %d", errUtils.ErrArtifactConfigInvalid, len(labels), MaxLabels))
	}
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if err := ValidateLabel(key, labels[key]); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
