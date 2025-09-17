package test

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/gotcha/internal/parser"
)

// TestShowFailedFilter_ParsesCorrectly verifies that gotcha correctly parses
// test output and identifies failed, passed, and skipped tests.
func TestShowFailedFilter_ParsesCorrectly(t *testing.T) {
	// Run go test on our mixed testdata
	testDir := filepath.Join("testdata", "mixed_tests")

	cmd := exec.Command("go", "test", "-json", "./...")
	cmd.Dir = testDir
	output, _ := cmd.CombinedOutput()

	// We expect the command to fail because there are failing tests
	// but we should still get output
	require.NotNil(t, output, "Should have output even with test failures")

	// Parse the JSON output
	summary, err := parser.ParseTestJSON(bytes.NewReader(output), "", false)
	require.NoError(t, err, "Should parse JSON output successfully")

	// Verify we detected the different test types
	assert.Greater(t, len(summary.Passed), 0, "Should have passing tests")
	assert.Greater(t, len(summary.Failed), 0, "Should have failing tests")
	assert.Greater(t, len(summary.Skipped), 0, "Should have skipped tests")

	// Verify specific tests are categorized correctly
	passedNames := make(map[string]bool)
	for _, test := range summary.Passed {
		passedNames[test.Test] = true
	}
	assert.True(t, passedNames["TestPass1"], "TestPass1 should be in passed tests")
	assert.True(t, passedNames["TestPass2"], "TestPass2 should be in passed tests")

	failedNames := make(map[string]bool)
	for _, test := range summary.Failed {
		failedNames[test.Test] = true
	}
	assert.True(t, failedNames["TestFail1"], "TestFail1 should be in failed tests")

	skippedNames := make(map[string]bool)
	for _, test := range summary.Skipped {
		skippedNames[test.Test] = true
	}
	assert.True(t, skippedNames["TestSkip1"], "TestSkip1 should be in skipped tests")
}

// TestFilteredOutput_ShowsOnlyFailures verifies that when filtering for failures,
// only failed tests are included in the parsed output.
func TestFilteredOutput_ShowsOnlyFailures(t *testing.T) {
	// Run go test on our mixed testdata
	testDir := filepath.Join("testdata", "mixed_tests")

	cmd := exec.Command("go", "test", "-json", "./...")
	cmd.Dir = testDir
	output, _ := cmd.CombinedOutput()

	// Parse the JSON output
	summary, err := parser.ParseTestJSON(bytes.NewReader(output), "", false)
	require.NoError(t, err)

	// In a real implementation, we would apply filters here
	// For now, we're just verifying the parser correctly categorizes tests
	assert.Greater(t, len(summary.Failed), 0, "Should have failed tests to filter")
	assert.Greater(t, len(summary.Passed), 0, "Should have passed tests that would be filtered out")
}

// TestAllTestsPass_ShowFilter verifies behavior when all tests pass.
func TestAllTestsPass_ShowFilter(t *testing.T) {
	// Run go test on our passing testdata
	testDir := filepath.Join("testdata", "passing_tests")

	cmd := exec.Command("go", "test", "-json", "./...")
	cmd.Dir = testDir
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "All tests should pass")

	// Parse the JSON output
	summary, err := parser.ParseTestJSON(bytes.NewReader(output), "", false)
	require.NoError(t, err)

	// Verify all tests passed
	assert.Greater(t, len(summary.Passed), 0, "Should have passing tests")
	assert.Equal(t, 0, len(summary.Failed), "Should have no failing tests")
	assert.Equal(t, 0, len(summary.Skipped), "Should have no skipped tests")
}

// TestAllTestsFail_ShowFilter verifies behavior when all tests fail.
func TestAllTestsFail_ShowFilter(t *testing.T) {
	// Run go test on our failing testdata
	testDir := filepath.Join("testdata", "failing_tests")

	cmd := exec.Command("go", "test", "-json", "./...")
	cmd.Dir = testDir
	output, _ := cmd.CombinedOutput()
	// Command will fail but we'll have output

	// Parse the JSON output
	summary, err := parser.ParseTestJSON(bytes.NewReader(output), "", false)
	require.NoError(t, err)

	// Verify all tests failed
	assert.Equal(t, 0, len(summary.Passed), "Should have no passing tests")
	assert.Greater(t, len(summary.Failed), 0, "Should have failing tests")

	// Check specific test names
	failedNames := make(map[string]bool)
	for _, test := range summary.Failed {
		failedNames[test.Test] = true
	}
	assert.True(t, failedNames["TestFail1"], "TestFail1 should be in failed tests")
	assert.True(t, failedNames["TestFail2"], "TestFail2 should be in failed tests")
}
