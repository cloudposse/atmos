package security

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// fixedTime is a stable timestamp used across all test reports.
var fixedTime = time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC)

// newTestSecurityReport builds a Report with representative findings for tests.
func newTestSecurityReport() *Report {
	return &Report{
		GeneratedAt:   fixedTime,
		Stack:         "tenant1-ue1-prod",
		Component:     "vpc",
		TotalFindings: 3,
		SeverityCounts: map[Severity]int{
			SeverityCritical: 1,
			SeverityHigh:     1,
			SeverityLow:      1,
		},
		MappedCount:   2,
		UnmappedCount: 1,
		Findings: []Finding{
			{
				ID:                 "finding-1",
				Title:              "Critical S3 bucket public access",
				Description:        "S3 bucket allows public read access.",
				Severity:           SeverityCritical,
				Source:             SourceSecurityHub,
				ComplianceStandard: "CIS-1.4",
				ResourceARN:        "arn:aws:s3:::my-bucket",
				ResourceType:       "AwsS3Bucket",
				Mapping: &ComponentMapping{
					Stack:         "tenant1-ue1-prod",
					Component:     "s3-bucket",
					ComponentPath: "components/terraform/s3-bucket",
					Mapped:        true,
					Confidence:    ConfidenceExact,
					Method:        "tag",
				},
				Remediation: &Remediation{
					Description:   "Enable block public access on the S3 bucket.",
					RootCause:     "Public access block not configured.",
					DeployCommand: "atmos terraform apply s3-bucket -s tenant1-ue1-prod",
				},
			},
			{
				ID:           "finding-2",
				Title:        "Security group allows ingress from 0.0.0.0/0",
				Description:  "Unrestricted ingress detected.",
				Severity:     SeverityHigh,
				Source:       SourceConfig,
				ResourceARN:  "arn:aws:ec2:us-east-1:123456789012:security-group/sg-123",
				ResourceType: "AwsEc2SecurityGroup",
				Mapping: &ComponentMapping{
					Stack:      "tenant1-ue1-prod",
					Component:  "vpc",
					Mapped:     true,
					Confidence: ConfidenceHigh,
					Method:     "state",
				},
			},
			{
				ID:           "finding-3",
				Title:        "Low severity info leak",
				Severity:     SeverityLow,
				Source:       SourceInspector,
				ResourceARN:  "arn:aws:lambda:us-east-1:123456789012:function:orphan",
				ResourceType: "AwsLambdaFunction",
				Mapping:      nil,
			},
		},
	}
}

// newTestComplianceReport builds a ComplianceReport for tests.
func newTestComplianceReport() *ComplianceReport {
	return &ComplianceReport{
		GeneratedAt:     fixedTime,
		Stack:           "tenant1-ue1-prod",
		Framework:       "cis-1.4",
		FrameworkTitle:  "CIS AWS Foundations Benchmark v1.4",
		TotalControls:   50,
		PassingControls: 45,
		FailingControls: 5,
		ScorePercent:    90.0,
		FailingDetails: []ComplianceControl{
			{
				ControlID: "CIS.1.14",
				Title:     "Ensure MFA is enabled for root",
				Severity:  SeverityCritical,
				Component: "account-settings",
				Stack:     "tenant1-ue1-prod",
				Remediation: &Remediation{
					Description: "Enable MFA on root account.",
				},
			},
			{
				ControlID:   "CIS.2.1",
				Title:       "Ensure CloudTrail is enabled",
				Severity:    SeverityHigh,
				Component:   "cloudtrail",
				Stack:       "tenant1-ue1-prod",
				Remediation: nil,
			},
		},
	}
}

