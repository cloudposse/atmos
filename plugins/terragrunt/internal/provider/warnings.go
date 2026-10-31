package provider

import (
	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
	pluginv1 "github.com/cloudposse/atmos/sdk/proto/atmos/plugin/v1"
)

// warningDiagnostics turns discovery findings into diagnostics the host prints without
// failing the command. The messages never contain evaluated values: only paths and patterns.
func warningDiagnostics(warnings []model.Warning) []*pluginv1.Diagnostic {
	diagnostics := make([]*pluginv1.Diagnostic, 0, len(warnings))
	for _, warning := range warnings {
		diagnostics = append(diagnostics, &pluginv1.Diagnostic{
			Field:   warning.Path,
			Message: warning.Message,
			Detail: &pluginv1.ErrorDetail{
				Code: warning.Code, Summary: warning.Message, UserSafe: true,
				Severity: pluginv1.Severity_SEVERITY_WARNING, Operation: "discover",
				Location: &pluginv1.SourceLocation{File: warning.Path},
			},
		})
	}
	return diagnostics
}
