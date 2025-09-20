//go:build gotcha_binary_integration
// +build gotcha_binary_integration

// DEPRECATED: This test builds the gotcha binary. Use testdata approach instead.
// See parser_integration_test.go for the recommended pattern.
package test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/tools/gotcha/pkg/constants"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestActualShowFailedBug reproduces the ACTUAL bug where gotcha shows passed tests
// even with "show: failed" when all tests pass
func TestActualShowFailedBug_AllTestsPass(t *testing.T) {
	// Create a temporary directory for our test
	tempDir := t.TempDir()

	// Create test files with ONLY PASSING tests
	testFile1 := filepath.Join(tempDir, "pass1_test.go")
	testContent1 := `package main

import (
	"github.com/cloudposse/atmos/tools/gotcha/pkg/constants"
	"testing"
)

func TestPass1(t *testing.T) {
	// This test passes
}

func TestPass2(t *testing.T) {
	// Another passing test
}

func TestPass3(t *testing.T) {
	// Yet another passing test
}
`
	err := os.WriteFile(testFile1, []byte(testContent1), constants.DefaultFilePerms)
	require.NoError(t, err)

	// Create go.mod for the test package
	goModFile := filepath.Join(tempDir, "go.mod")
	goModContent := `module testpkg

go 1.21
`
	err = os.WriteFile(goModFile, []byte(goModContent), constants.DefaultFilePerms)
	require.NoError(t, err)

	// Create exact .gotcha.yaml config as user provided
	configFile := filepath.Join(tempDir, ".gotcha.yaml")
	configContent := `# Gotcha Configuration File
# Configuration for the gotcha test summary tool

# Output format: stdin, markdown, both, github, or stream
format: stream

# Space-separated list of packages to test
packages:
  - "./..."

# Additional arguments to pass to go test
testargs: "-timeout 40m"

# Filter displayed tests: all, failed, passed, skipped
show: failed

# Output file for test results
output: test-results.json

# Coverage profile file for detailed analysis
coverprofile: coverage.out

# Exclude mock files from coverage calculations
exclude-mocks: true

# Alert configuration
# Emit a terminal bell (\a) sound when tests complete
alert: false

# Package filtering configuration
filter:
  # Regex patterns to include packages (default: include all)
  include:
    - ".*"
  
  # Regex patterns to exclude packages
  exclude: []
`
	err = os.WriteFile(configFile, []byte(configContent), constants.DefaultFilePerms)
	require.NoError(t, err)

	// Build the gotcha binary
	gotchaBinary := filepath.Join(tempDir, "gotcha-test-binary")
	gotchaDir, err := filepath.Abs("..")
	require.NoError(t, err)

	buildCmd := CreateGotchaCommand("go", "build", "-o", gotchaBinary, ".")
	buildCmd.Dir = gotchaDir
	buildOut, buildErr := buildCmd.CombinedOutput()
	if buildErr != nil {
		t.Fatalf("Failed to build gotcha binary: %v\nOutput: %s", buildErr, buildOut)
	}

	// Run gotcha WITHOUT any subcommand - this reproduces the exact user scenario
	cmd := CreateGotchaCommand(gotchaBinary, ".")
	cmd.Dir = tempDir

	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Run the command (should succeed since all tests pass)
	err = cmd.Run()
	require.NoError(t, err, "Command should succeed when all tests pass")

	// Get the combined output
	output := stderr.String() + stdout.String()
	t.Logf("=====================================")
	t.Logf("Gotcha output with 'show: failed' and ALL PASSING tests:")
	t.Logf("=====================================")
	t.Logf("%s", output)
	t.Logf("=====================================")

	// THIS IS THE BUG: With show: failed and all tests passing,
	// we should see NO test details at all, but gotcha shows "All X tests passed"

	// Check for the problematic output
	if strings.Contains(output, "All") && strings.Contains(output, "tests passed") {
		t.Errorf("BUG CONFIRMED: gotcha shows 'All X tests passed' even with 'show: failed' filter")
		t.Logf("Expected: No individual test output with 'show: failed' when all tests pass")
		t.Logf("Actual: Shows summary line about all tests passing")
	}

	// Also check if any individual test lines are shown
	var individualTestsShown []string
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "TestPass") && strings.Contains(line, "✔") {
			individualTestsShown = append(individualTestsShown, line)
		}
	}

	if len(individualTestsShown) > 0 {
		t.Errorf("BUG: Individual passing tests shown with 'show: failed':")
		for _, test := range individualTestsShown {
			t.Logf("  %s", test)
		}
	}

	// The correct behavior with show: failed and all tests passing should be:
	// - No individual test lines
	// - Still show the summary statistics (total, passed, failed, etc.)
	// - But NOT the "All X tests passed" line in the package section
	assert.NotContains(t, output, "✔ TestPass", "Individual passing tests should not be shown")
	assert.NotContains(t, output, "All", "Should not show 'All X tests passed' with show: failed")
}

