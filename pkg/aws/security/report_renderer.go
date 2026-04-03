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
		tagHint := "the configured resource tags"
		if report.TagMapping != nil {
			tagHint = fmt.Sprintf("`%s` and `%s` tags", report.TagMapping.StackTag, report.TagMapping.ComponentTag)
		}
		fmt.Fprintf(&sb, "> %d findings could not be mapped to Atmos components. "+
			"These resources may be managed outside of Atmos or may be missing %s.\n\n",
			report.UnmappedCount, tagHint)
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
		"stack", "component", "mapped", "confidence",
		"root_cause", "deploy_command", "risk_level",
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
		rootCause, deployCmd, riskLevel := "", "", ""
		if f.Remediation != nil {
			rootCause = f.Remediation.RootCause
			deployCmd = f.Remediation.DeployCommand
			riskLevel = f.Remediation.RiskLevel
		}
		if err := cw.Write([]string{
			f.ID, f.Title, string(f.Severity), string(f.Source),
			f.ResourceARN, f.ResourceType,
			stack, component, mapped, confidence,
			rootCause, deployCmd, riskLevel,
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
	if f.AccountID != "" {
		fmt.Fprintf(sb, "| **Account** | %s |\n", f.AccountID)
	}

	if f.Mapping != nil && f.Mapping.Mapped {
		fmt.Fprintf(sb, "| **Component** | %s |\n", f.Mapping.Component)
		fmt.Fprintf(sb, "| **Stack** | %s |\n", f.Mapping.Stack)
		if f.Mapping.ComponentPath != "" {
			fmt.Fprintf(sb, "| **Path** | `%s` |\n", f.Mapping.ComponentPath)
		}
		fmt.Fprintf(sb, "| **Confidence** | %s |\n", f.Mapping.Confidence)
		if f.Mapping.Method != "" {
			fmt.Fprintf(sb, "| **Mapped By** | %s |\n", f.Mapping.Method)
		}
	} else {
		sb.WriteString("| **Component** | *unmapped* |\n")
	}

	// Show resource tags if available (helps users identify the resource).
	if len(f.ResourceTags) > 0 {
		renderResourceTags(sb, f.ResourceTags)
	}

	sb.WriteString("\n")

	if f.Description != "" {
		fmt.Fprintf(sb, "#### Finding Details\n\n%s\n\n", f.Description)
	}

	if f.Remediation != nil {
		renderRemediationMarkdown(sb, f.Remediation)
	}

	sb.WriteString("---\n\n")
}

// mdNewline is the newline string used in Markdown rendering.
const mdNewline = "\n"

// renderResourceTags renders resource tags as a compact key=value list.
func renderResourceTags(sb *strings.Builder, tags map[string]string) {
	if len(tags) == 0 {
		return
	}
	sb.WriteString("\n**Resource Tags:**\n\n")
	for k, v := range tags {
		fmt.Fprintf(sb, "- `%s` = `%s`\n", k, v)
	}
}

// renderRemediationMarkdown renders the full Remediation struct as Markdown subsections.
func renderRemediationMarkdown(sb *strings.Builder, r *Remediation) {
	sb.WriteString("#### Remediation\n\n")

	if r.RootCause != "" {
		fmt.Fprintf(sb, "**Root Cause:** %s\n\n", r.RootCause)
	}

	renderSteps(sb, r.Steps)
	renderCodeChanges(sb, r.CodeChanges)

	if r.StackChanges != "" {
		fmt.Fprintf(sb, "**Stack Changes:**\n\n%s\n\n", r.StackChanges)
	}
	if r.DeployCommand != "" {
		fmt.Fprintf(sb, "**Deploy:** `%s`\n\n", r.DeployCommand)
	}
	if r.RiskLevel != "" {
		fmt.Fprintf(sb, "**Risk:** %s\n\n", r.RiskLevel)
	}

	renderReferences(sb, r.References)

	// Fall back to description if no structured fields are populated.
	if r.RootCause == "" && len(r.Steps) == 0 && r.DeployCommand == "" {
		fmt.Fprintf(sb, "%s\n\n", r.Description)
	}
}

// renderSteps renders an ordered list of remediation steps.
func renderSteps(sb *strings.Builder, steps []string) {
	if len(steps) == 0 {
		return
	}
	sb.WriteString("**Steps:**\n\n")
	for i, step := range steps {
		fmt.Fprintf(sb, "%d. %s%s", i+1, step, mdNewline)
	}
	sb.WriteString(mdNewline)
}

// renderCodeChanges renders code change diffs.
func renderCodeChanges(sb *strings.Builder, changes []CodeChange) {
	if len(changes) == 0 {
		return
	}
	sb.WriteString("**Code Changes:**\n\n")
	for _, change := range changes {
		fmt.Fprintf(sb, "File: `%s`", change.FilePath)
		if change.Line > 0 {
			fmt.Fprintf(sb, " (line %d)", change.Line)
		}
		sb.WriteString(mdNewline)
		if change.Before != "" {
			fmt.Fprintf(sb, "```diff\n- %s\n+ %s\n```\n\n", change.Before, change.After)
		}
	}
}

// renderReferences renders a bulleted list of references.
func renderReferences(sb *strings.Builder, refs []string) {
	if len(refs) == 0 {
		return
	}
	sb.WriteString("**References:**\n\n")
	for _, ref := range refs {
		fmt.Fprintf(sb, "- %s%s", ref, mdNewline)
	}
	sb.WriteString(mdNewline)
}
