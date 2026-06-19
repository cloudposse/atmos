package ci

import (
	"context"

	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Annotation and SARIFReport are re-exported from the internal provider package
// so callers outside pkg/ci (e.g. pkg/hooks) can build the values these helpers
// accept without importing an internal package. They are type aliases, so the
// internal types remain the single source of truth.
type (
	// Annotation is a provider-neutral, line-anchored finding.
	Annotation = provider.Annotation
	// AnnotationLevel is the severity bucket an Annotation renders as.
	AnnotationLevel = provider.AnnotationLevel
	// SARIFReport is a request to publish SARIF to the provider's findings store.
	SARIFReport = provider.SARIFReport
)

// Re-exported annotation levels (see provider.AnnotationLevel).
const (
	AnnotationError   = provider.AnnotationError
	AnnotationWarning = provider.AnnotationWarning
	AnnotationNotice  = provider.AnnotationNotice
)

// Annotate renders line-anchored annotations via the active CI provider when it
// implements the Annotator capability (GitHub Actions: `::error`/`::warning`
// shown inline on the PR diff). It is a no-op — returning nil — when no provider
// is detected or the detected provider is not an Annotator. Like
// WriteStepSummary, this is the seam non-CI subsystems (pkg/hooks scanner
// output) use to surface findings without depending on the internal provider
// types or any provider-specific env vars.
func Annotate(annotations []Annotation) error {
	defer perf.Track(nil, "ci.Annotate")()

	if len(annotations) == 0 {
		return nil
	}
	p := Detect()
	if p == nil {
		return nil
	}
	a, ok := p.(provider.Annotator)
	if !ok {
		return nil
	}
	return a.Annotate(annotations)
}

// ReportSARIF publishes a SARIF document to the active CI provider's
// security-findings store when it implements the SARIFReporter capability
// (GitHub: Code Scanning / the Security tab). It is a no-op — returning nil —
// when no provider is detected or the detected provider is not a SARIFReporter.
func ReportSARIF(ctx context.Context, report SARIFReport) error {
	defer perf.Track(nil, "ci.ReportSARIF")()

	if len(report.Body) == 0 {
		return nil
	}
	p := Detect()
	if p == nil {
		return nil
	}
	r, ok := p.(provider.SARIFReporter)
	if !ok {
		return nil
	}
	return r.ReportSARIF(ctx, report)
}