// TestShowFailedBug_MixedResults tests show: failed with mixed test results
func TestShowFailedBug_MixedResults(t *testing.T) {
	// Create a temporary directory for our test
	tempDir := t.TempDir()

	// Create test files with mixed results
	testFile := filepath.Join(tempDir, "mixed_test.go")
	testContent := `package main

import (
	"github.com/cloudposse/atmos/tools/gotcha/pkg/constants"
	"testing"
)

func TestPass1(t *testing.T) {}
func TestPass2(t *testing.T) {}
func TestPass3(t *testing.T) {}
func TestFail1(t *testing.T) { t.Fatal("fail") }
func TestSkip1(t *testing.T) { t.Skip("skip") }
`
	err := os.WriteFile(testFile, []byte(testContent), constants.DefaultFilePerms)
	require.NoError(t, err)

	// Create go.mod
	goModFile := filepath.Join(tempDir, "go.mod")
	goModContent := `module testpkg
go 1.21
`
	err = os.WriteFile(goModFile, []byte(goModContent), constants.DefaultFilePerms)
	require.NoError(t, err)

	// Create .gotcha.yaml with show: failed
	configFile := filepath.Join(tempDir, ".gotcha.yaml")
	configContent := `format: stream
show: failed
packages:
  - "."
`
	err = os.WriteFile(configFile, []byte(configContent), constants.DefaultFilePerms)
	require.NoError(t, err)

	// Build the gotcha binary
	gotchaBinary := filepath.Join(tempDir, "gotcha-test-binary")
	gotchaDir, err := filepath.Abs("..")
	require.NoError(t, err)

	buildCmd := CreateGotchaCommand("go", "build", "-o", gotchaBinary, ".")
	buildCmd.Dir = gotchaDir
	buildOut, buildErr := buildCmd.CombinedOutput()
	if buildErr != nil {
		t.Fatalf("Failed to build: %v\n%s", buildErr, buildOut)
	}

	// Run gotcha
	cmd := CreateGotchaCommand(gotchaBinary, ".")
	cmd.Dir = tempDir

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	_ = cmd.Run() // Ignore error from failed test

	outputStr := output.String()
	t.Logf("Output with mixed results and 'show: failed':\n%s", outputStr)

	// Count what's shown
	passCount := strings.Count(outputStr, "✔ TestPass")
	failCount := strings.Count(outputStr, "✘ TestFail")
	skipCount := strings.Count(outputStr, "⊘ TestSkip")

	// With show: failed, we should see only failed and skipped tests
	assert.Equal(t, 0, passCount, "No passing tests should be individually shown with 'show: failed'")
	assert.Greater(t, failCount, 0, "Failed tests should be shown")
	assert.Greater(t, skipCount, 0, "Skipped tests should be shown")
}

// TestCacheYamlLogging tests that cache.yaml should log ALL tests regardless of filter
func TestCacheYamlLogging(t *testing.T) {
	t.Skip("Cache functionality regression - to be fixed")

	// This test would verify that:
	// 1. cache.yaml is created/updated
	// 2. It contains ALL test information, not just filtered tests
	// 3. Test run information is logged
	// 4. This allows estimation of how many tests match a filter
}
