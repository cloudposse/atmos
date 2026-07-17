// Package validation contains provider-neutral validation diagnostics and their
// common output representations.
package validation

import (
	"sort"
	"strings"

	"github.com/cloudposse/atmos/pkg/ci"
)

// Severity is the normalized severity of a validation finding.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityNotice  Severity = "notice"
)

// Diagnostic is a source location and message emitted by a validator.
type Diagnostic struct {
	Source   string
	RuleID   string
	Severity Severity
	Message  string
	File     string
	Line     int
	Column   int
	// EndLine and EndColumn optionally identify a multi-line finding. They are
	// primarily used by human renderers and CI annotations; zero means the
	// location ends at Line/Column.
	EndLine   int
	EndColumn int
}

// Report is the complete result of one validator invocation.
type Report struct {
	Diagnostics         []Diagnostic
	FilesChecked        int
	Target              string
	RenderedDiagnostics string
}

// HasErrors reports whether the report contains at least one error.
func (r Report) HasErrors() bool {
	for _, diagnostic := range r.Diagnostics {
		if diagnostic.Severity == SeverityError {
			return true
		}
	}
	return false
}

// ToAnnotations maps the report to provider-neutral CI annotations.
func (r Report) ToAnnotations() []ci.Annotation {
	diagnostics := r.sortedDiagnostics()
	annotations := make([]ci.Annotation, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		annotations = append(annotations, ci.Annotation{
			Path:      diagnostic.File,
			StartLine: diagnostic.Line,
			EndLine:   diagnostic.EndLine,
			Level:     annotationLevel(diagnostic.Severity),
			Title:     diagnostic.RuleID,
			Message:   diagnostic.Message,
		})
	}
	return annotations
}

func (r Report) sortedDiagnostics() []Diagnostic {
	diagnostics := append([]Diagnostic(nil), r.Diagnostics...)
	sort.SliceStable(diagnostics, func(i, j int) bool {
		left, right := diagnostics[i], diagnostics[j]
		if left.File != right.File {
			return left.File < right.File
		}
		if left.Line != right.Line {
			return left.Line < right.Line
		}
		if left.RuleID != right.RuleID {
			return left.RuleID < right.RuleID
		}
		return left.Message < right.Message
	})
	return diagnostics
}

func annotationLevel(severity Severity) ci.AnnotationLevel {
	switch strings.ToLower(string(severity)) {
	case string(SeverityNotice):
		return ci.AnnotationNotice
	case string(SeverityWarning):
		return ci.AnnotationWarning
	default:
		return ci.AnnotationError
	}
}
