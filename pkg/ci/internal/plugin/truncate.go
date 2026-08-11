package plugin

import (
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// DetailOutputMaxBytes caps the size of a detail block embedded in a CI job
	// summary, keeping the total well under the platform limit (e.g. GitHub's
	// ~1 MiB job summary).
	DetailOutputMaxBytes = 12 * 1024
	// DetailLineBacktrackMaxBytes bounds how far truncation scans backward to
	// align the cut to a line boundary.
	DetailLineBacktrackMaxBytes = 4 * 1024
	// DetailTruncatedPrefix marks output that was truncated to its tail.
	DetailTruncatedPrefix = "... output truncated ...\n"
)

// TruncateDetail caps long detail output while preserving the tail, aligning the
// cut to a line boundary where possible. Shared across CI plugins (terraform,
// kubernetes) so the cap and marker stay identical.
func TruncateDetail(value string) string {
	defer perf.Track(nil, "plugin.TruncateDetail")()

	value = strings.TrimSpace(value)
	if len(value) <= DetailOutputMaxBytes {
		return value
	}
	start := len(value) - DetailOutputMaxBytes
	if prev := strings.LastIndexByte(value[:start], '\n'); prev >= 0 && start-prev <= DetailLineBacktrackMaxBytes {
		start = prev + 1
	} else if next := strings.IndexByte(value[start:], '\n'); next >= 0 {
		start += next + 1
	}
	return DetailTruncatedPrefix + strings.TrimLeft(value[start:], "\r\n")
}
