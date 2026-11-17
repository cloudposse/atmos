package artifacts

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	sigsyaml "sigs.k8s.io/yaml"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/ui"
)

// result is the structured result of one operation. The operation fields flatten into the top level
// with their named-output spelling (ref, location, resolved_digest, record_digest, content_digest,
// representation_digest, size, outcome), exactly like a step result, and the command-specific parts
// follow. JSON and YAML share these names; the display format never changes semantics.
type result struct {
	artifact.OperationResult

	// Error is set when the operation failed. The envelope is still emitted, and the exit code is nonzero.
	Error string `json:"error,omitempty"`

	// Record is the sealed Record of a publication (set for dry runs and publications).
	Record *artifact.Record `json:"record,omitempty"`

	// Inspection is the metadata view of an inspected artifact.
	Inspection *artifact.Inspection `json:"inspection,omitempty"`

	// Verification is the authenticated verification result; absent means unchecked.
	Verification *artifact.VerificationResult `json:"verification,omitempty"`

	// Subject is the signed or verified subject.
	Subject *artifact.SignatureSubject `json:"subject,omitempty"`

	// Evidence lists the evidence stored by sign.
	Evidence []artifact.SignatureEvidence `json:"evidence,omitempty"`

	// Document is the descriptor of a shown document.
	Document *artifact.Document `json:"document,omitempty"`

	// Content is the document text of show.
	Content *string `json:"content,omitempty"`
}

// failed fills the failure markers of an envelope. The service leaves the outcome empty on failure
// so a failure is never mistaken for success; the envelope reports it explicitly.
func (r *result) failed(err error, action artifact.Action, repository, requested string) {
	if err == nil {
		return
	}
	r.Error = err.Error()
	r.Outcome = artifact.OutcomeFailed
	if r.Action == "" {
		r.Action = action
	}
	if r.Repository == "" {
		r.Repository = repository
	}
	if r.Requested == "" {
		r.Requested = requested
	}
}

// joinValues renders a format list for flag help and errors.
func joinValues(values []string) string {
	return strings.Join(values, ", ")
}

// validateFormat rejects a format a command does not support before any work is done.
func validateFormat(value string, valid []string) error {
	if slices.Contains(valid, value) {
		return nil
	}
	return errUtils.Build(fmt.Errorf("%w: unsupported format %q; supported formats: %s", errUtils.ErrInvalidFlag, value, joinValues(valid))).
		WithHintf("Use `--format=%s`.", valid[0]).
		WithExitCode(2).
		Err()
}

// isStructured reports whether the format is machine-readable.
func isStructured(format string) bool {
	return format == formatJSON || format == formatYAML
}

// writeStructured writes a value as JSON or YAML to the data channel (stdout). YAML is derived from
// the JSON encoding, so both share the same field names and number formatting.
func writeStructured(format string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s output: %w", format, err)
	}
	if format == formatYAML {
		converted, err := sigsyaml.JSONToYAML(raw)
		if err != nil {
			return fmt.Errorf("encode yaml output: %w", err)
		}
		return data.Write(string(converted))
	}
	return data.Writeln(string(raw))
}

// keyValue is one line of the text rendering.
type keyValue struct {
	key   string
	value string
}

// writeText writes aligned `key: value` lines to the data channel, skipping empty values so a
// missing optional value is omitted, never filled with a misleading empty string.
func writeText(pairs []keyValue) error {
	width := 0
	for _, pair := range pairs {
		if pair.value != "" && len(pair.key) > width {
			width = len(pair.key)
		}
	}
	var builder strings.Builder
	for _, pair := range pairs {
		if pair.value == "" {
			continue
		}
		fmt.Fprintf(&builder, "%-*s %s\n", width+1, pair.key+":", pair.value)
	}
	return data.Write(builder.String())
}

// operationPairs are the named outputs shared by every operation, in a stable order.
func operationPairs(op *artifact.OperationResult) []keyValue {
	size := ""
	if op.Size > 0 {
		size = fmt.Sprintf("%d", op.Size)
	}
	return []keyValue{
		{"outcome", string(op.Outcome)},
		{"ref", op.Ref},
		{"location", op.Location},
		{"resolved_digest", string(op.ResolvedDigest)},
		{"record_digest", string(op.RecordDigest)},
		{"content_digest", string(op.ContentDigest)},
		{"representation_digest", string(op.RepresentationDigest)},
		{"size", size},
		{"lock_scope", string(op.LockScope)},
	}
}

// reportDiagnostics sends the non-fatal messages of an operation to the UI channel (stderr).
func reportDiagnostics(diagnostics []artifact.Diagnostic) {
	for index := range diagnostics {
		diagnostic := diagnostics[index]
		switch diagnostic.Severity {
		case artifact.SeverityWarning:
			ui.Warning(diagnostic.Message)
		case artifact.SeverityInfo:
			ui.Info(diagnostic.Message)
		case artifact.SeverityError:
			// The error itself is returned and rendered once by the error path.
		}
	}
}

// emit renders a result. Structured formats always render, including on failure; the text format
// renders only a successful result, because a failure is reported by the returned error.
func emit(format string, res *result, failed bool, text func(*result) error) error {
	if isStructured(format) {
		return writeStructured(format, res)
	}
	if failed {
		return nil
	}
	reportDiagnostics(res.Diagnostics)
	return text(res)
}

// textOperation is the default rendering of a plain operation result.
func textOperation(res *result) error {
	return writeText(operationPairs(&res.OperationResult))
}
