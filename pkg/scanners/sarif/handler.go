package sarif

import (
	"fmt"
	"os"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/scanners"
)

// HandlerOptions configures how NewResultHandler reads SARIF.
type HandlerOptions struct {
	// Kind is the registered hook kind name (e.g., "checkov", "trivy", "kics").
	Kind string
	// OutputPath returns the file path to read SARIF from given the
	// ExecContext. Use DefaultOutputFile for tools that write to a single
	// file (checkov, trivy). KICS needs a custom path because it writes
	// `results.sarif` inside ATMOS_OUTPUT_DIR rather than to ATMOS_OUTPUT_FILE.
	OutputPath func(ctx *scanners.Context) string
	// MaxFindings caps how many individual findings appear in the markdown
	// table. Defaults to 10 when zero.
	MaxFindings int
}

// NewResultHandler returns a scanners.ResultHandler that reads SARIF from
// opts.OutputPath, parses it via Parse, and produces a Summary with a
// single markdown body used by every consumer (terminal, Pro, PR comments).
func NewResultHandler(opts HandlerOptions) scanners.ResultHandler {
	defer perf.Track(nil, "sarif.NewResultHandler")()

	return func(ctx *scanners.Context) (*scanners.Summary, error) {
		if ctx == nil || opts.OutputPath == nil {
			return nil, nil
		}
		path := opts.OutputPath(ctx)
		if path == "" {
			return nil, nil
		}

		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			// Tool didn't produce a SARIF file (clean run, no findings).
			label := labelOrDefault(opts.Kind, "")
			return &scanners.Summary{
				Kind:   label,
				Status: scanners.StatusSuccess,
				Title:  "no findings",
				Body:   fmt.Sprintf("## %s\n\n✅ no findings\n", label),
			}, nil
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
		})

		return &scanners.Summary{
			Kind:     label,
			Status:   statusForFindings(findings),
			Title:    titleForFindings(findings),
			Counts:   findings.CountsBySeverity(),
			Body:     body,
			Findings: toScannerFindings(findings),
			// Preserve the tool's SARIF fidelity while normalizing artifact
			// paths so annotations and Code Scanning can anchor to PR files.
			SARIF: data,
		}, nil
	}
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

// toScannerFindings maps parsed SARIF findings into the provider-neutral
// scanners.Finding shape the runner translates to CI annotations.
func toScannerFindings(f *Findings) []scanners.Finding {
	if f == nil || len(f.Findings) == 0 {
		return nil
	}
	out := make([]scanners.Finding, 0, len(f.Findings))
	for _, fd := range f.Findings {
		out = append(out, scanners.Finding{
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
func statusForFindings(f *Findings) scanners.SummaryStatus {
	if f == nil || f.Count() == 0 {
		return scanners.StatusSuccess
	}
	switch f.HighestSeverity() {
	case SeverityCritical, SeverityHigh:
		return scanners.StatusWarning
	default:
		return scanners.StatusSuccess
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
func DefaultOutputFile(ctx *scanners.Context) string {
	defer perf.Track(nil, "sarif.DefaultOutputFile")()

	if ctx == nil {
		return ""
	}
	return ctx.OutputFile
}
