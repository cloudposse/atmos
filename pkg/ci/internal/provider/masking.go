package provider

import (
	atmosio "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/perf"
)

// MaskPublishedContent applies the global secret masker before CI content
// leaves the process through a human-facing provider surface.
func MaskPublishedContent(content string) string {
	defer perf.Track(nil, "provider.MaskPublishedContent")()

	return atmosio.MaskString(content)
}
