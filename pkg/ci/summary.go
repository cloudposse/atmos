package ci

import (
	"github.com/cloudposse/atmos/pkg/perf"
)

// WriteStepSummary appends content to the active CI provider's job step
// summary (e.g. GitHub Actions' $GITHUB_STEP_SUMMARY). It is a no-op — returning
// nil — when no CI provider is detected, or when the detected provider does not
// expose a summary destination (its OutputWriter resolves to a NoopOutputWriter
// or a FileOutputWriter with an empty summary path).
//
// This is the seam used by non-CI subsystems (e.g. pkg/hooks scanner output) to
// surface markdown reports in the pipeline run without depending on the
// internal provider types or hard-coding any provider-specific env vars.
func WriteStepSummary(content string) error {
	defer perf.Track(nil, "ci.WriteStepSummary")()

	p := Detect()
	if p == nil {
		return nil
	}
	w := p.OutputWriter()
	if w == nil {
		return nil
	}
	return w.WriteSummary(content)
}
