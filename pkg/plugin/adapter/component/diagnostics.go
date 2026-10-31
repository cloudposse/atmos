package component

import (
	"errors"
	"fmt"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosio "github.com/cloudposse/atmos/pkg/io"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
	"github.com/cloudposse/atmos/pkg/ui"
)

// debugHint tells the user how to reveal the masked diagnostic a plugin marked as not safe to print.
const debugHint = "Run with `ATMOS_LOGS_LEVEL=Debug` to log the full diagnostic from the plugin, with secrets masked."

// Failure is a plugin failure that carries the plugin's structured description. Its message is the
// plugin's headline with secrets masked; it unwraps to the Atmos sentinel for the failure class.
type Failure struct {
	Detail   *pluginv1.ErrorDetail
	sentinel error
}

// Error returns the masked headline.
//
//nolint:lintroller // Trivial accessor; exempt from performance tracking.
func (f *Failure) Error() string { return atmosio.MaskString(headline(f.Detail)) }

// Unwrap exposes the sentinel so errors.Is keeps classifying the failure.
//
//nolint:lintroller // Trivial accessor; exempt from performance tracking.
func (f *Failure) Unwrap() error { return f.sentinel }

func headline(detail *pluginv1.ErrorDetail) string {
	if detail.GetSummary() != "" {
		return detail.GetSummary()
	}
	if detail.GetCode() != "" {
		return detail.GetCode()
	}
	return errUtils.ErrPluginAPI.Error()
}

func sentinelFor(code codes.Code) error {
	switch code {
	case codes.Unimplemented:
		return errUtils.ErrPluginUnsupportedExtension
	case codes.InvalidArgument, codes.FailedPrecondition, codes.NotFound:
		return errUtils.ErrPluginInvalidConfig
	default:
		return errUtils.ErrPluginAPI
	}
}

// detailFromStatus extracts the first structured plugin failure carried by a gRPC status error.
func detailFromStatus(err error) (*pluginv1.ErrorDetail, bool) {
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

// DetailError renders a plugin's structured failure with the Atmos error builder: the headline as the
// message, an explanation naming the unit, stack, file, setting, and operation, the plugin's hints, and
// the detail text when the plugin marked it safe. Detail text that is not safe stays out of the error
// and goes to the masked debug log. Every field passes through secret masking before display.
func DetailError(detail *pluginv1.ErrorDetail, class codes.Code) error {
	defer perf.Track(nil, "componentadapter.DetailError")()

	if detail == nil {
		return errUtils.ErrPluginAPI
	}
	builder := errUtils.Build(&Failure{Detail: detail, sentinel: sentinelFor(class)})
	if where := describeWhere(detail); where != "" {
		builder.WithExplanation(where)
	}
	if detail.UserSafe && detail.Detail != "" {
		builder.WithExplanation(mask(detail.Detail))
	}
	for _, hint := range detail.Hints {
		builder.WithHint(mask(hint))
	}
	if !detail.UserSafe && detail.Detail != "" {
		log.Debug("Plugin diagnostic", "code", detail.Code, "unit", detail.Unit, "stack", detail.Stack, "operation", detail.Operation, "detail", mask(detail.Detail))
		builder.WithHint(debugHint)
	}
	if location := detail.GetLocation(); location.GetFile() != "" {
		builder.WithLocation(location.File, int(location.Line))
	}
	return builder.WithContext("plugin_error_code", detail.Code).Err()
}

func mask(text string) string { return atmosio.MaskString(text) }

// describeWhere states where the problem is, using only the always-safe fields.
func describeWhere(detail *pluginv1.ErrorDetail) string {
	var parts []string
	if detail.Unit != "" {
		unit := fmt.Sprintf("Component `%s`", detail.Unit)
		if detail.Stack != "" {
			unit += fmt.Sprintf(" in stack `%s`", detail.Stack)
		}
		parts = append(parts, unit)
	}
	if file := detail.GetLocation().GetFile(); file != "" {
		parts = append(parts, "defined in `"+formatLocation(detail.Location)+"`")
	}
	if detail.ConfigKey != "" {
		parts = append(parts, fmt.Sprintf("setting `%s`", detail.ConfigKey))
	}
	if detail.Operation != "" {
		parts = append(parts, "during "+detail.Operation)
	}
	return mask(strings.Join(parts, ", "))
}

func formatLocation(location *pluginv1.SourceLocation) string {
	text := location.File
	if location.Line > 0 {
		text += fmt.Sprintf(":%d", location.Line)
		if location.Column > 0 {
			text += fmt.Sprintf(":%d", location.Column)
		}
	}
	return text
}

// DiagnosticError converts one plugin diagnostic into an error, or nil for a warning.
func DiagnosticError(diagnostic *pluginv1.Diagnostic) error {
	defer perf.Track(nil, "componentadapter.DiagnosticError")()

	detail := diagnostic.GetDetail()
	if detail == nil {
		// The legacy form carries a field and a message the plugin may not have vetted.
		detail = &pluginv1.ErrorDetail{Code: "invalid_configuration", Summary: "Plugin rejected the configuration", ConfigKey: diagnostic.GetField()}
	}
	if detail.Severity == pluginv1.Severity_SEVERITY_WARNING {
		return nil
	}
	return DetailError(detail, codes.InvalidArgument)
}

// firstDiagnosticError returns the first error-severity diagnostic as an error. Warnings are
// shown on the UI channel and never fail the call.
func firstDiagnosticError(diagnostics []*pluginv1.Diagnostic) error {
	var failure error
	for _, diagnostic := range diagnostics {
		if err := DiagnosticError(diagnostic); err != nil {
			if failure == nil {
				failure = err
			}
			continue
		}
		WarnDiagnostic(diagnostic)
	}
	return failure
}

// WarnDiagnostic prints a warning-severity diagnostic on the UI channel with secrets masked.
func WarnDiagnostic(diagnostic *pluginv1.Diagnostic) {
	defer perf.Track(nil, "componentadapter.WarnDiagnostic")()

	detail := diagnostic.GetDetail()
	if detail == nil || detail.Severity != pluginv1.Severity_SEVERITY_WARNING {
		return
	}
	text := headline(detail)
	if where := describeWhere(detail); where != "" {
		text += " (" + where + ")"
	}
	if detail.UserSafe && detail.Detail != "" {
		text += ": " + detail.Detail
	}
	ui.Warning(mask(text))
	for _, hint := range detail.Hints {
		ui.Hint(mask(hint))
	}
}
