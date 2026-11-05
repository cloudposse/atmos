package script

import (
	"errors"
	"slices"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Diagnostic carries authored error metadata through module, retry and parallel
// wrappers. It is immutable; enrichment is applied only at the reporting boundary.
// Language bindings validate and snapshot arguments before constructing updates.
type Diagnostic struct {
	message string
	enrich  []func(*errUtils.ErrorBuilder)
	causes  []error
}

// NewDiagnostic creates an authored error without a language-specific wrapper.
func NewDiagnostic(message string) *Diagnostic {
	defer perf.Track(nil, "script.NewDiagnostic")()
	return &Diagnostic{message: message}
}

// Message returns the authored message without its cause chain.
func (d *Diagnostic) Message() string {
	defer perf.Track(nil, "script.Diagnostic.Message")()
	return d.message
}

// With returns a new diagnostic with an additional metadata update.
// The update must capture immutable values and must not mutate shared state.
func (d *Diagnostic) With(enrich func(*errUtils.ErrorBuilder)) *Diagnostic {
	defer perf.Track(nil, "script.Diagnostic.With")()

	return &Diagnostic{message: d.message, enrich: append(slices.Clone(d.enrich), enrich), causes: d.causes}
}

// WithCause returns a new diagnostic preserving the original cause chain.
func (d *Diagnostic) WithCause(cause error) *Diagnostic {
	defer perf.Track(nil, "script.Diagnostic.WithCause")()

	return &Diagnostic{message: d.message, enrich: d.enrich, causes: append(slices.Clone(d.causes), cause)}
}

// Err creates a distinct raised error from this immutable diagnostic template.
// Separate raises retain their own identity when parallel failures are aggregated.
func (d *Diagnostic) Err() error {
	defer perf.Track(nil, "script.Diagnostic.Err")()

	return &Diagnostic{message: d.message, enrich: d.enrich, causes: d.causes}
}

// Error returns the message followed by its authored causes.
func (d *Diagnostic) Error() string {
	defer perf.Track(nil, "script.Diagnostic.Error")()
	message := d.message
	for _, cause := range d.causes {
		if cause != nil {
			message += ": " + cause.Error()
		}
	}
	return message
}

// Unwrap exposes causes without allowing callers to mutate the diagnostic.
func (d *Diagnostic) Unwrap() []error {
	defer perf.Track(nil, "script.Diagnostic.Unwrap")()
	return slices.Clone(d.causes)
}

// EnrichDiagnostics applies metadata innermost first, visiting each diagnostic
// once. Outer settings take precedence; distinct parallel failures retain theirs.
//
//nolint:errorlint // Visit immediate nodes rather than recursively matching the same descendant.
func EnrichDiagnostics(builder *errUtils.ErrorBuilder, err error) {
	defer perf.Track(nil, "script.EnrichDiagnostics")()
	seen := make(map[*Diagnostic]bool)
	var visit func(error)
	visit = func(err error) {
		if err == nil {
			return
		}
		diagnostic, ok := err.(*Diagnostic)
		if ok {
			if seen[diagnostic] {
				return
			}
			seen[diagnostic] = true
		}
		if many, ok := err.(interface{ Unwrap() []error }); ok {
			for _, cause := range many.Unwrap() {
				visit(cause)
			}
		} else {
			visit(errors.Unwrap(err))
		}
		if diagnostic != nil {
			for _, enrich := range diagnostic.enrich {
				enrich(builder)
			}
		}
	}
	visit(err)
}
