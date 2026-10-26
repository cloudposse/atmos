package sarif

import (
	"path/filepath"
	"strings"

	"github.com/cloudposse/atmos/pkg/hooks"
)

// init registers the shared SARIF parser as the handler for a hook's
// `format: sarif`. This lets a generic `kind: command` hook running any
// SARIF-emitting tool (tfsec, semgrep, gitleaks, snyk, …) get the same
// findings summary, CI annotations, and SARIF upload as the built-in scanner
// kinds — with no Go code. Registering from here (rather than wiring it in
// pkg/hooks) keeps pkg/hooks free of an import cycle with this package.
func init() {
	handler := NewResultHandler(HandlerOptions{
		// Empty Kind → the report is labeled by the SARIF's own tool name.
		OutputPath: customFormatOutputPath,
	})
	if err := hooks.RegisterFormatHandler(hooks.FormatSARIF, handler); err != nil {
		panic("failed to register sarif format handler: " + err.Error())
	}
}

// customFormatOutputPath resolves where a `format: sarif` command wrote its
// SARIF: the hook's `results:` filename inside $ATMOS_OUTPUT_DIR when set,
// otherwise $ATMOS_OUTPUT_FILE.
func customFormatOutputPath(ctx *hooks.ExecContext) string {
	if ctx == nil || ctx.Hook == nil {
		return ""
	}
	if ctx.Hook.Results != "" {
		rel := filepath.Clean(ctx.Hook.Results)
		if filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return ""
		}
		return filepath.Join(ctx.OutputDir, rel)
	}
	return ctx.OutputFile
}
