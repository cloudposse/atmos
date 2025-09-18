package markdown

import (
	"bytes"
	"testing"

	"github.com/cloudposse/atmos/tools/gotcha/pkg/types"
	"github.com/stretchr/testify/assert"
)

// TestWriteContentWithExitDiagnostic tests that exit code diagnostics are included in output
func TestWriteContentWithExitDiagnostic(t *testing.T) {
	tests := []struct {
		name           string
		summary        *types.TestSummary
		expectedPhrases []string
	}{
		{
			name: "with exit code diagnostic",
			summary: &types.TestSummary{
				Passed: []types.TestResult{
					{Package: "pkg1", Test: "TestFoo", Status: "pass", Duration: 1.0},
					{Package: "pkg2", Test: "TestBar", Status: "pass", Duration: 0.5},
				},
				Failed: []types.TestResult{},
				Skipped: []types.TestResult{},
				ExitCodeDiagnostic: `TestMain initialization failed but continued execution.

Found log messages indicating early failure:
  - 'Failed to locate git repository'

This suggests TestMain encountered an error but didn't properly exit. Check that TestMain:
  1. Properly handles initialization errors
  2. Calls os.Exit(m.Run()) even when early errors occur
  3. Doesn't use logger.Fatal() from charmbracelet/log (which doesn't exit)`,
			},
			expectedPhrases: []string{
				"⚠️", // Warning emoji in title
				"Process Exit Issue",
				"All tests passed but the test process exited with a non-zero code",
				"Click for diagnostic details",
				"TestMain initialization failed",
				"Failed to locate git repository",
				"os.Exit(m.Run())",
			},
		},
		{
			name: "without exit code diagnostic",
			summary: &types.TestSummary{
				Passed: []types.TestResult{
					{Package: "pkg1", Test: "TestFoo", Status: "pass", Duration: 1.0},
				},
				Failed: []types.TestResult{},
				Skipped: []types.TestResult{},
				ExitCodeDiagnostic: "", // No diagnostic
			},
			expectedPhrases: []string{
				"✅", // Success emoji (not warning)
				"Test Results",
			},
		},
		{
			name: "with failures no diagnostic shown",
			summary: &types.TestSummary{
				Passed: []types.TestResult{
					{Package: "pkg1", Test: "TestFoo", Status: "pass", Duration: 1.0},
				},
				Failed: []types.TestResult{
					{Package: "pkg1", Test: "TestBad", Status: "fail", Duration: 0.1},
				},
				Skipped: []types.TestResult{},
				ExitCodeDiagnostic: "Some diagnostic", // Should not be shown when there are failures
			},
			expectedPhrases: []string{
				"❌", // Failure emoji takes precedence
				"Test Results",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			WriteContent(&buf, tt.summary, "markdown")
			
			output := buf.String()
			
			for _, phrase := range tt.expectedPhrases {
				assert.Contains(t, output, phrase, 
					"Output should contain '%s' for test case %s", phrase, tt.name)
			}
			
			// Check that diagnostic section is NOT present when not needed
			if tt.summary.ExitCodeDiagnostic == "" || len(tt.summary.Failed) > 0 {
				assert.NotContains(t, output, "Process Exit Issue",
					"Should not show exit diagnostic when not needed")
			}
			
			// For debugging, log the output
			if t.Failed() {
				t.Logf("Output for %s:\n%s", tt.name, output)
			}
		})
	}
}

// TestDiagnosticInGitHubFormat tests that diagnostics appear in GitHub format output
func TestDiagnosticInGitHubFormat(t *testing.T) {
	summary := &types.TestSummary{
		Passed: []types.TestResult{
			{Package: "test/pkg", Test: "TestPass", Status: "pass", Duration: 0.5},
		},
		ExitCodeDiagnostic: "TestMain issue detected",
	}

	var buf bytes.Buffer
	WriteContent(&buf, summary, "github")
	
	output := buf.String()
	
	// Should contain warning emoji and diagnostic section
	assert.Contains(t, output, "⚠️")
	assert.Contains(t, output, "Process Exit Issue")
	assert.Contains(t, output, "TestMain issue detected")
	
	// Should use collapsible details
	assert.Contains(t, output, "<details>")
	assert.Contains(t, output, "</details>")
	assert.Contains(t, output, "<summary>")
}