func TestRenderSecurityReport_Markdown(t *testing.T) {
	report := newTestSecurityReport()
	renderer := NewReportRenderer(FormatMarkdown)
	var buf bytes.Buffer

	err := renderer.RenderSecurityReport(&buf, report)
	require.NoError(t, err)

	output := buf.String()

	// Verify header and metadata.
	assert.Contains(t, output, "# Security Report: tenant1-ue1-prod / vpc")
	assert.Contains(t, output, "**Generated:** 2026-03-09T12:00:00Z")
	assert.Contains(t, output, "**Stack:** tenant1-ue1-prod")
	assert.Contains(t, output, "**Findings:** 3")

	// Verify severity sections.
	assert.Contains(t, output, "## CRITICAL Findings (1)")
	assert.Contains(t, output, "## HIGH Findings (1)")
	assert.Contains(t, output, "## LOW Findings (1)")
	// Medium and informational should not appear.
	assert.NotContains(t, output, "## MEDIUM Findings")
	assert.NotContains(t, output, "## INFORMATIONAL Findings")

	// Verify finding details.
	assert.Contains(t, output, "### 1. Critical S3 bucket public access")
	assert.Contains(t, output, "| **Severity** | CRITICAL |")
	assert.Contains(t, output, "| **Source** | security-hub (CIS-1.4) |")
	assert.Contains(t, output, "| **Resource** | `arn:aws:s3:::my-bucket` |")
	assert.Contains(t, output, "| **Component** | s3-bucket |")
	assert.Contains(t, output, "| **Path** | `components/terraform/s3-bucket` |")
	assert.Contains(t, output, "| **Confidence** | exact |")

	// Verify remediation section with structured fields.
	assert.Contains(t, output, "#### Remediation")
	assert.Contains(t, output, "**Root Cause:** Public access block not configured.")
	assert.Contains(t, output, "**Deploy:** `atmos terraform apply s3-bucket -s tenant1-ue1-prod`")

	// Verify finding description section.
	assert.Contains(t, output, "#### Finding Details")
	assert.Contains(t, output, "S3 bucket allows public read access.")

	// Verify unmapped finding renders correctly.
	assert.Contains(t, output, "| **Component** | *unmapped* |")

	// Verify summary table.
	assert.Contains(t, output, "## Summary")
	assert.Contains(t, output, "| Severity | Count | Mapped | Unmapped |")
	assert.Contains(t, output, "| CRITICAL | 1 | 1 | 0 |")
	assert.Contains(t, output, "| HIGH | 1 | 1 | 0 |")
	assert.Contains(t, output, "| LOW | 1 | 0 | 1 |")
	assert.Contains(t, output, "| **Total** | **3** | **2** | **1** |")

	// Verify unmapped note.
	assert.Contains(t, output, "1 findings could not be mapped to Atmos components")
}

func TestRenderSecurityReport_JSON(t *testing.T) {
	report := newTestSecurityReport()
	renderer := NewReportRenderer(FormatJSON)
	var buf bytes.Buffer

	err := renderer.RenderSecurityReport(&buf, report)
	require.NoError(t, err)

	// Verify it produces valid JSON.
	var decoded Report
	err = json.Unmarshal(buf.Bytes(), &decoded)
	require.NoError(t, err)

	// Spot-check fields round-trip correctly.
	assert.Equal(t, report.Stack, decoded.Stack)
	assert.Equal(t, report.Component, decoded.Component)
	assert.Equal(t, report.TotalFindings, decoded.TotalFindings)
	assert.Equal(t, report.MappedCount, decoded.MappedCount)
	assert.Equal(t, report.UnmappedCount, decoded.UnmappedCount)
	assert.Len(t, decoded.Findings, 3)
	assert.Equal(t, SeverityCritical, decoded.Findings[0].Severity)
	assert.Equal(t, "s3-bucket", decoded.Findings[0].Mapping.Component)
	assert.Nil(t, decoded.Findings[2].Mapping)
}

func TestRenderSecurityReport_YAML(t *testing.T) {
	report := newTestSecurityReport()
	renderer := NewReportRenderer(FormatYAML)
	var buf bytes.Buffer

	err := renderer.RenderSecurityReport(&buf, report)
	require.NoError(t, err)

	// Verify it produces valid YAML.
	var decoded Report
	err = yaml.Unmarshal(buf.Bytes(), &decoded)
	require.NoError(t, err)

	assert.Equal(t, report.Stack, decoded.Stack)
	assert.Equal(t, report.TotalFindings, decoded.TotalFindings)
	assert.Len(t, decoded.Findings, 3)
	assert.Equal(t, ConfidenceExact, decoded.Findings[0].Mapping.Confidence)
}

