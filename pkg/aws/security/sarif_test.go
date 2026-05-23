package security

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseOutputFormat_SARIF(t *testing.T) {
	cases := []struct {
		in   string
		want OutputFormat
	}{
		{"sarif", FormatSARIF},
		{"SARIF", FormatSARIF},
		{"Sarif", FormatSARIF},
	}
	for _, tc := range cases {
		got, err := ParseOutputFormat(tc.in)
		require.NoError(t, err, "ParseOutputFormat(%q)", tc.in)
		assert.Equal(t, tc.want, got)
	}
}

func TestSeverityToLevel(t *testing.T) {
	cases := []struct {
		sev  Severity
		want string
	}{
		{SeverityCritical, "error"},
		{SeverityHigh, "error"},
		{SeverityMedium, "warning"},
		{SeverityLow, "note"},
		{SeverityInformational, "note"},
		{Severity("UNKNOWN"), "none"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, severityToLevel(tc.sev), "severity %s", tc.sev)
	}
}

func TestBuildSARIFLog_StructureAndCounts(t *testing.T) {
	report := newTestSecurityReport()

	log := BuildSARIFLog(report)
	require.NotNil(t, log)
	assert.Equal(t, "2.1.0", log.Version)
	require.Len(t, log.Runs, 1)

	run := log.Runs[0]
	assert.Equal(t, "atmos", run.Tool.Driver.Name)
	assert.NotEmpty(t, run.Tool.Driver.Version)

	// Three findings in the fixture → three results.
	require.Len(t, run.Results, len(report.Findings))

	// All distinct titles in the fixture → distinct rules.
	titles := map[string]struct{}{}
	for i := range report.Findings {
		titles[report.Findings[i].Title] = struct{}{}
	}
	assert.Len(t, run.Tool.Driver.Rules, len(titles))
}

func TestBuildSARIFLog_OrderingStable(t *testing.T) {
	report := newTestSecurityReport()
	first := BuildSARIFLog(report)
	second := BuildSARIFLog(report)

	a, err := json.Marshal(first)
	require.NoError(t, err)
	b, err := json.Marshal(second)
	require.NoError(t, err)
	assert.JSONEq(t, string(a), string(b))
}

func TestBuildSARIFLog_MappedFindingProducesPhysicalLocation(t *testing.T) {
	report := newTestSecurityReport()
	log := BuildSARIFLog(report)
	results := log.Runs[0].Results

	var mapped *Result
	for i := range results {
		if results[i].RuleID == slugify("Critical S3 bucket public access") {
			mapped = &results[i]
			break
		}
	}
	require.NotNil(t, mapped, "expected the critical S3 finding result")
	require.Len(t, mapped.Locations, 1)
	require.NotNil(t, mapped.Locations[0].PhysicalLocation)
	require.NotNil(t, mapped.Locations[0].PhysicalLocation.ArtifactLocation)
	assert.Equal(t, "components/terraform/s3-bucket", mapped.Locations[0].PhysicalLocation.ArtifactLocation.URI)
	assert.Equal(t, "%SRCROOT%", mapped.Locations[0].PhysicalLocation.ArtifactLocation.URIBaseID)

	// Mapped finding still surfaces the ARN as a logical location alongside the file.
	require.Len(t, mapped.Locations[0].LogicalLocations, 1)
	assert.Equal(t, "arn:aws:s3:::my-bucket", mapped.Locations[0].LogicalLocations[0].Name)

	// Remediation metadata is preserved on properties.
	assert.Equal(t, "atmos terraform apply s3-bucket -s tenant1-ue1-prod", mapped.Properties["remediation_deploy_command"])
}

