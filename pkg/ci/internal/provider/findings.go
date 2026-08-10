package provider

import "context"

// AnnotationLevel is the severity bucket a CI annotation renders as. The
// values mirror the three levels GitHub Actions workflow commands support
// (`::error` / `::warning` / `::notice`); other providers map them to their
// nearest equivalent.
type AnnotationLevel string

const (
	// AnnotationError is the highest level (GitHub: ::error).
	AnnotationError AnnotationLevel = "error"
	// AnnotationWarning is the middle level (GitHub: ::warning).
	AnnotationWarning AnnotationLevel = "warning"
	// AnnotationNotice is the lowest level (GitHub: ::notice).
	AnnotationNotice AnnotationLevel = "notice"
)

// Annotation is a provider-neutral, line-anchored finding. Scanner hooks build
// these from parsed SARIF findings and hand them to the active provider's
// Annotator; the provider renders them inline on the change (GitHub: workflow
// annotations on the PR diff). StartLine is 1-based; 0 means "unknown", in
// which case the annotation anchors at the file level.
type Annotation struct {
	Path      string
	StartLine int
	EndLine   int
	Level     AnnotationLevel
	Title     string
	Message   string
}

// Annotator is an optional capability for providers that can render
// line-anchored annotations in the current run (GitHub Actions: `::error` /
// `::warning` workflow commands shown inline on the PR diff). Providers
// implement this when their platform surfaces per-line annotations from log
// output without needing GitHub Advanced Security.
type Annotator interface {
	// Annotate renders the given annotations in the current run. It is
	// best-effort from the caller's perspective: a returned error is logged,
	// never fatal.
	Annotate(annotations []Annotation) error
}

// SARIFReport is a provider-neutral request to publish a SARIF document to the
// provider's security-findings store. Body is the raw SARIF 2.1.0 bytes the
// scanner produced (passed through unmodified for full fidelity); Category
// identifies the analysis so multiple uploads for the same commit (e.g. one
// per component) coexist instead of overwriting each other.
type SARIFReport struct {
	Body     []byte
	Category string
}

// SARIFReporter is an optional capability for providers that ingest SARIF into
// a security-findings store (GitHub: Code Scanning / the Security tab — which
// requires GitHub Advanced Security on private repos). Providers implement this
// when their platform exposes a documented SARIF ingestion endpoint.
type SARIFReporter interface {
	// ReportSARIF uploads the SARIF document to the provider's findings store.
	// Best-effort from the caller's perspective; a returned error is logged,
	// never fatal.
	ReportSARIF(ctx context.Context, report SARIFReport) error
}