func TestRenderSecurityReport_CSV(t *testing.T) {
	report := newTestSecurityReport()
	renderer := NewReportRenderer(FormatCSV)
	var buf bytes.Buffer

	err := renderer.RenderSecurityReport(&buf, report)
	require.NoError(t, err)

	// Parse the CSV output.
	reader := csv.NewReader(strings.NewReader(buf.String()))
	records, err := reader.ReadAll()
	require.NoError(t, err)

	// Header + 3 data rows.
	require.Len(t, records, 4)

	// Verify header row.
	expectedHeaders := []string{
		"id", "title", "severity", "source", "resource_arn", "resource_type",
		"stack", "component", "mapped", "confidence",
		"root_cause", "deploy_command", "risk_level",
	}
	assert.Equal(t, expectedHeaders, records[0])

	// Verify mapped finding row.
	assert.Equal(t, "finding-1", records[1][0])
	assert.Equal(t, "CRITICAL", records[1][2])
	assert.Equal(t, "tenant1-ue1-prod", records[1][6])
	assert.Equal(t, "s3-bucket", records[1][7])
	assert.Equal(t, "true", records[1][8])
	assert.Equal(t, "exact", records[1][9])

	// Verify unmapped finding row (nil Mapping).
	assert.Equal(t, "finding-3", records[3][0])
	assert.Equal(t, "", records[3][6]) // stack empty.
	assert.Equal(t, "", records[3][7]) // component empty.
	assert.Equal(t, "false", records[3][8])
	assert.Equal(t, "", records[3][9])  // confidence empty.
	assert.Equal(t, "", records[3][10]) // root_cause empty.
	assert.Equal(t, "", records[3][11]) // deploy_command empty.
	assert.Equal(t, "", records[3][12]) // risk_level empty.
}

func TestRenderComplianceReport_Markdown(t *testing.T) {
	report := newTestComplianceReport()
	renderer := NewReportRenderer(FormatMarkdown)
	var buf bytes.Buffer

	err := renderer.RenderComplianceReport(&buf, report)
	require.NoError(t, err)

	output := buf.String()

	// Verify header and metadata.
	assert.Contains(t, output, "# Compliance Report: CIS AWS Foundations Benchmark v1.4")
	assert.Contains(t, output, "**Date:** 2026-03-09T12:00:00Z")
	assert.Contains(t, output, "**Stack:** tenant1-ue1-prod")
	assert.Contains(t, output, "**Framework:** CIS AWS Foundations Benchmark v1.4")

	// Verify score line.
	assert.Contains(t, output, "## Score: 45/50 Controls Passing (90%)")

	// Verify failing controls table.
	assert.Contains(t, output, "### Failing Controls")
	assert.Contains(t, output, "| Control | Title | Severity |")
	assert.Contains(t, output, "| CIS.1.14 | Ensure MFA is enabled for root | CRITICAL |")
	assert.Contains(t, output, "| CIS.2.1 | Ensure CloudTrail is enabled | HIGH |")
}

func TestRenderComplianceReport_JSON(t *testing.T) {
	report := newTestComplianceReport()
	renderer := NewReportRenderer(FormatJSON)
	var buf bytes.Buffer

	err := renderer.RenderComplianceReport(&buf, report)
	require.NoError(t, err)

	var decoded ComplianceReport
	err = json.Unmarshal(buf.Bytes(), &decoded)
	require.NoError(t, err)

	assert.Equal(t, report.Framework, decoded.Framework)
	assert.Equal(t, report.FrameworkTitle, decoded.FrameworkTitle)
	assert.Equal(t, report.TotalControls, decoded.TotalControls)
	assert.Equal(t, report.PassingControls, decoded.PassingControls)
	assert.Equal(t, report.FailingControls, decoded.FailingControls)
	assert.InDelta(t, report.ScorePercent, decoded.ScorePercent, 0.01)
	assert.Len(t, decoded.FailingDetails, 2)
	assert.NotNil(t, decoded.FailingDetails[0].Remediation)
	assert.Nil(t, decoded.FailingDetails[1].Remediation)
}

