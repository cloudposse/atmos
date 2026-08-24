package security

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/stretchr/testify/require"
)

// ocsfSchemaJSON embeds the upstream OCSF 1.4.0 Detection Finding JSON Schema
// (resolved with cloud + compliance + vulnerability profiles) so validation
// runs offline and is reproducible across platforms. The file is fetched once
// from
// https://schema.ocsf.io/schema/1.4.0/classes/detection_finding?profiles=cloud,compliance,vulnerability
// and committed verbatim.
//
//go:embed testdata/ocsf-1.4.0-detection_finding.json
var ocsfSchemaJSON []byte

func compileOCSFSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	const schemaURL = "https://schemastore.atmos.tools/ocsf-1.4.0-detection-finding.json"
	require.NoError(t, compiler.AddResource(schemaURL, bytes.NewReader(ocsfSchemaJSON)),
		"register embedded OCSF Detection Finding schema")
	schema, err := compiler.Compile(schemaURL)
	require.NoError(t, err, "compile OCSF Detection Finding schema")
	return schema
}

// validateAgainstOCSFSchema marshals each event, decodes into a generic value,
// and validates against the embedded OCSF schema. The decode step normalizes
// Go types into the JSON-unmarshalled shape (map[string]any, []any, etc.) that
// santhosh-tekuri/jsonschema expects.
func validateAgainstOCSFSchema(t *testing.T, schema *jsonschema.Schema, events []OCSFEvent) {
	t.Helper()
	for i := range events {
		raw, err := json.Marshal(events[i])
		require.NoErrorf(t, err, "marshal OCSF event %d", i)
		var generic any
		require.NoErrorf(t, json.Unmarshal(raw, &generic), "decode OCSF event %d", i)
		if err := schema.Validate(generic); err != nil {
			t.Fatalf("OCSF event %d (uid=%s) fails 1.4.0 schema validation:\n%s\n\n--- offending document ---\n%s",
				i, events[i].FindingInfo.UID, err, raw)
		}
	}
}

func TestBuildOCSFEvents_ValidatesAgainstOCSF140Schema(t *testing.T) {
	schema := compileOCSFSchema(t)
	events := BuildOCSFEvents(newTestSecurityReport())
	require.NotEmpty(t, events)
	validateAgainstOCSFSchema(t, schema, events)
}

func TestBuildOCSFEvents_ValidatesAgainstSchema_WithVulnerability(t *testing.T) {
	schema := compileOCSFSchema(t)
	report := newTestSecurityReport()
	report.Findings[0].Vulnerability = &VulnerabilityDetails{
		CVEID:         "CVE-2024-12345",
		CWEIDs:        []string{"CWE-79"},
		EPSSScore:     0.42,
		CVSS:          []CVSSScore{{BaseScore: 7.5, Version: "3.1", Vector: "CVSS:3.1/AV:N/AC:L/PR:N/UI:N"}},
		ReferenceURLs: []string{"https://nvd.nist.gov/vuln/detail/CVE-2024-12345"},
		Packages: []VulnerablePackage{
			{Name: "lodash", Version: "4.17.20", FixedInVersion: "4.17.21", PackageManager: "npm"},
		},
	}
	validateAgainstOCSFSchema(t, schema, BuildOCSFEvents(report))
}

func TestBuildOCSFEvents_ValidatesAgainstSchema_WithSourceFields(t *testing.T) {
	schema := compileOCSFSchema(t)
	report := newTestSecurityReport()
	score := 9.5
	report.Findings[0].SourceSeverity = &SourceSeverity{Score: &score, Label: "CRITICAL"}
	report.Findings[0].SourceLifecycle = &SourceLifecycle{
		WorkflowStatus:   "NOTIFIED",
		RecordState:      "ACTIVE",
		ComplianceStatus: "FAILED",
	}
	report.Findings[0].SourceRemediation = &SourceRemediation{
		Text: "Enable S3 Block Public Access.",
		URL:  "https://docs.aws.amazon.com/securityhub/",
	}
	report.Findings[0].SourceURL = "https://console.aws.amazon.com/securityhub/home"
	validateAgainstOCSFSchema(t, schema, BuildOCSFEvents(report))
}

func TestBuildComplianceOCSFEvents_ValidatesAgainstOCSF140Schema(t *testing.T) {
	schema := compileOCSFSchema(t)
	events := buildComplianceOCSFEvents(newTestComplianceReport())
	require.NotEmpty(t, events)
	validateAgainstOCSFSchema(t, schema, events)
}
