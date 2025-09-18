package test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/tools/gotcha/internal/parser"
)

// TestParser_WithRealGoTestOutput verifies the parser works with real go test JSON output.
func TestParser_WithRealGoTestOutput(t *testing.T) {
	// Skip unless explicitly enabled to prevent recursive test execution
	if os.Getenv("GOTCHA_ENABLE_INTEGRATION_TESTS") != "1" {
		t.Skipf("Skipping integration test: set GOTCHA_ENABLE_INTEGRATION_TESTS=1 to run")
	}
	
	tests := []struct {
		name          string
		testDir       string
		expectPass    int
		expectFail    int
		expectSkip    int
		shouldCmdFail bool
	}{
		{
			name:          "All passing tests",
			testDir:       filepath.Join("testdata", "passing_tests"),
			expectPass:    6, // TestPass1, TestPass2, TestPass3, TestPassWithSubtests, plus 2 subtests
			expectFail:    0,
			expectSkip:    0,
			shouldCmdFail: false,
		},
		{
			name:          "All failing tests",
			testDir:       filepath.Join("testdata", "failing_tests"),
			expectPass:    0,
			expectFail:    3, // TestFail1, TestFail2, TestFailWithMessage
			expectSkip:    0,
			shouldCmdFail: true,
		},
		{
			name:          "All skipping tests",
			testDir:       filepath.Join("testdata", "skipping_tests"),
			expectPass:    1, // TestSkipWithReason passes in non-short mode
			expectFail:    0,
			expectSkip:    3, // TestSkip1, TestSkip2, TestConditionalSkip
			shouldCmdFail: false,
		},
		{
			name:          "Mixed test results",
			testDir:       filepath.Join("testdata", "mixed_tests"),
			expectPass:    4, // TestPass1, TestPass2, TestPass3, TestSkip2 (passes in non-short mode)
			expectFail:    2, // TestFail1, TestFail2
			expectSkip:    1, // TestSkip1
			shouldCmdFail: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Run go test with JSON output
			cmd := exec.Command("go", "test", "-json", "./...")
			cmd.Dir = tt.testDir
			cmd.Env = append(os.Environ(), "GOWORK=off") // Disable workspace to avoid module conflicts
			output, err := cmd.CombinedOutput()

			// Check if command failed as expected
			if tt.shouldCmdFail {
				assert.Error(t, err, "Expected command to fail due to test failures")
			} else {
				assert.NoError(t, err, "Expected command to succeed")
			}

			// Always should have output
			require.NotEmpty(t, output, "Should have JSON output")

			// Parse the JSON output
			summary, err := parser.ParseTestJSON(bytes.NewReader(output), "", false)
			require.NoError(t, err, "Parser should handle output successfully")

			// Verify counts
			assert.Equal(t, tt.expectPass, len(summary.Passed),
				"Unexpected number of passing tests")
			assert.Equal(t, tt.expectFail, len(summary.Failed),
				"Unexpected number of failing tests")
			assert.Equal(t, tt.expectSkip, len(summary.Skipped),
				"Unexpected number of skipped tests")
		})
	}
}

// TestParser_HandlesEmptyCoverage verifies the parser handles tests without coverage.
func TestParser_HandlesEmptyCoverage(t *testing.T) {
	// Skip unless explicitly enabled to prevent recursive test execution
	if os.Getenv("GOTCHA_ENABLE_INTEGRATION_TESTS") != "1" {
		t.Skipf("Skipping integration test: set GOTCHA_ENABLE_INTEGRATION_TESTS=1 to run")
	}
	
	// Run go test without coverage
	testDir := filepath.Join("testdata", "passing_tests")
	cmd := exec.Command("go", "test", "-json", "./...")
	cmd.Dir = testDir
	cmd.Env = append(os.Environ(), 
		"GOWORK=off", // Disable workspace to avoid module conflicts
		"GOTCHA_TEST_RECURSIVE=1") // Prevent recursive execution
	output, err := cmd.CombinedOutput()
	require.NoError(t, err)

	// Parse without coverage profile
	summary, err := parser.ParseTestJSON(bytes.NewReader(output), "", false)
	require.NoError(t, err)

	// Should handle missing coverage gracefully
	// Coverage field will be empty when no coverage is requested
	assert.Equal(t, "", summary.Coverage, "Coverage should be empty without -cover flag")
}