func TestRenderComplianceReport_YAML(t *testing.T) {
	report := newTestComplianceReport()
	renderer := NewReportRenderer(FormatYAML)
	var buf bytes.Buffer

	err := renderer.RenderComplianceReport(&buf, report)
	require.NoError(t, err)

	var decoded ComplianceReport
	err = yaml.Unmarshal(buf.Bytes(), &decoded)
	require.NoError(t, err)

	assert.Equal(t, report.Framework, decoded.Framework)
	assert.Equal(t, report.TotalControls, decoded.TotalControls)
	assert.Len(t, decoded.FailingDetails, 2)
}

func TestRenderComplianceReport_CSV(t *testing.T) {
	report := newTestComplianceReport()
	renderer := NewReportRenderer(FormatCSV)
	var buf bytes.Buffer

	err := renderer.RenderComplianceReport(&buf, report)
	require.NoError(t, err)

	reader := csv.NewReader(strings.NewReader(buf.String()))
	records, err := reader.ReadAll()
	require.NoError(t, err)

	// Header + 2 failing controls.
	require.Len(t, records, 3)

	expectedHeaders := []string{
		"control_id", "title", "severity", "component", "stack", "has_remediation",
	}
	assert.Equal(t, expectedHeaders, records[0])

	// Control with remediation.
	assert.Equal(t, "CIS.1.14", records[1][0])
	assert.Equal(t, "true", records[1][5])

	// Control without remediation.
	assert.Equal(t, "CIS.2.1", records[2][0])
	assert.Equal(t, "false", records[2][5])
}

func TestRenderSecurityReport_EmptyFindings(t *testing.T) {
	report := &Report{
		GeneratedAt:    fixedTime,
		Stack:          "tenant1-ue1-dev",
		TotalFindings:  0,
		SeverityCounts: map[Severity]int{},
		Findings:       []Finding{},
		MappedCount:    0,
		UnmappedCount:  0,
	}

	formats := []struct {
		name   string
		format OutputFormat
	}{
		{name: "markdown", format: FormatMarkdown},
		{name: "json", format: FormatJSON},
		{name: "yaml", format: FormatYAML},
		{name: "csv", format: FormatCSV},
	}

	for _, tc := range formats {
		t.Run(tc.name, func(t *testing.T) {
			renderer := NewReportRenderer(tc.format)
			var buf bytes.Buffer
			err := renderer.RenderSecurityReport(&buf, report)
			require.NoError(t, err)
			assert.NotEmpty(t, buf.String())
		})
	}

	// Verify markdown specifics for empty report.
	t.Run("markdown_no_severity_sections", func(t *testing.T) {
		renderer := NewReportRenderer(FormatMarkdown)
		var buf bytes.Buffer
		err := renderer.RenderSecurityReport(&buf, report)
		require.NoError(t, err)

		output := buf.String()
		assert.Contains(t, output, "**Findings:** 0")
		assert.NotContains(t, output, "## CRITICAL Findings")
		assert.NotContains(t, output, "## HIGH Findings")
		// Summary total row should show zeros.
		assert.Contains(t, output, "| **Total** | **0** | **0** | **0** |")
		// No unmapped note when unmapped count is zero.
		assert.NotContains(t, output, "findings could not be mapped")
	})
}