func TestBuildSARIFLog_UnmappedFindingHasLogicalLocationOnly(t *testing.T) {
	report := newTestSecurityReport()
	log := BuildSARIFLog(report)
	results := log.Runs[0].Results

	// finding-3 in the fixture has Mapping == nil — its result should carry no
	// physical location and emit the ARN as a logical location.
	unmappedRule := slugify("Low severity info leak")
	var unmapped *Result
	for i := range results {
		if results[i].RuleID == unmappedRule {
			unmapped = &results[i]
			break
		}
	}
	require.NotNil(t, unmapped, "expected an unmapped result")
	_, hasMapped := unmapped.Properties["mapped"]
	assert.False(t, hasMapped, "unmapped findings should not carry a `mapped` property")
	require.Len(t, unmapped.Locations, 1)
	assert.Nil(t, unmapped.Locations[0].PhysicalLocation)
	require.Len(t, unmapped.Locations[0].LogicalLocations, 1)
	assert.Equal(t, "arn:aws:lambda:us-east-1:123456789012:function:orphan", unmapped.Locations[0].LogicalLocations[0].Name)
}

func TestSARIFRenderer_OutputIsValidJSON(t *testing.T) {
	report := newTestSecurityReport()
	renderer := NewReportRenderer(FormatSARIF)
	require.NotNil(t, renderer)

	var buf bytes.Buffer
	require.NoError(t, renderer.RenderSecurityReport(&buf, report))

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &decoded), "SARIF output must be valid JSON")
	assert.Equal(t, "2.1.0", decoded["version"])
	runs, ok := decoded["runs"].([]any)
	require.True(t, ok, "runs must be a JSON array")
	require.Len(t, runs, 1)
}

func TestBuildSARIFLog_NilReport(t *testing.T) {
	log := BuildSARIFLog(nil)
	require.NotNil(t, log)
	require.Len(t, log.Runs, 1)
	assert.Empty(t, log.Runs[0].Results)
}

func TestBuildSARIFLog_EmptyFindings(t *testing.T) {
	report := &Report{GeneratedAt: fixedTime, Findings: []Finding{}}
	log := BuildSARIFLog(report)
	require.Len(t, log.Runs, 1)
	assert.Empty(t, log.Runs[0].Results)
	assert.Empty(t, log.Runs[0].Tool.Driver.Rules)
}

func TestSlugify(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Critical S3 bucket public access", "critical-s3-bucket-public-access"},
		{"  spaced   text  ", "spaced-text"},
		{"!!!", "finding"},
		{"already-slug", "already-slug"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, slugify(tc.in), "slugify(%q)", tc.in)
	}
}

func TestSeverityToSecuritySeverity(t *testing.T) {
	assert.Equal(t, "9.5", severityToSecuritySeverity(SeverityCritical))
	assert.Equal(t, "8.0", severityToSecuritySeverity(SeverityHigh))
	assert.Equal(t, "5.5", severityToSecuritySeverity(SeverityMedium))
	assert.Equal(t, "3.0", severityToSecuritySeverity(SeverityLow))
	assert.Equal(t, "1.0", severityToSecuritySeverity(SeverityInformational))
	assert.Equal(t, "0.0", severityToSecuritySeverity(Severity("UNKNOWN")))
}

func TestSeverityRank(t *testing.T) {
	cases := []struct {
		sev  Severity
		want int
	}{
		{SeverityCritical, rankCritical},
		{SeverityHigh, rankHigh},
		{SeverityMedium, rankMedium},
		{SeverityLow, rankLow},
		{SeverityInformational, rankInformational},
		{Severity("BOGUS"), rankUnknown},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, severityRank(tc.sev), "severity %s", tc.sev)
	}
}

func TestBuildComplianceSARIFLog_NilReport(t *testing.T) {
	log := buildComplianceSARIFLog(nil)
	require.NotNil(t, log)
	assert.Equal(t, "2.1.0", log.Version)
	require.Len(t, log.Runs, 1)
	assert.Empty(t, log.Runs[0].Results)
}

func TestBuildComplianceSARIFLog_EmptyDetails(t *testing.T) {
	report := &ComplianceReport{
		Framework: "cis-aws",
	}
	log := buildComplianceSARIFLog(report)
	require.NotNil(t, log)
	require.Len(t, log.Runs, 1)
	assert.Empty(t, log.Runs[0].Tool.Driver.Rules)
	assert.Empty(t, log.Runs[0].Results)
}

