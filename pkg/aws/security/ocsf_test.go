package security

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildOCSFEvents_BasicFields(t *testing.T) {
	events := BuildOCSFEvents(newTestSecurityReport())
	require.Len(t, events, 3, "one event per finding")

	// Findings are sorted by severity desc, then ID asc, so finding-1 (critical) comes first.
	e := events[0]
	assert.Equal(t, ocsfClassUID, e.ClassUID)
	assert.Equal(t, "Detection Finding", e.ClassName)
	assert.Equal(t, ocsfCategoryUID, e.CategoryUID)
	assert.Equal(t, ocsfSeverityCritical, e.SeverityID)
	assert.Equal(t, "CRITICAL", e.Severity)
	assert.Equal(t, "finding-1", e.FindingInfo.UID)
	assert.Equal(t, "Critical S3 bucket public access", e.FindingInfo.Title)
	assert.Equal(t, ocsfCloudProvider, e.Cloud.Provider)
	require.Len(t, e.Resources, 1)
	assert.Equal(t, "arn:aws:s3:::my-bucket", e.Resources[0].UID)
	assert.Equal(t, "AwsS3Bucket", e.Resources[0].Type)
	assert.Equal(t, ocsfVersion, e.Metadata.Version)
	assert.Equal(t, ocsfProductName, e.Metadata.Product.Name)
	assert.NotEmpty(t, e.Metadata.CorrelationUID, "every event carries a correlation UID")
}

func TestBuildOCSFEvents_SourceSurfacedAsProductUID(t *testing.T) {
	events := BuildOCSFEvents(newTestSecurityReport())
	require.Len(t, events, 3)

	bySource := map[string]string{}
	for _, e := range events {
		bySource[e.FindingInfo.UID] = e.FindingInfo.ProductUID
	}
	assert.Equal(t, "security-hub", bySource["finding-1"],
		"Finding.Source flows into finding_info.product_uid so OCSF consumers can tell "+
			"which AWS service detected the finding (orthogonal to metadata.product=atmos)")
	assert.Equal(t, "config", bySource["finding-2"])
	assert.Equal(t, "inspector", bySource["finding-3"])
}

func TestBuildOCSFEvents_SharedCorrelationUID(t *testing.T) {
	events := BuildOCSFEvents(newTestSecurityReport())
	require.GreaterOrEqual(t, len(events), 2)
	for i := 1; i < len(events); i++ {
		assert.Equal(t, events[0].Metadata.CorrelationUID, events[i].Metadata.CorrelationUID,
			"all events in a batch share the correlation UID")
	}
}

func TestBuildOCSFEvents_MappingEnrichments(t *testing.T) {
	events := BuildOCSFEvents(newTestSecurityReport())
	require.NotEmpty(t, events)

	mappedEvent := events[0] // finding-1, mapped
	enrichmentByName := map[string]OCSFEnrichment{}
	for _, e := range mappedEvent.Enrichments {
		enrichmentByName[e.Name] = e
	}
	require.Contains(t, enrichmentByName, "atmos.stack")
	require.Contains(t, enrichmentByName, "atmos.component")
	require.Contains(t, enrichmentByName, "atmos.mapping.confidence")
	require.Contains(t, enrichmentByName, "atmos.mapping.mapped")
	assert.Equal(t, "tenant1-ue1-prod", enrichmentByName["atmos.stack"].Value)
	assert.Equal(t, "s3-bucket", enrichmentByName["atmos.component"].Value)
	assert.Equal(t, "exact", enrichmentByName["atmos.mapping.confidence"].Value)
	assert.Equal(t, "atmos", enrichmentByName["atmos.stack"].Provider)
	assert.Equal(t, "string", enrichmentByName["atmos.stack"].Type)
	assert.Equal(t, "tenant1-ue1-prod", enrichmentByName["atmos.stack"].Data, "data field is required by schema")
	assert.Equal(t, true, enrichmentByName["atmos.mapping.mapped"].Data, "boolean enrichments preserve typed data")
}

func TestBuildOCSFEvents_NoMapping_NoAtmosEnrichments(t *testing.T) {
	events := BuildOCSFEvents(newTestSecurityReport())
	require.NotEmpty(t, events)
	// finding-3 (low severity, no mapping) lands last after severity-desc sort.
	unmappedEvent := events[len(events)-1]
	assert.Empty(t, unmappedEvent.Enrichments,
		"findings without a Mapping must not emit atmos.* enrichments")
}

