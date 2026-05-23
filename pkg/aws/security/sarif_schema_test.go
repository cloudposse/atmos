package security

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/stretchr/testify/require"
)

// sarifSchemaJSON embeds the upstream SARIF 2.1.0 JSON Schema (draft-04) so
// validation runs offline and is reproducible across platforms. The file is
// copied verbatim from
// https://raw.githubusercontent.com/oasis-tcs/sarif-spec/main/sarif-2.1/schema/sarif-schema-2.1.0.json
//
//go:embed testdata/sarif-schema-2.1.0.json
var sarifSchemaJSON []byte

// compileSARIFSchema parses the embedded SARIF 2.1.0 schema. Test helper —
// keeps the per-test setup terse.
func compileSARIFSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	const schemaURL = "https://schemastore.atmos.tools/sarif-2.1.0.json"
	require.NoError(t, compiler.AddResource(schemaURL, bytes.NewReader(sarifSchemaJSON)),
		"register embedded SARIF 2.1.0 schema")
	schema, err := compiler.Compile(schemaURL)
	require.NoError(t, err, "compile SARIF 2.1.0 schema")
	return schema
}

// validateAgainstSARIFSpec marshals doc, decodes back into a generic value,
// and validates it against the embedded SARIF 2.1.0 schema. The decode step
// is required because santhosh-tekuri/jsonschema/v5 validates Go values that
// it expects to be of unmarshalled-JSON shape (map[string]any, []any, etc.).
func validateAgainstSARIFSpec(t *testing.T, schema *jsonschema.Schema, doc *SARIFLog) {
	t.Helper()
	raw, err := json.Marshal(doc)
	require.NoError(t, err, "marshal SARIF log")
	var generic any
	require.NoError(t, json.Unmarshal(raw, &generic), "decode SARIF log into generic JSON")
	if err := schema.Validate(generic); err != nil {
		// jsonschema errors are deeply nested — surface the full chain so a
		// schema violation is immediately diagnosable.
		t.Fatalf("SARIF document fails 2.1.0 schema validation:\n%s\n\n--- offending document ---\n%s", err, raw)
	}
}

func TestBuildSARIFLog_ValidatesAgainstSARIF210Schema(t *testing.T) {
	schema := compileSARIFSchema(t)
	validateAgainstSARIFSpec(t, schema, BuildSARIFLog(newTestSecurityReport()))
}

func TestBuildSARIFLog_EmptyReportValidatesAgainstSchema(t *testing.T) {
	schema := compileSARIFSchema(t)
	// nil report (callers may produce one when no findings exist).
	validateAgainstSARIFSpec(t, schema, BuildSARIFLog(nil))
	// Empty-but-non-nil report (zero findings).
	validateAgainstSARIFSpec(t, schema, BuildSARIFLog(&Report{
		GeneratedAt: fixedTime,
	}))
}

func TestBuildSARIFLog_UnmappedFindingValidatesAgainstSchema(t *testing.T) {
	// Unmapped findings emit logical-only locations — confirm the resulting
	// shape still satisfies the SARIF schema (logical and physical locations
	// have different required fields).
	schema := compileSARIFSchema(t)
	report := &Report{
		GeneratedAt: fixedTime,
		Findings: []Finding{
			{
				ID:          "f-unmapped",
				Title:       "Unmapped finding",
				Severity:    SeverityHigh,
				Source:      SourceSecurityHub,
				ResourceARN: "arn:aws:s3:::orphan-bucket",
			},
		},
	}
	validateAgainstSARIFSpec(t, schema, BuildSARIFLog(report))
}

func TestBuildComplianceSARIFLog_ValidatesAgainstSARIF210Schema(t *testing.T) {
	schema := compileSARIFSchema(t)
	report := &ComplianceReport{
		Framework: "cis-aws",
		FailingDetails: []ComplianceControl{
			{ControlID: "CIS.1.1", Title: "Root user MFA", Severity: SeverityHigh, Stack: "ue1", Component: "iam"},
			{ControlID: "CIS.2.1", Title: "CloudTrail enabled", Severity: SeverityCritical, Stack: "ue1", Component: "cloudtrail"},
			// One control without an explicit ControlID — exercises slug fallback path.
			{Title: "Encryption at rest required", Severity: SeverityMedium},
		},
	}
	validateAgainstSARIFSpec(t, schema, buildComplianceSARIFLog(report))
}

func TestSARIFRenderer_OutputValidatesAgainstSchema(t *testing.T) {
	// Belt-and-suspenders: validate what the renderer actually writes, not
	// just the in-memory document. Catches any JSON-marshalling drift.
	schema := compileSARIFSchema(t)
	r := &sarifRenderer{}

	var buf bytes.Buffer
	require.NoError(t, r.RenderSecurityReport(&buf, newTestSecurityReport()))
	var generic any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &generic))
	if err := schema.Validate(generic); err != nil {
		t.Fatalf("RenderSecurityReport output fails schema:\n%s\n\n%s", err, buf.String())
	}

	buf.Reset()
	require.NoError(t, r.RenderComplianceReport(&buf, &ComplianceReport{
		Framework: "cis-aws",
		FailingDetails: []ComplianceControl{
			{ControlID: "CIS.1.1", Title: "Root user MFA", Severity: SeverityHigh, Stack: "ue1", Component: "iam"},
		},
	}))
	require.NoError(t, json.Unmarshal(buf.Bytes(), &generic))
	if err := schema.Validate(generic); err != nil {
		t.Fatalf("RenderComplianceReport output fails schema:\n%s\n\n%s", err, buf.String())
	}
}

// TestSARIFSchema_DetectsInvalidDocument is a sanity check on the validation
// pipeline itself: hand it a deliberately broken SARIF (wrong version), and
// confirm the schema rejects it. Without this, a silent regression in the
// schema compile/validate plumbing would let everything pass.
func TestSARIFSchema_DetectsInvalidDocument(t *testing.T) {
	schema := compileSARIFSchema(t)
	bad := map[string]any{
		"version": "1.0.0", // schema requires "2.1.0".
		"runs":    []any{},
	}
	err := schema.Validate(bad)
	require.Error(t, err, "schema must reject SARIF docs with the wrong version")
	require.Contains(t, strings.ToLower(err.Error()), "version",
		"validator error should mention the version mismatch")
}
