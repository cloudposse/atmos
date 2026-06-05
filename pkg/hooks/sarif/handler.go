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
		if os.IsNotExist(err) {
			// Tool didn't produce a SARIF file (clean run, no findings).
			return &hooks.Summary{
				Kind:   opts.Kind,
				Status: hooks.StatusSuccess,
				Title:  "no findings",
				Body:   fmt.Sprintf("## %s\n\n✅ no findings\n", opts.Kind),
			}, nil
		}
		if err != nil {
			return nil, fmt.Errorf("%s: read SARIF: %w: %w", opts.Kind, errUtils.ErrReadFile, err)
		}

		findings, err := Parse(data)
		if err != nil {
			return nil, fmt.Errorf("%s: parse SARIF: %w: %w", opts.Kind, errUtils.ErrParseFile, err)
		}

		body := RenderMarkdown(findings, RenderMarkdownOptions{
			Tool:        opts.Kind,
			MaxFindings: opts.MaxFindings,
		})

		return &hooks.Summary{
			Kind:   opts.Kind,
			Status: statusForFindings(findings),
			Title:  titleForFindings(findings),
			Counts: findings.CountsBySeverity(),
			Body:   body,
		}, nil
	}
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