func TestBuildOCSFEvents_AWSAndAIRemediationSplit(t *testing.T) {
	report := newTestSecurityReport()
	awsRemediationURL := "https://docs.aws.amazon.com/securityhub/latest/userguide/securityhub-cis-controls.html"
	report.Findings[0].SourceRemediation = &SourceRemediation{
		Text: "Enable S3 Block Public Access.",
		URL:  awsRemediationURL,
	}

	events := BuildOCSFEvents(report)
	require.NotEmpty(t, events)
	e := events[0]

	require.NotNil(t, e.Remediation, "SourceRemediation populates native OCSF remediation")
	assert.Equal(t, "Enable S3 Block Public Access.", e.Remediation.Desc)
	assert.Equal(t, []string{awsRemediationURL}, e.Remediation.References)

	require.Contains(t, e.Unmapped, "atmos.remediation")
	aiRem, ok := e.Unmapped["atmos.remediation"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "Enable block public access on the S3 bucket.", aiRem["description"])
	assert.Equal(t, "atmos terraform apply s3-bucket -s tenant1-ue1-prod", aiRem["deploy_command"])
}

func TestBuildOCSFEvents_VulnerabilityMapping(t *testing.T) {
	report := newTestSecurityReport()
	report.Findings[2].Vulnerability = &VulnerabilityDetails{
		CVEID:          "CVE-2024-12345",
		CWEIDs:         []string{"CWE-79", "CWE-89"},
		EPSSScore:      0.42,
		PackageName:    "lodash",
		PackageVersion: "4.17.20",
		FixedInVersion: "4.17.21",
		CVSS: []CVSSScore{
			{BaseScore: 7.5, Vector: "CVSS:3.1/AV:N/AC:L/PR:N/UI:N", Version: "3.1"},
		},
		ReferenceURLs: []string{"https://nvd.nist.gov/vuln/detail/CVE-2024-12345"},
	}

	events := BuildOCSFEvents(report)
	require.NotEmpty(t, events)
	// finding-3 with vuln is low-severity, so it lands last after severity sort.
	vulnEvent := events[len(events)-1]
	require.Len(t, vulnEvent.Vulnerabilities, 1)

	v := vulnEvent.Vulnerabilities[0]
	require.NotNil(t, v.CVE)
	assert.Equal(t, "CVE-2024-12345", v.CVE.UID)
	assert.Equal(t, "CWE-79", v.CVE.CWEUID, "first CWE goes onto cve.cwe_uid")
	require.NotNil(t, v.CVE.EPSS)
	assert.Equal(t, "0.42", v.CVE.EPSS.Score, "OCSF 1.4.0 schemas EPSS.score as a string")
	require.Len(t, v.CVE.CVSS, 1)
	assert.Equal(t, 7.5, v.CVE.CVSS[0].BaseScore)
	assert.Equal(t, "3.1", v.CVE.CVSS[0].Version)

	require.Len(t, v.AffectedPackages, 1)
	assert.Equal(t, "lodash", v.AffectedPackages[0].Name)
	assert.Equal(t, "4.17.21", v.AffectedPackages[0].FixedInVersion)

	require.Contains(t, vulnEvent.Unmapped, "atmos.vulnerability.cwe_ids",
		"CWEs beyond the first are preserved in unmapped")

	require.Contains(t, vulnEvent.Metadata.Profiles, "vulnerability",
		"presence of Vulnerability activates the vulnerability profile")
}

func TestBuildOCSFEvents_LifecycleStatusMapping(t *testing.T) {
	report := newTestSecurityReport()
	report.Findings[0].SourceLifecycle = &SourceLifecycle{WorkflowStatus: "RESOLVED", RecordState: "ACTIVE"}
	events := BuildOCSFEvents(report)
	assert.Equal(t, ocsfStatusResolved, events[0].StatusID)
	assert.Equal(t, "Resolved", events[0].Status)
	assert.Equal(t, "RESOLVED", events[0].StatusCode)
	assert.Equal(t, "ACTIVE", events[0].StatusDetail)
}

func TestBuildOCSFEvents_EmptyReport(t *testing.T) {
	assert.Equal(t, []OCSFEvent{}, BuildOCSFEvents(nil))
	assert.Equal(t, []OCSFEvent{}, BuildOCSFEvents(&Report{}))
}

func TestBuildOCSFEvents_Deterministic(t *testing.T) {
	report := newTestSecurityReport()
	a, err := json.Marshal(BuildOCSFEvents(report))
	require.NoError(t, err)
	b, err := json.Marshal(BuildOCSFEvents(report))
	require.NoError(t, err)
	assert.True(t, bytes.Equal(a, b),
		"BuildOCSFEvents must be byte-stable for the same input")
}

func TestBuildOCSFEvents_ResourceTagsSortedDeterministically(t *testing.T) {
	report := newTestSecurityReport()
	report.Findings[0].ResourceTags = map[string]string{"Zone": "us-east-1a", "App": "billing", "Mid": "x"}

	first, err := json.Marshal(BuildOCSFEvents(report))
	require.NoError(t, err)
	for i := 0; i < 5; i++ {
		next, err := json.Marshal(BuildOCSFEvents(report))
		require.NoError(t, err)
		assert.True(t, bytes.Equal(first, next), "resource tags must serialize in stable order across runs")
	}

	events := BuildOCSFEvents(report)
	tags := events[0].Resources[0].Tags
	require.Len(t, tags, 3)
	assert.Equal(t, []string{"App", "Mid", "Zone"}, []string{tags[0].Name, tags[1].Name, tags[2].Name})
}

