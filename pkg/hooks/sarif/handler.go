package sarif

import (
	"fmt"
	"os"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/hooks"
)

// HandlerOptions configures how NewResultHandler reads SARIF.
type HandlerOptions struct {
	// Kind is the registered hook kind name (e.g., "checkov", "trivy", "kics").
	Kind string
	// OutputPath returns the file path to read SARIF from given the
	// ExecContext. Use DefaultOutputFile for tools that write to a single
	// file (checkov, trivy). KICS needs a custom path because it writes
	// `results.sarif` inside ATMOS_OUTPUT_DIR rather than to ATMOS_OUTPUT_FILE.
	OutputPath func(ctx *hooks.ExecContext) string
	// MaxFindings caps how many individual findings appear in the markdown
	// table. Defaults to 10 when zero.
	MaxFindings int
}

// NewResultHandler returns a hooks.ResultHandler that reads SARIF from
// opts.OutputPath, parses it via Parse, and produces a Summary with a
// single markdown body used by every consumer (terminal, Pro, PR comments).
func NewResultHandler(opts HandlerOptions) hooks.ResultHandler {
	return func(ctx *hooks.ExecContext) (*hooks.Summary, error) {
		if ctx == nil || opts.OutputPath == nil {
			return nil, nil
		}
		path := opts.OutputPath(ctx)
		if path == "" {
			return nil, nil
		}

		data, err := os.ReadFile(path)
		// runSubprocess always pre-creates the capture file when CaptureStdout is set, so
		// "file exists but is empty" and "file doesn't exist" are the same signal: the
		// scanner produced nothing. missingReportSummary already knows how to turn that
		// into success vs. failure based on ctx.ExitCode.
		if os.IsNotExist(err) || (err == nil && len(data) == 0) {
			return missingReportSummary(ctx, opts)
		}
		if err != nil {
			return nil, fmt.Errorf("%s: read SARIF: %w: %w", labelOrDefault(opts.Kind, ""), errUtils.ErrReadFile, err)
		}

		data = normalizeArtifactURIs(data, ctx)
		findings, err := Parse(data)
		if err != nil {
			return nil, fmt.Errorf("%s: parse SARIF: %w: %w", labelOrDefault(opts.Kind, ""), errUtils.ErrParseFile, err)
		}

		// When Kind is empty (a generic `kind: command` hook using
		// `format: sarif`), fall back to the SARIF's own tool driver name
		// (e.g. "tfsec") so the report is labeled by the actual tool.
		label := labelOrDefault(opts.Kind, findings.Tool)
		body := RenderMarkdown(findings, RenderMarkdownOptions{
			Tool:        label,
			MaxFindings: opts.MaxFindings,
			RepoBaseURL: githubBlobBaseURL(),
		})

		return &hooks.Summary{
			Kind:     label,
			Status:   statusForFindings(findings),
			Title:    titleForFindings(findings),
			Counts:   findings.CountsBySeverity(),
			Body:     body,
			Findings: toHookFindings(findings),
			// Preserve the tool's SARIF fidelity while normalizing artifact
			// paths so annotations and Code Scanning can anchor to PR files.
			SARIF: data,
		}, nil
	}
}

// missingReportSummary reports a scanner failure when it exited unsuccessfully
// without writing SARIF; otherwise, a missing report represents a clean run
// with no findings.
func missingReportSummary(ctx *hooks.ExecContext, opts HandlerOptions) (*hooks.Summary, error) {
	label := labelOrDefault(opts.Kind, "")
	if ctx.ExitCode != 0 {
		return &hooks.Summary{
			Kind:   label,
			Status: hooks.StatusFailure,
			Title:  "scan failed",
			Body:   fmt.Sprintf("## %s\n\n⚠ scanner failed before producing a SARIF report (exit code %d)\n", label, ctx.ExitCode),
		}, nil
	}

	return &hooks.Summary{
		Kind:   label,
		Status: hooks.StatusSuccess,
		Title:  "no findings",
		Body:   fmt.Sprintf("## %s\n\n✅ no findings\n", label),
	}, nil
}

// labelOrDefault resolves the report label: the configured kind, else the
// SARIF tool driver name, else "scan".
func labelOrDefault(kind, toolName string) string {
	if kind != "" {
		return kind
	}
	if toolName != "" {
		return toolName
	}
	return "scan"
}

// toHookFindings maps the parsed SARIF findings into the provider-neutral
// hooks.Finding shape the engine translates to CI annotations. Keeping the
// mapping here (rather than handing sarif.Finding to the engine) avoids
// pkg/hooks importing this subpackage, which would be an import cycle.
func toHookFindings(f *Findings) []hooks.Finding {
	if f == nil || len(f.Findings) == 0 {
		return nil
	}
	out := make([]hooks.Finding, 0, len(f.Findings))
	for _, fd := range f.Findings {
		out = append(out, hooks.Finding{
			Path:     fd.File,
			Line:     fd.Line,
			Severity: fd.Severity.String(),
			RuleID:   fd.RuleID,
			Message:  fd.Message,
		})
	}
	return out
}

// statusForFindings maps the highest finding severity to a Summary status.
// Critical/High → warning, anything else → success. Hooks can promote
// warnings to failures by setting on_failure: fail; the status itself
// only drives the run-page card.
func statusForFindings(f *Findings) hooks.SummaryStatus {
	if f == nil || f.Count() == 0 {
		return hooks.StatusSuccess
	}
	switch f.HighestSeverity() {
	case SeverityCritical, SeverityHigh:
		return hooks.StatusWarning
	default:
		return hooks.StatusSuccess
	}
}

// titleForFindings produces the compact one-line headline shown on the run
// page and PR check (e.g. "2 HIGH, 5 MED"). Falls back to "no findings"
// when there's nothing to report.
func titleForFindings(f *Findings) string {
	if f == nil || f.Count() == 0 {
		return "no findings"
	}
	counts := f.CountsBySeverity()
	parts := make([]string, 0, len(counts))
	for _, sev := range []Severity{SeverityCritical, SeverityHigh, SeverityMedium, SeverityLow, SeverityInfo} {
		name := sev.String()
		if c, ok := counts[name]; ok && c > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", c, severityHeadlineLabel(name)))
		}
	}
	if len(parts) == 0 {
		return "no findings"
	}
	return joinComma(parts)
}

// severityHeadlineLabel is the short uppercase label used in titles.
func severityHeadlineLabel(s string) string {
	switch s {
	case "critical":
		return "CRIT"
	case "high":
		return "HIGH"
	case "medium":
		return "MED"
	case "low":
		return "LOW"
	default:
		return "INFO"
	}
}

func joinComma(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ", "
		}
		out += p
	}
	return out
}

// DefaultOutputFile returns ctx.OutputFile. Used by tools that write a
// single output file (checkov, trivy).
func DefaultOutputFile(ctx *hooks.ExecContext) string {
	if ctx == nil {
		return ""
	}
	return ctx.OutputFile
}
