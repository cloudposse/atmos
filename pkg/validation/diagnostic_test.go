package validation

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci"
)

func TestReportToAnnotations(t *testing.T) {
	report := Report{Diagnostics: []Diagnostic{
		{RuleID: "second", Severity: SeverityWarning, Message: "later", File: "b.tf", Line: 2},
		{RuleID: "first", Severity: SeverityError, Message: "first", File: "a.tf", Line: 1},
	}}

	assert.Equal(t, []ci.Annotation{
		{Path: "a.tf", StartLine: 1, Level: ci.AnnotationError, Title: "first", Message: "first"},
		{Path: "b.tf", StartLine: 2, Level: ci.AnnotationWarning, Title: "second", Message: "later"},
	}, report.ToAnnotations())
	assert.True(t, report.HasErrors())
}

func TestReportSARIF(t *testing.T) {
	report := Report{Diagnostics: []Diagnostic{{
		Source: "editorconfig", RuleID: "editorconfig", Severity: SeverityError,
		Message: "wrong indentation", File: "config/context.tf", Line: 42,
	}}}

	body, err := report.SARIF()
	require.NoError(t, err)
	var document map[string]any
	require.NoError(t, json.Unmarshal(body, &document))
	assert.Equal(t, "2.1.0", document["version"])
	runs := document["runs"].([]any)
	results := runs[0].(map[string]any)["results"].([]any)
	location := results[0].(map[string]any)["locations"].([]any)[0].(map[string]any)["physicalLocation"].(map[string]any)
	assert.Equal(t, "config/context.tf", location["artifactLocation"].(map[string]any)["uri"])
	assert.EqualValues(t, 42, location["region"].(map[string]any)["startLine"])
}

func TestEmptyReportSARIF(t *testing.T) {
	body, err := (Report{}).SARIF()
	require.NoError(t, err)
	var document struct {
		Version string `json:"version"`
		Runs    []struct {
			Results []json.RawMessage `json:"results"`
		} `json:"runs"`
	}
	require.NoError(t, json.Unmarshal(body, &document))
	assert.Equal(t, "2.1.0", document.Version)
	require.Len(t, document.Runs, 1)
	assert.Empty(t, document.Runs[0].Results)
}