func TestBuildOCSFEvents_TypeUIDFromActivity(t *testing.T) {
	report := newTestSecurityReport()
	// finding-2 has UpdatedAt > CreatedAt only if we set them; default both zero → Create.
	events := BuildOCSFEvents(report)
	for _, e := range events {
		assert.Equal(t, ocsfActivityCreate, e.ActivityID)
		assert.Equal(t, ocsfTypeUIDCreate, e.TypeUID)
	}

	report.Findings[0].CreatedAt = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	report.Findings[0].UpdatedAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	events = BuildOCSFEvents(report)
	assert.Equal(t, ocsfActivityUpdate, events[0].ActivityID)
	assert.Equal(t, ocsfTypeUIDUpdate, events[0].TypeUID)

	report.Findings[0].SourceLifecycle = &SourceLifecycle{RecordState: "ARCHIVED"}
	events = BuildOCSFEvents(report)
	assert.Equal(t, ocsfActivityClose, events[0].ActivityID)
	assert.Equal(t, ocsfTypeUIDClose, events[0].TypeUID)
}

func TestBuildComplianceOCSFEvents_Basic(t *testing.T) {
	events := buildComplianceOCSFEvents(newTestComplianceReport())
	require.Len(t, events, 2)

	// Sorted critical first → CIS.1.14
	first := events[0]
	assert.Equal(t, "CIS.1.14", first.FindingInfo.UID)
	assert.Equal(t, "Ensure MFA is enabled for root", first.FindingInfo.Title)
	assert.Contains(t, first.FindingInfo.Types, "Compliance")
	assert.Contains(t, first.FindingInfo.Types, "CIS AWS Foundations Benchmark v1.4")
	assert.Equal(t, ocsfSeverityCritical, first.SeverityID)
	require.NotNil(t, first.Remediation)
	assert.Equal(t, "Enable MFA on root account.", first.Remediation.Desc)

	// Stack/component come through as enrichments.
	names := []string{}
	for _, e := range first.Enrichments {
		names = append(names, e.Name)
	}
	assert.Contains(t, names, "atmos.component")
	assert.Contains(t, names, "atmos.stack")
}

func TestBuildComplianceOCSFEvents_ZeroGeneratedAtUsesZeroTime(t *testing.T) {
	report := newTestComplianceReport()
	report.GeneratedAt = time.Time{}

	events := buildComplianceOCSFEvents(report)
	require.NotEmpty(t, events)
	assert.Equal(t, int64(0), events[0].Time)

	a, err := json.Marshal(events)
	require.NoError(t, err)
	b, err := json.Marshal(buildComplianceOCSFEvents(report))
	require.NoError(t, err)
	assert.True(t, bytes.Equal(a, b), "zero GeneratedAt compliance output must be byte-stable")
}

func TestBuildComplianceOCSFEvents_EmptyReport(t *testing.T) {
	assert.Equal(t, []OCSFEvent{}, buildComplianceOCSFEvents(nil))
	assert.Equal(t, []OCSFEvent{}, buildComplianceOCSFEvents(&ComplianceReport{}))
}

func TestOCSFRenderer_RenderSecurityReport_RoundTrip(t *testing.T) {
	r := NewReportRenderer(FormatOCSF)
	var buf bytes.Buffer
	require.NoError(t, r.RenderSecurityReport(&buf, newTestSecurityReport()))

	var events []OCSFEvent
	require.NoError(t, json.Unmarshal(buf.Bytes(), &events))
	require.Len(t, events, 3)
	assert.Equal(t, ocsfClassUID, events[0].ClassUID)
}

func TestOCSFRenderer_RenderComplianceReport_RoundTrip(t *testing.T) {
	r := NewReportRenderer(FormatOCSF)
	var buf bytes.Buffer
	require.NoError(t, r.RenderComplianceReport(&buf, newTestComplianceReport()))

	var events []OCSFEvent
	require.NoError(t, json.Unmarshal(buf.Bytes(), &events))
	require.Len(t, events, 2)
	assert.Contains(t, events[0].FindingInfo.Types, "Compliance")
}

func TestParseOutputFormat_OCSF(t *testing.T) {
	f, err := ParseOutputFormat("ocsf")
	require.NoError(t, err)
	assert.Equal(t, FormatOCSF, f)

	f, err = ParseOutputFormat("OCSF")
	require.NoError(t, err)
	assert.Equal(t, FormatOCSF, f)
}
