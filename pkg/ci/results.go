package ci

import (
	"context"
	"errors"
	"fmt"
	"strings"

	cockroach "github.com/cockroachdb/errors"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/policy"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/markdown"
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
	if err := a.Annotate(annotations); err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrCIAnnotationFailed, err)
	}
	return nil
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
	if err := r.ReportSARIF(ctx, report); err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrCISARIFUploadFailed, err)
	}
	return nil
}

// plainReasonWidth keeps policy reasons on their authored lines; annotations wrap in the UI.
const plainReasonWidth = 4096

// defaultFailureTitle titles a failure annotation when the error has no custom title.
const defaultFailureTitle = "Atmos command failed"

// AnnotateFailure reports the error that failed a command as annotations through the
// active CI provider's Annotator, so it works with any provider that implements one.
// A policy denial produces one annotation per denying rule. Any other error produces
// one annotation from its title, message, and explanation. A location recorded with
// errors.ErrorBuilder.WithLocation anchors the annotations to that file and line.
//
// It skips errors that are not command failures: silent exit-code carriers,
// interrupts, and failures of nested Atmos commands, which already reported their
// own failure. When configuration is loaded, ci.annotations.enabled applies. Before
// configuration loads, provider detection alone decides. It is a no-op when no
// annotating provider is detected.
func AnnotateFailure(atmosConfig *schema.AtmosConfiguration, err error) error {
	defer perf.Track(atmosConfig, "ci.AnnotateFailure")()

	if !isAnnotatableFailure(err) {
		return nil
	}
	if atmosConfig != nil && atmosConfig.Initialized && !AnnotationsEnabled(atmosConfig) {
		return nil
	}
	location, _ := errUtils.GetLocation(err)
	if violations := policy.Violations(err); len(violations) > 0 {
		return Annotate(policyAnnotations(violations, "", location))
	}
	return Annotate([]Annotation{failureAnnotation(err, location)})
}

// AnnotatePolicyDenial renders one error-level annotation per rule that denied err,
// titled with the rule and the denied subject (for example "terraform destroy").
// It needs no Atmos configuration. A location recorded on err anchors the
// annotations. It is a no-op when err is not a policy denial or no annotating CI
// provider is detected.
func AnnotatePolicyDenial(err error, subject string) error {
	defer perf.Track(nil, "ci.AnnotatePolicyDenial")()

	violations := policy.Violations(err)
	if len(violations) == 0 {
		return nil
	}
	location, _ := errUtils.GetLocation(err)
	return Annotate(policyAnnotations(violations, subject, location))
}

// isAnnotatableFailure reports whether err is a command failure worth annotating.
func isAnnotatableFailure(err error) bool {
	// The nested-command marker is a cockroachdb mark, which only cockroach.Is matches.
	if err == nil || cockroach.Is(err, errUtils.ErrNestedAtmosFailed) {
		return false
	}
	var exitCodeErr errUtils.ExitCodeError
	if errors.As(err, &exitCodeErr) && (exitCodeErr.Silent || exitCodeErr.Code == 0) {
		return false
	}
	return errUtils.GetExitCode(err) != errUtils.ExitCodeSIGINT
}

// policyAnnotations builds one error-level annotation per denying rule.
func policyAnnotations(violations []policy.Violation, subject string, location errUtils.Location) []Annotation {
	annotations := make([]Annotation, 0, len(violations))
	for _, violation := range violations {
		title := "Policy " + violation.RuleID + " denied execution"
		if subject != "" {
			title = "Policy " + violation.RuleID + " denied " + subject
		}
		annotations = append(annotations, Annotation{
			Path:      location.File,
			StartLine: location.Line,
			Level:     AnnotationError,
			Title:     title,
			Message:   plainPolicyReason(violation),
		})
	}
	return annotations
}

// failureAnnotation builds one error-level annotation from an error's title, message,
// and explanation.
func failureAnnotation(err error, location errUtils.Location) Annotation {
	title := errUtils.GetTitle(err)
	if title == "" {
		title = defaultFailureTitle
	}
	message := err.Error()
	for _, detail := range cockroach.GetAllDetails(err) {
		if plain := plainMarkdown(detail); plain != "" {
			message += "\n\n" + plain
		}
	}
	return Annotation{
		Path:      location.File,
		StartLine: location.Line,
		Level:     AnnotationError,
		Title:     title,
		Message:   message,
	}
}

// plainPolicyReason renders an author's Markdown reason as plain text, falling back to
// naming the rule when the reason is empty.
func plainPolicyReason(violation policy.Violation) string {
	if plain := plainMarkdown(violation.Reason); plain != "" {
		return plain
	}
	return "Denied by " + violation.RuleID + "."
}

// plainMarkdown renders Markdown as plain text for annotation messages, falling back
// to the source text when it cannot be rendered.
func plainMarkdown(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	renderer, err := markdown.NewRenderer(schema.AtmosConfiguration{}, markdown.WithWidth(plainReasonWidth))
	if err != nil {
		return content
	}
	plain, err := renderer.RenderReadable(content)
	if err != nil || strings.TrimSpace(plain) == "" {
		return content
	}
	return strings.TrimSpace(plain)
}
