package model

// Problem codes are stable identifiers shared with the host and with tests.
const (
	CodeUnsupportedSetting = "unsupported_setting"
	CodeSyntaxError        = "syntax_error"
	CodeEvaluationError    = "evaluation_failed"
	CodeEffectDenied       = "effect_denied"
	CodeDependency         = "dependency_error"
	CodeInvalidSource      = "invalid_source"
	CodeInvalidGenerate    = "invalid_generate_path"
	CodeMissingSource      = "missing_terraform_source"
	CodeUnsupportedHook    = "unsupported_hook"
	CodeWorkspace          = "workspace_unsupported"
	CodeInternal           = "plugin_error"
)

// Problem is a rejection the user can act on. Summary, Hints, File, Line, Column, and Key never
// contain evaluated values and are always shown. Detail is shown only when Safe is true.
type Problem struct {
	Code    string
	Summary string
	Detail  string
	Hints   []string
	// File is an absolute path; the provider makes it project relative.
	File         string
	Line, Column int
	// Key names the setting, block, or lifecycle that caused the problem.
	Key  string
	Safe bool
	// Err is the underlying cause. It is never shown to the user.
	Err error
}

// Error returns the headline and detail for local diagnostics and tests. The provider never sends it
// to the host: it sends only the structured fields, honoring Safe.
func (p *Problem) Error() string {
	if p.Detail == "" {
		return p.Summary
	}
	return p.Summary + ": " + p.Detail
}

// Unwrap exposes the cause for classification.
func (p *Problem) Unwrap() error { return p.Err }
