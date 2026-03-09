package security

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/perf"
)

// ReportRenderer renders security and compliance reports in various formats.
type ReportRenderer interface {
	// RenderSecurityReport renders a security findings report.
	RenderSecurityReport(w io.Writer, report *Report) error

	// RenderComplianceReport renders a compliance posture report.
	RenderComplianceReport(w io.Writer, report *ComplianceReport) error
}

// NewReportRenderer creates a renderer for the given output format.
func NewReportRenderer(format OutputFormat) ReportRenderer {
	defer perf.Track(nil, "security.NewReportRenderer")()
	switch format {
	case FormatJSON:
		return &jsonRenderer{}
	case FormatYAML:
		return &yamlRenderer{}
	case FormatCSV:
		return &csvRenderer{}
	default:
		return &markdownRenderer{}
	}
}

// markdownRenderer renders reports as rich Markdown for terminal display.
type markdownRenderer struct{}

func (r *markdownRenderer) RenderSecurityReport(w io.Writer, report *Report) error {
	var sb strings.Builder

	fmt.Fprintf(&sb, "# Security Report: %s\n\n", reportTarget(report.Stack, report.Component))
	fmt.Fprintf(&sb, "**Generated:** %s\n", report.GeneratedAt.Format(time.RFC3339))
	if report.Stack != "" {
		fmt.Fprintf(&sb, "**Stack:** %s\n", report.Stack)
	}
	fmt.Fprintf(&sb, "**Findings:** %d", report.TotalFindings)
	if len(report.SeverityCounts) > 0 {
		counts := severityCountsString(report.SeverityCounts)
		fmt.Fprintf(&sb, " (%s)", counts)
	}
	sb.WriteString("\n\n---\n\n")

	// Group findings by severity.
	for _, sev := range []Severity{SeverityCritical, SeverityHigh, SeverityMedium, SeverityLow, SeverityInformational} {
		findings := filterBySeverity(report.Findings, sev)
		if len(findings) == 0 {
			continue
		}
		fmt.Fprintf(&sb, "## %s Findings (%d)\n\n", sev, len(findings))
		for i := range findings {
			renderFindingMarkdown(&sb, &findings[i], i+1)
		}
	}

	// Summary table.
	sb.WriteString("## Summary\n\n")
	sb.WriteString("| Severity | Count | Mapped | Unmapped |\n")
	sb.WriteString("|----------|-------|--------|----------|\n")
	for _, sev := range []Severity{SeverityCritical, SeverityHigh, SeverityMedium, SeverityLow} {
		count := report.SeverityCounts[sev]
		if count == 0 {
			continue
		}
		mapped, unmapped := countMappedBySeverity(report.Findings, sev)
		fmt.Fprintf(&sb, "| %s | %d | %d | %d |\n", sev, count, mapped, unmapped)
	}
	fmt.Fprintf(&sb, "| **Total** | **%d** | **%d** | **%d** |\n\n",
		report.TotalFindings, report.MappedCount, report.UnmappedCount)

	if report.UnmappedCount > 0 {
		fmt.Fprintf(&sb, "> %d findings could not be mapped to Atmos components. "+
			"These resources may be managed outside of Atmos or may be missing `atmos:*` tags.\n\n",
			report.UnmappedCount)
	}

	_, err := io.WriteString(w, sb.String())
	return err
}

func (r *markdownRenderer) RenderComplianceReport(w io.Writer, report *ComplianceReport) error {
	var sb strings.Builder

	fmt.Fprintf(&sb, "# Compliance Report: %s\n\n", report.FrameworkTitle)
	fmt.Fprintf(&sb, "**Date:** %s\n", report.GeneratedAt.Format(time.RFC3339))
	if report.Stack != "" {
		fmt.Fprintf(&sb, "**Stack:** %s\n", report.Stack)
	}
	fmt.Fprintf(&sb, "**Framework:** %s\n\n", report.FrameworkTitle)
	fmt.Fprintf(&sb, "## Score: %d/%d Controls Passing (%.0f%%)\n\n",
		report.PassingControls, report.TotalControls, report.ScorePercent)

	if len(report.FailingDetails) > 0 {
		sb.WriteString("### Failing Controls\n\n")
		sb.WriteString("| Control | Title | Severity | Component | Remediation |\n")
		sb.WriteString("|---------|-------|----------|-----------|-------------|\n")
		for _, ctrl := range report.FailingDetails {
			hasRemediation := "No"
			if ctrl.Remediation != nil {
				hasRemediation = "Yes"
			}
			fmt.Fprintf(&sb, "| %s | %s | %s | %s | %s |\n",
				ctrl.ControlID, ctrl.Title, ctrl.Severity, ctrl.Component, hasRemediation)
		}
		sb.WriteString("\n")
	}

	_, err := io.WriteString(w, sb.String())
	return err
}

// jsonRenderer renders reports as structured JSON.
type jsonRenderer struct{}

