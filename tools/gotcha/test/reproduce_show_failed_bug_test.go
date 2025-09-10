package test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReproduceShowFailedBug reproduces the exact bug reported where
// gotcha displays all tests despite having "show: failed" in .gotcha.yaml
func TestReproduceShowFailedBug(t *testing.T) {
	// Create a temporary directory for our test
	tempDir := t.TempDir()

	// Create multiple test files to simulate a real project
	// First test file with various test outcomes
	testFile1 := filepath.Join(tempDir, "example1_test.go")
	testContent1 := `package main

import (
	"testing"
	"time"
)

func TestPass1(t *testing.T) {
	// This test passes
	if 1+1 != 2 {
		t.Fatal("math is broken")
	}
}

func TestPass2(t *testing.T) {
	// Another passing test
	time.Sleep(10 * time.Millisecond)
}

func TestFail1(t *testing.T) {
	t.Fatal("This test fails intentionally")
}

func TestSkip1(t *testing.T) {
	t.Skip("This test is skipped")
}
`
	err := os.WriteFile(testFile1, []byte(testContent1), 0644)
	require.NoError(t, err)

	// Second test file with more tests
	testFile2 := filepath.Join(tempDir, "example2_test.go")
	testContent2 := `package main

import (
	"testing"
)

func TestPass3(t *testing.T) {
	// Yet another passing test
}

func TestPass4(t *testing.T) {
	// One more passing test
}

func TestFail2(t *testing.T) {
	t.Error("This test also fails")
	t.FailNow()
}
`
	err = os.WriteFile(testFile2, []byte(testContent2), 0644)
	require.NoError(t, err)

	// Create go.mod for the test package
	goModFile := filepath.Join(tempDir, "go.mod")
	goModContent := `module testpkg

go 1.21
`
	err = os.WriteFile(goModFile, []byte(goModContent), 0644)
	require.NoError(t, err)

	// Create a .gotcha.yaml config file that EXACTLY matches the user's config
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
    # Example exclusions:
    # - "^tests"           # exclude packages starting with "tests"
    # - ".*mock.*"         # exclude packages containing "mock"
    # - ".*_test$"         # exclude test packages
`
	err = os.WriteFile(configFile, []byte(configContent), 0644)
	require.NoError(t, err)

	// Build the gotcha binary from the root of the gotcha directory
	gotchaBinary := filepath.Join(tempDir, "gotcha-test-binary")
	t.Logf("Building gotcha binary at %s", gotchaBinary)
	
	// Get the absolute path to the gotcha directory
	gotchaDir, err := filepath.Abs("..")
	require.NoError(t, err)
	
	buildCmd := exec.Command("go", "build", "-o", gotchaBinary, ".")
	buildCmd.Dir = gotchaDir
	buildOut, buildErr := buildCmd.CombinedOutput()
	if buildErr != nil {
		t.Fatalf("Failed to build gotcha binary: %v\nOutput: %s", buildErr, buildOut)
	}

	// Run gotcha WITHOUT any subcommand (just like the user is doing)
	// This should use the root command which should respect the config
	cmd := exec.Command(gotchaBinary, ".")
	cmd.Dir = tempDir
	
	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Run the command (we expect it to fail because we have failing tests)
	err = cmd.Run()
	// Don't check error - we expect non-zero exit due to test failures

	// Get the combined output
	output := stderr.String() + stdout.String()
	t.Logf("=====================================")
	t.Logf("Gotcha output with 'show: failed' config:")
	t.Logf("=====================================")
	t.Logf("%s", output)
	t.Logf("=====================================")

	// Parse the output to see what tests were displayed
	lines := strings.Split(output, "\n")
	
	var testResults []string
	for _, line := range lines {
		// Look for test output patterns in stream format
		// Stream format shows: ✔ TestPass, ✘ TestFail, ⊘ TestSkip
		if strings.Contains(line, "Test") && (strings.Contains(line, "✔") || strings.Contains(line, "✘") || strings.Contains(line, "⊘")) {
			testResults = append(testResults, strings.TrimSpace(line))
		}
	}

	t.Logf("Found %d test result lines", len(testResults))
	for _, result := range testResults {
		t.Logf("  %s", result)
	}

	// Count the different types of tests shown
	var passedCount, failedCount, skippedCount int
	for _, result := range testResults {
		if strings.Contains(result, "✔") {
			passedCount++
			t.Logf("PASSED test shown (BUG!): %s", result)
		}
		if strings.Contains(result, "✘") {
			failedCount++
			t.Logf("FAILED test shown (expected): %s", result)
		}
		if strings.Contains(result, "⊘") {
			skippedCount++
			t.Logf("SKIPPED test shown (expected): %s", result)
		}
	}

	t.Logf("Summary: Passed=%d, Failed=%d, Skipped=%d", passedCount, failedCount, skippedCount)

	// With show: failed, we should see ONLY failed and skipped tests, NOT passed tests
	assert.Equal(t, 0, passedCount, "No passed tests should be shown with 'show: failed' filter, but found %d", passedCount)
	assert.Greater(t, failedCount, 0, "Failed tests should be shown with 'show: failed' filter")
	assert.Greater(t, skippedCount, 0, "Skipped tests should be shown with 'show: failed' filter")

	// Also verify that test-results.json was created (from the config)
	resultsFile := filepath.Join(tempDir, "test-results.json")
	_, err = os.Stat(resultsFile)
	assert.NoError(t, err, "test-results.json should be created as specified in config")

	// Check if coverage.out was created
	coverageFile := filepath.Join(tempDir, "coverage.out")
	_, err = os.Stat(coverageFile)
	// Coverage file might not exist if tests don't have coverage, so we don't assert on this
	if err == nil {
		t.Logf("Coverage file was created: %s", coverageFile)
	}
}

// TestShowFailedWithStreamSubcommand tests the same scenario but explicitly using the stream subcommand
func TestShowFailedWithStreamSubcommand(t *testing.T) {
	// Create a temporary directory for our test
	tempDir := t.TempDir()

	// Create a simple test file
	testFile := filepath.Join(tempDir, "simple_test.go")
	testContent := `package main

import "testing"

func TestPassA(t *testing.T) {}
func TestPassB(t *testing.T) {}
func TestFailA(t *testing.T) { t.Fatal("fail") }
func TestSkipA(t *testing.T) { t.Skip("skip") }
`
	err := os.WriteFile(testFile, []byte(testContent), 0644)
	require.NoError(t, err)

	// Create go.mod
	goModFile := filepath.Join(tempDir, "go.mod")
	goModContent := `module testpkg
go 1.21
`
	err = os.WriteFile(goModFile, []byte(goModContent), 0644)
	require.NoError(t, err)

	// Create the same .gotcha.yaml config
	configFile := filepath.Join(tempDir, ".gotcha.yaml")
	configContent := `format: stream
packages:
  - "."
show: failed
output: test-results.json
`
	err = os.WriteFile(configFile, []byte(configContent), 0644)
	require.NoError(t, err)

	// Build the gotcha binary
	gotchaBinary := filepath.Join(tempDir, "gotcha-test-binary")
	gotchaDir, err := filepath.Abs("..")
	require.NoError(t, err)
	
	buildCmd := exec.Command("go", "build", "-o", gotchaBinary, ".")
	buildCmd.Dir = gotchaDir
	buildOut, buildErr := buildCmd.CombinedOutput()
	if buildErr != nil {
		t.Fatalf("Failed to build: %v\n%s", buildErr, buildOut)
	}

	// Test 1: Run with explicit stream subcommand
	t.Run("explicit_stream_subcommand", func(t *testing.T) {
		cmd := exec.Command(gotchaBinary, "stream", ".")
		cmd.Dir = tempDir
		
		var output bytes.Buffer
		cmd.Stdout = &output
		cmd.Stderr = &output

		_ = cmd.Run()
		
		outputStr := output.String()
		t.Logf("Output with 'gotcha stream .':\n%s", outputStr)

		passCount := strings.Count(outputStr, "✔")
		assert.Equal(t, 0, passCount, "No passed tests should be shown with stream subcommand and 'show: failed'")
	})

	// Test 2: Run without subcommand (root command)
	t.Run("root_command", func(t *testing.T) {
		cmd := exec.Command(gotchaBinary, ".")
		cmd.Dir = tempDir
		
		var output bytes.Buffer
		cmd.Stdout = &output
		cmd.Stderr = &output

		_ = cmd.Run()
		
		outputStr := output.String()
		t.Logf("Output with 'gotcha .':\n%s", outputStr)

		passCount := strings.Count(outputStr, "✔")
		assert.Equal(t, 0, passCount, "No passed tests should be shown with root command and 'show: failed'")
	})
}