func TestRenderSecurityReport_FindingsWithNilMapping(t *testing.T) {
	report := &Report{
		GeneratedAt:   fixedTime,
		Stack:         "tenant1-ue1-staging",
		TotalFindings: 2,
		SeverityCounts: map[Severity]int{
			SeverityMedium: 2,
		},
		MappedCount:   0,
		UnmappedCount: 2,
		Findings: []Finding{
			{
				ID:           "unmapped-1",
				Title:        "Unmapped finding one",
				Severity:     SeverityMedium,
				Source:       SourceGuardDuty,
				ResourceARN:  "arn:aws:ec2:us-west-2:111111111111:instance/i-abc",
				ResourceType: "AwsEc2Instance",
				Mapping:      nil,
			},
			{
				ID:           "unmapped-2",
				Title:        "Unmapped finding two",
				Severity:     SeverityMedium,
				Source:       SourceMacie,
				ResourceARN:  "arn:aws:s3:::another-bucket",
				ResourceType: "AwsS3Bucket",
				Mapping:      nil,
				Remediation:  nil,
			},
		},
	}

	t.Run("markdown_shows_unmapped", func(t *testing.T) {
		renderer := NewReportRenderer(FormatMarkdown)
		var buf bytes.Buffer
		err := renderer.RenderSecurityReport(&buf, report)
		require.NoError(t, err)

		output := buf.String()
		// Both findings should show unmapped.
		assert.Equal(t, 2, strings.Count(output, "| **Component** | *unmapped* |"))
		// Unmapped note should appear.
		assert.Contains(t, output, "2 findings could not be mapped to Atmos components")
	})

	t.Run("csv_shows_unmapped_fields", func(t *testing.T) {
		renderer := NewReportRenderer(FormatCSV)
		var buf bytes.Buffer
		err := renderer.RenderSecurityReport(&buf, report)
		require.NoError(t, err)

		reader := csv.NewReader(strings.NewReader(buf.String()))
		records, err := reader.ReadAll()
		require.NoError(t, err)
		require.Len(t, records, 3) // header + 2 rows.

		// Both rows should have empty stack/component and mapped=false.
		for _, row := range records[1:] {
			assert.Equal(t, "", row[6])      // stack.
			assert.Equal(t, "", row[7])      // component.
			assert.Equal(t, "false", row[8]) // mapped.
			assert.Equal(t, "", row[9])      // confidence.
		}
	})

	t.Run("json_nil_mapping_omitted", func(t *testing.T) {
		renderer := NewReportRenderer(FormatJSON)
		var buf bytes.Buffer
		err := renderer.RenderSecurityReport(&buf, report)
		require.NoError(t, err)

		var decoded Report
		err = json.Unmarshal(buf.Bytes(), &decoded)
		require.NoError(t, err)
		for _, f := range decoded.Findings {
			assert.Nil(t, f.Mapping)
		}
	})
}

func TestRenderComplianceReport_EmptyFailingDetails(t *testing.T) {
	report := &ComplianceReport{
		GeneratedAt:     fixedTime,
		Stack:           "tenant1-ue1-prod",
		Framework:       "cis-1.4",
		FrameworkTitle:  "CIS AWS Foundations Benchmark v1.4",
		TotalControls:   50,
		PassingControls: 50,
		FailingControls: 0,
		ScorePercent:    100.0,
		FailingDetails:  []ComplianceControl{},
	}

	t.Run("markdown_no_failing_table", func(t *testing.T) {
		renderer := NewReportRenderer(FormatMarkdown)
		var buf bytes.Buffer
		err := renderer.RenderComplianceReport(&buf, report)
		require.NoError(t, err)

		output := buf.String()
		assert.Contains(t, output, "## Score: 50/50 Controls Passing (100%)")
		assert.NotContains(t, output, "### Failing Controls")
	})

	t.Run("json_round_trip", func(t *testing.T) {
		renderer := NewReportRenderer(FormatJSON)
		var buf bytes.Buffer
		err := renderer.RenderComplianceReport(&buf, report)
		require.NoError(t, err)

		var decoded ComplianceReport
		err = json.Unmarshal(buf.Bytes(), &decoded)
		require.NoError(t, err)
		assert.Equal(t, 0, decoded.FailingControls)
		assert.Empty(t, decoded.FailingDetails)
	})
}