func (r *jsonRenderer) RenderSecurityReport(w io.Writer, report *Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

func (r *jsonRenderer) RenderComplianceReport(w io.Writer, report *ComplianceReport) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

// yamlRenderer renders reports as YAML.
type yamlRenderer struct{}

func (r *yamlRenderer) RenderSecurityReport(w io.Writer, report *Report) error {
	enc := yaml.NewEncoder(w)
	defer enc.Close()
	return enc.Encode(report)
}

func (r *yamlRenderer) RenderComplianceReport(w io.Writer, report *ComplianceReport) error {
	enc := yaml.NewEncoder(w)
	defer enc.Close()
	return enc.Encode(report)
}

// csvRenderer renders findings as flat CSV rows.
type csvRenderer struct{}

func (r *csvRenderer) RenderSecurityReport(w io.Writer, report *Report) error {
	cw := csv.NewWriter(w)
	defer cw.Flush()

	// Header row.
	if err := cw.Write([]string{
		"id", "title", "severity", "source", "resource_arn", "resource_type",
		"stack", "component", "mapped", "confidence", "remediation",
	}); err != nil {
		return err
	}

	for i := range report.Findings {
		f := &report.Findings[i]
		stack, component, mapped, confidence := "", "", "false", ""
		if f.Mapping != nil {
			stack = f.Mapping.Stack
			component = f.Mapping.Component
			if f.Mapping.Mapped {
				mapped = "true"
			}
			confidence = string(f.Mapping.Confidence)
		}
		remediation := ""
		if f.Remediation != nil {
			remediation = f.Remediation.Description
		}
		if err := cw.Write([]string{
			f.ID, f.Title, string(f.Severity), string(f.Source),
			f.ResourceARN, f.ResourceType,
			stack, component, mapped, confidence, remediation,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (r *csvRenderer) RenderComplianceReport(w io.Writer, report *ComplianceReport) error {
	cw := csv.NewWriter(w)
	defer cw.Flush()

	if err := cw.Write([]string{
		"control_id", "title", "severity", "component", "stack", "has_remediation",
	}); err != nil {
		return err
	}

	for _, ctrl := range report.FailingDetails {
		hasRemediation := "false"
		if ctrl.Remediation != nil {
			hasRemediation = "true"
		}
		if err := cw.Write([]string{
			ctrl.ControlID, ctrl.Title, string(ctrl.Severity),
			ctrl.Component, ctrl.Stack, hasRemediation,
		}); err != nil {
			return err
		}
	}
	return nil
}

// Helper functions.

func reportTarget(stack, component string) string {
	if stack != "" && component != "" {
		return fmt.Sprintf("%s / %s", stack, component)
	}
	if stack != "" {
		return stack
	}
	return "All Stacks"
}

func severityCountsString(counts map[Severity]int) string {
	var parts []string
	for _, sev := range []Severity{SeverityCritical, SeverityHigh, SeverityMedium, SeverityLow} {
		if c, ok := counts[sev]; ok && c > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", c, sev))
		}
	}
	return strings.Join(parts, ", ")
}

func filterBySeverity(findings []Finding, severity Severity) []Finding {
	var result []Finding
	for i := range findings {
		if findings[i].Severity == severity {
			result = append(result, findings[i])
		}
	}
	return result
}

func countMappedBySeverity(findings []Finding, severity Severity) (mapped, unmapped int) {
	for i := range findings {
		f := &findings[i]
		if f.Severity != severity {
			continue
		}
		if f.Mapping != nil && f.Mapping.Mapped {
			mapped++
		} else {
			unmapped++
		}
	}
	return mapped, unmapped
}

func renderFindingMarkdown(sb *strings.Builder, f *Finding, num int) {
	fmt.Fprintf(sb, "### %d. %s\n\n", num, f.Title)
	sb.WriteString("| Field | Value |\n")
	sb.WriteString("|-------|-------|\n")
	fmt.Fprintf(sb, "| **Severity** | %s |\n", f.Severity)
	fmt.Fprintf(sb, "| **Source** | %s", f.Source)
	if f.ComplianceStandard != "" {
		fmt.Fprintf(sb, " (%s)", f.ComplianceStandard)
	}
	sb.WriteString(" |\n")
	fmt.Fprintf(sb, "| **Resource** | `%s` |\n", f.ResourceARN)

	if f.Mapping != nil && f.Mapping.Mapped {
		fmt.Fprintf(sb, "| **Component** | %s |\n", f.Mapping.Component)
		fmt.Fprintf(sb, "| **Stack** | %s |\n", f.Mapping.Stack)
		if f.Mapping.ComponentPath != "" {
			fmt.Fprintf(sb, "| **Path** | `%s` |\n", f.Mapping.ComponentPath)
		}
		fmt.Fprintf(sb, "| **Confidence** | %s |\n", f.Mapping.Confidence)
	} else {
		sb.WriteString("| **Component** | *unmapped* |\n")
	}
	sb.WriteString("\n")

	if f.Description != "" {
		fmt.Fprintf(sb, "#### Finding Details\n\n%s\n\n", f.Description)
	}

	if f.Remediation != nil {
		sb.WriteString("#### Remediation\n\n")
		if f.Remediation.RootCause != "" {
			fmt.Fprintf(sb, "**Root Cause:** %s\n\n", f.Remediation.RootCause)
		}
		fmt.Fprintf(sb, "%s\n\n", f.Remediation.Description)
		if f.Remediation.DeployCommand != "" {
			fmt.Fprintf(sb, "**Deploy:** `%s`\n\n", f.Remediation.DeployCommand)
		}
	}

	sb.WriteString("---\n\n")
}
