package validation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFromGCCTextParsesLocationBearingLines(t *testing.T) {
	report := FromGCCText("stacks", "stacks/orgs/acme/platform/dev.yaml:12:5: error: unexpected field")
	require.Len(t, report.Diagnostics, 1)
	diagnostic := report.Diagnostics[0]
	assert.Equal(t, "stacks/orgs/acme/platform/dev.yaml", diagnostic.File)
	assert.Equal(t, 12, diagnostic.Line)
	assert.Equal(t, 5, diagnostic.Column)
	assert.Equal(t, "unexpected field", diagnostic.Message)
}

func TestFromGCCTextFallsBackToRawMessageWhenNoLocationFound(t *testing.T) {
	report := FromGCCText("stacks", "general stack setup failure with no location")
	require.Len(t, report.Diagnostics, 1)
	assert.Equal(t, "general stack setup failure with no location", report.Diagnostics[0].Message)
	assert.Empty(t, report.Diagnostics[0].File)
}

func TestFromGCCTextReturnsNoDiagnosticsForEmptyMessage(t *testing.T) {
	report := FromGCCText("stacks", "   ")
	assert.Empty(t, report.Diagnostics)
}

// TestFromGCCTextPreservesUnmatchedTextAlongsideParsedDiagnostics guards
// against dropping generic, non-location-bearing lines when they appear
// alongside GCC-formatted diagnostics in the same message.
func TestFromGCCTextPreservesUnmatchedTextAlongsideParsedDiagnostics(t *testing.T) {
	message := "stacks/orgs/acme/platform/dev.yaml:12:5: error: unexpected field\n" +
		"general stack setup failure with no location"
	report := FromGCCText("stacks", message)

	require.Len(t, report.Diagnostics, 2)

	located := report.Diagnostics[0]
	assert.Equal(t, "stacks/orgs/acme/platform/dev.yaml", located.File)
	assert.Equal(t, 12, located.Line)
	assert.Equal(t, 5, located.Column)
	assert.Equal(t, "unexpected field", located.Message)

	generic := report.Diagnostics[1]
	assert.Empty(t, generic.File)
	assert.Equal(t, "general stack setup failure with no location", generic.Message)
}