func TestBuildComplianceSARIFLog_DeduplicatesRules(t *testing.T) {
	report := &ComplianceReport{
		Framework: "cis-aws",
		FailingDetails: []ComplianceControl{
			{ControlID: "CIS.1.1", Title: "Root user MFA", Severity: SeverityHigh, Stack: "ue1", Component: "iam"},
			{ControlID: "CIS.1.1", Title: "Root user MFA", Severity: SeverityHigh, Stack: "ue2", Component: "iam"},
			{ControlID: "CIS.2.1", Title: "CloudTrail enabled", Severity: SeverityCritical, Stack: "ue1", Component: "cloudtrail"},
		},
	}
	log := buildComplianceSARIFLog(report)
	require.NotNil(t, log)
	rules := log.Runs[0].Tool.Driver.Rules
	results := log.Runs[0].Results
	assert.Len(t, rules, 2, "two unique ControlIDs should produce two rules")
	assert.Len(t, results, 3, "all three failing details should produce results")
}

func TestBuildComplianceSARIFLog_SlugFallbackForMissingControlID(t *testing.T) {
	report := &ComplianceReport{
		Framework: "custom",
		FailingDetails: []ComplianceControl{
			{Title: "Encryption at rest required!", Severity: SeverityHigh},
		},
	}
	log := buildComplianceSARIFLog(report)
	require.NotNil(t, log)
	rules := log.Runs[0].Tool.Driver.Rules
	require.Len(t, rules, 1)
	assert.Equal(t, "encryption-at-rest-required", rules[0].ID,
		"missing ControlID must fall back to slugified Title")
}

func TestBuildComplianceSARIFLog_OrderingIsDeterministic(t *testing.T) {
	// Inputs in a random order; the sorted output must be (severity desc, then
	// ControlID asc, then Title, Stack, Component).
	report := &ComplianceReport{
		Framework: "cis-aws",
		FailingDetails: []ComplianceControl{
			{ControlID: "CIS.3.1", Title: "Low risk", Severity: SeverityLow},
			{ControlID: "CIS.2.1", Title: "High risk", Severity: SeverityHigh, Stack: "ue2"},
			{ControlID: "CIS.1.1", Title: "Critical risk", Severity: SeverityCritical},
			{ControlID: "CIS.2.1", Title: "High risk", Severity: SeverityHigh, Stack: "ue1"},
		},
	}
	log := buildComplianceSARIFLog(report)
	results := log.Runs[0].Results
	require.Len(t, results, 4)
	// Critical first.
	assert.Equal(t, "CIS.1.1", results[0].RuleID)
	// Then the two highs in stack asc order (ue1 before ue2).
	assert.Equal(t, "CIS.2.1", results[1].RuleID)
	assert.Equal(t, "ue1", results[1].Properties["stack"])
	assert.Equal(t, "CIS.2.1", results[2].RuleID)
	assert.Equal(t, "ue2", results[2].Properties["stack"])
	// Low last.
	assert.Equal(t, "CIS.3.1", results[3].RuleID)

	// Run again — must be byte-identical.
	log2 := buildComplianceSARIFLog(report)
	a, err := json.Marshal(log)
	require.NoError(t, err)
	b, err := json.Marshal(log2)
	require.NoError(t, err)
	assert.True(t, bytes.Equal(a, b), "SARIF output must be deterministic across runs")
}

func TestSARIFRenderer_RenderComplianceReport(t *testing.T) {
	r := &sarifRenderer{}
	report := &ComplianceReport{
		Framework: "cis-aws",
		FailingDetails: []ComplianceControl{
			{ControlID: "CIS.1.1", Title: "Root user MFA", Severity: SeverityHigh, Stack: "ue1", Component: "iam"},
		},
	}
	var buf bytes.Buffer
	require.NoError(t, r.RenderComplianceReport(&buf, report))
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &decoded))
	assert.Equal(t, "2.1.0", decoded["version"])
}