func TestNewReportRenderer_DefaultsToMarkdown(t *testing.T) {
	// An unknown format should fall back to markdown.
	renderer := NewReportRenderer(OutputFormat("unknown"))
	var buf bytes.Buffer

	report := &Report{
		GeneratedAt:    fixedTime,
		TotalFindings:  0,
		SeverityCounts: map[Severity]int{},
		Findings:       []Finding{},
	}
	err := renderer.RenderSecurityReport(&buf, report)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "# Security Report:")
}

func TestReportTarget(t *testing.T) {
	tests := []struct {
		name      string
		stack     string
		component string
		expected  string
	}{
		{name: "both_set", stack: "prod", component: "vpc", expected: "prod / vpc"},
		{name: "stack_only", stack: "prod", component: "", expected: "prod"},
		{name: "neither_set", stack: "", component: "", expected: "All Stacks"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, reportTarget(tc.stack, tc.component))
		})
	}
}

func TestSeverityCountsString(t *testing.T) {
	counts := map[Severity]int{
		SeverityCritical: 2,
		SeverityHigh:     3,
		SeverityLow:      1,
	}
	result := severityCountsString(counts)
	assert.Contains(t, result, "2 CRITICAL")
	assert.Contains(t, result, "3 HIGH")
	assert.Contains(t, result, "1 LOW")
	// Medium not in the map, should not appear.
	assert.NotContains(t, result, "MEDIUM")
}

func TestFilterBySeverity(t *testing.T) {
	findings := []Finding{
		{ID: "1", Severity: SeverityCritical},
		{ID: "2", Severity: SeverityHigh},
		{ID: "3", Severity: SeverityCritical},
		{ID: "4", Severity: SeverityLow},
	}

	critical := filterBySeverity(findings, SeverityCritical)
	assert.Len(t, critical, 2)
	assert.Equal(t, "1", critical[0].ID)
	assert.Equal(t, "3", critical[1].ID)

	medium := filterBySeverity(findings, SeverityMedium)
	assert.Empty(t, medium)
}

func TestCountMappedBySeverity(t *testing.T) {
	findings := []Finding{
		{ID: "1", Severity: SeverityHigh, Mapping: &ComponentMapping{Mapped: true}},
		{ID: "2", Severity: SeverityHigh, Mapping: nil},
		{ID: "3", Severity: SeverityHigh, Mapping: &ComponentMapping{Mapped: false}},
		{ID: "4", Severity: SeverityLow, Mapping: &ComponentMapping{Mapped: true}},
	}

	mapped, unmapped := countMappedBySeverity(findings, SeverityHigh)
	assert.Equal(t, 1, mapped)
	assert.Equal(t, 2, unmapped)

	mapped, unmapped = countMappedBySeverity(findings, SeverityLow)
	assert.Equal(t, 1, mapped)
	assert.Equal(t, 0, unmapped)

	mapped, unmapped = countMappedBySeverity(findings, SeverityCritical)
	assert.Equal(t, 0, mapped)
	assert.Equal(t, 0, unmapped)
}

