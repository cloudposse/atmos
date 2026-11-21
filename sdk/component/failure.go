package component

import (
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	pluginv1 "github.com/cloudposse/atmos/sdk/proto/atmos/plugin/v1"
)

// Failure builds the structured, user-facing description of a plugin problem.
//
// The host renders a Failure with its error builder: a title, an explanation, hints, the
// authored file and position, and the unit and stack. Everything except the detail text is
// always shown, so it must never contain evaluated values. Detail text is shown only after
// SafeDetail; Detail marks it as possibly secret and the host then keeps it in the masked
// debug log.
//
//nolint:lintroller // Pure SDK helper; the standalone SDK cannot import host perf tracking.
type Failure struct {
	detail *pluginv1.ErrorDetail
}

// NewFailure starts a failure with a stable machine-readable code and a one-line headline.
func NewFailure(code, summary string) *Failure {
	return &Failure{detail: &pluginv1.ErrorDetail{Code: code, Summary: summary, Severity: pluginv1.Severity_SEVERITY_ERROR}}
}

// Detail sets explanatory text that may contain evaluated values. The host does not print it.
func (f *Failure) Detail(text string) *Failure {
	f.detail.Detail, f.detail.UserSafe = text, false
	return f
}

// SafeDetail sets explanatory text that contains no evaluated values, so the host prints it.
func (f *Failure) SafeDetail(text string) *Failure {
	f.detail.Detail, f.detail.UserSafe = text, true
	return f
}

// Hint appends an actionable remedy. Hints are shown in order.
func (f *Failure) Hint(text string) *Failure {
	f.detail.Hints = append(f.detail.Hints, text)
	return f
}

// At records the authored position; line and column are 1-based and zero means unknown.
func (f *Failure) At(file string, line, column uint32) *Failure {
	f.detail.Location = &pluginv1.SourceLocation{File: file, Line: line, Column: column}
	return f
}

// Unit names the component and the stack it belongs to, in Atmos terms.
func (f *Failure) Unit(stack, name string) *Failure {
	f.detail.Stack, f.detail.Unit = stack, name
	return f
}

// Key names the setting or block that caused the failure.
func (f *Failure) Key(key string) *Failure {
	f.detail.ConfigKey = key
	return f
}

// Operation names the failing step, such as "discover" or "prepare".
func (f *Failure) Operation(operation string) *Failure {
	f.detail.Operation = operation
	return f
}

// Warning downgrades the failure so the host prints it without failing the command.
func (f *Failure) Warning() *Failure {
	f.detail.Severity = pluginv1.Severity_SEVERITY_WARNING
	return f
}

// Proto returns an independent copy of the wire form.
func (f *Failure) Proto() *pluginv1.ErrorDetail {
	return proto.Clone(f.detail).(*pluginv1.ErrorDetail)
}

// Diagnostic returns the failure as a per-item diagnostic that does not fail the call.
func (f *Failure) Diagnostic() *pluginv1.Diagnostic {
	return &pluginv1.Diagnostic{Field: f.detail.ConfigKey, Message: f.detail.Summary, Detail: f.Proto()}
}

// Err returns a gRPC status error carrying the failure as a status detail.
func (f *Failure) Err(code codes.Code) error {
	withDetail, err := status.New(code, f.detail.Summary).WithDetails(f.Proto())
	if err != nil {
		// A detail that cannot be attached must still fail the call with its headline.
		return status.Error(code, f.detail.Summary)
	}
	return withDetail.Err()
}

// FailureDetail extracts the structured failure from an error, looking through wrapping.
func FailureDetail(err error) (*pluginv1.ErrorDetail, bool) {
	if err == nil {
		return nil, false
	}
	var carrier interface{ GRPCStatus() *status.Status }
	if !errors.As(err, &carrier) {
		return nil, false
	}
	for _, item := range carrier.GRPCStatus().Details() {
		if detail, ok := item.(*pluginv1.ErrorDetail); ok {
			return detail, true
		}
	}
	return nil, false
}