func TestRenderRemediationMarkdown_AllFields(t *testing.T) {
	report := &Report{
		GeneratedAt:    time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC),
		Stack:          "prod-us-east-1",
		TotalFindings:  1,
		SeverityCounts: map[Severity]int{SeverityHigh: 1},
		MappedCount:    1,
		Findings: []Finding{
			{
				ID:          "full-001",
				Title:       "EBS volume not encrypted",
				Severity:    SeverityHigh,
				Source:      SourceSecurityHub,
				ResourceARN: "arn:aws:ec2:us-east-1:123:volume/vol-abc",
				Mapping: &ComponentMapping{
					Component: "ebs",
					Stack:     "prod-us-east-1",
					Mapped:    true,
				},
				Remediation: &Remediation{
					RootCause:     "Encryption not enabled on the EBS volume.",
					Steps:         []string{"Add encryption variable to stack config", "Apply the change"},
					CodeChanges:   []CodeChange{{FilePath: "main.tf", Before: "encrypted = false", After: "encrypted = true"}},
					StackChanges:  "vars:\n  encryption_enabled: true",
					DeployCommand: "atmos terraform apply ebs -s prod-us-east-1",
					RiskLevel:     "low",
					References:    []string{"https://docs.aws.amazon.com/ebs", "CIS 2.2.1"},
				},
			},
		},
	}

	var buf strings.Builder
	renderer := NewReportRenderer(FormatMarkdown)
	require.NoError(t, renderer.RenderSecurityReport(&buf, report))
	output := buf.String()

	// Root cause.
	assert.Contains(t, output, "**Root Cause:** Encryption not enabled")

	// Steps.
	assert.Contains(t, output, "**Steps:**")
	assert.Contains(t, output, "1. Add encryption variable to stack config")
	assert.Contains(t, output, "2. Apply the change")

	// Code changes.
	assert.Contains(t, output, "**Code Changes:**")
	assert.Contains(t, output, "File: `main.tf`")
	assert.Contains(t, output, "- encrypted = false")
	assert.Contains(t, output, "+ encrypted = true")

	// Stack changes.
	assert.Contains(t, output, "**Stack Changes:**")
	assert.Contains(t, output, "encryption_enabled: true")

	// Deploy.
	assert.Contains(t, output, "**Deploy:** `atmos terraform apply ebs -s prod-us-east-1`")

	// Risk.
	assert.Contains(t, output, "**Risk:** low")

	// References.
	assert.Contains(t, output, "**References:**")
	assert.Contains(t, output, "- https://docs.aws.amazon.com/ebs")
	assert.Contains(t, output, "- CIS 2.2.1")
}

func TestRenderRemediationMarkdown_DescriptionFallback(t *testing.T) {
	// When no structured fields are populated, falls back to Description.
	report := &Report{
		GeneratedAt:    time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC),
		TotalFindings:  1,
		SeverityCounts: map[Severity]int{SeverityLow: 1},
		Findings: []Finding{
			{
				ID:       "fallback-001",
				Title:    "Minor issue",
				Severity: SeverityLow,
				Source:   SourceConfig,
				Remediation: &Remediation{
					Description: "This is a plain text description from the AI.",
				},
			},
		},
	}

	var buf strings.Builder
	renderer := NewReportRenderer(FormatMarkdown)
	require.NoError(t, renderer.RenderSecurityReport(&buf, report))
	output := buf.String()

	assert.Contains(t, output, "This is a plain text description from the AI.")
}

func TestRenderCSV_WithRemediation(t *testing.T) {
	report := &Report{
		TotalFindings: 1,
		Findings: []Finding{
			{
				ID:       "csv-001",
				Title:    "Test",
				Severity: SeverityHigh,
				Source:   SourceSecurityHub,
				Mapping: &ComponentMapping{
					Stack:     "prod",
					Component: "vpc",
					Mapped:    true,
				},
				Remediation: &Remediation{
					RootCause:     "Missing encryption",
					DeployCommand: "atmos terraform apply vpc -s prod",
					RiskLevel:     "medium",
				},
			},
		},
	}

	var buf strings.Builder
	renderer := NewReportRenderer(FormatCSV)
	require.NoError(t, renderer.RenderSecurityReport(&buf, report))

	r := csv.NewReader(strings.NewReader(buf.String()))
	records, err := r.ReadAll()
	require.NoError(t, err)
	require.Len(t, records, 2) // header + 1 row.

	// Verify new CSV columns.
	assert.Equal(t, "root_cause", records[0][10])
	assert.Equal(t, "deploy_command", records[0][11])
	assert.Equal(t, "risk_level", records[0][12])

	// Verify data.
	assert.Equal(t, "Missing encryption", records[1][10])
	assert.Equal(t, "atmos terraform apply vpc -s prod", records[1][11])
	assert.Equal(t, "medium", records[1][12])
}
