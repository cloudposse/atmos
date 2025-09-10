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

// TestShowFailedFilter_IndividualTestDisplay verifies that the show:failed filter
// correctly hides individual passing test lines while still showing summaries
func TestShowFailedFilter_IndividualTestDisplay(t *testing.T) {
	tempDir := t.TempDir()

	// Create test files with mixed results
	testFile := filepath.Join(tempDir, "mixed_test.go")
	testContent := `package main

import (
	"testing"
)

func TestPass1(t *testing.T) {
	// This passes
}

func TestPass2(t *testing.T) {
	// This also passes
}

func TestPass3(t *testing.T) {
	// Another passing test
}

func TestFail1(t *testing.T) {
	t.Fatal("This test fails")
}

func TestFail2(t *testing.T) {
	t.Error("This test also fails")
}

func TestSkip1(t *testing.T) {
	t.Skip("This test is skipped")
}
`
	err := os.WriteFile(testFile, []byte(testContent), 0o644)
	require.NoError(t, err)

	// Create go.mod
	goModFile := filepath.Join(tempDir, "go.mod")
	err = os.WriteFile(goModFile, []byte("module testpkg\ngo 1.21\n"), 0o644)
	require.NoError(t, err)

	// Create .gotcha.yaml with show: failed
	configFile := filepath.Join(tempDir, ".gotcha.yaml")
	configContent := `format: stream
show: failed
packages:
  - "."
`
	err = os.WriteFile(configFile, []byte(configContent), 0o644)
	require.NoError(t, err)

	// Build gotcha
	gotchaBinary := filepath.Join(tempDir, "gotcha-test")
	gotchaDir, _ := filepath.Abs("..")

	buildCmd := exec.Command("go", "build", "-o", gotchaBinary, ".")
	buildCmd.Dir = gotchaDir
	buildOut, buildErr := buildCmd.CombinedOutput()
	require.NoError(t, buildErr, "Build failed: %s", buildOut)

	// Run gotcha
	cmd := exec.Command(gotchaBinary, ".")
	cmd.Dir = tempDir

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	_ = cmd.Run() // Ignore error from failed tests

	outputStr := output.String()
	t.Logf("Full output:\n%s", outputStr)

	// Parse individual test lines (not summaries)
	lines := strings.Split(outputStr, "\n")
	var individualPassingTests []string
	var individualFailingTests []string
	var individualSkippedTests []string

	for _, line := range lines {
		// Individual test lines have the test name and status symbol
		// We're looking for lines like "✔ TestPass1" or "✘ TestFail1"
		if strings.Contains(line, "✔") && strings.Contains(line, "Test") {
			// Check if this is an individual test line, not a summary
			if strings.Contains(line, "TestPass") || strings.Contains(line, "TestFail") || strings.Contains(line, "TestSkip") {
				individualPassingTests = append(individualPassingTests, strings.TrimSpace(line))
			}
		}
		if strings.Contains(line, "✘") && strings.Contains(line, "Test") {
			if strings.Contains(line, "TestPass") || strings.Contains(line, "TestFail") || strings.Contains(line, "TestSkip") {
				individualFailingTests = append(individualFailingTests, strings.TrimSpace(line))
			}
		}
		if strings.Contains(line, "⊘") && strings.Contains(line, "Test") {
			if strings.Contains(line, "TestPass") || strings.Contains(line, "TestFail") || strings.Contains(line, "TestSkip") {
				individualSkippedTests = append(individualSkippedTests, strings.TrimSpace(line))
			}
		}
	}

	t.Logf("Individual passing test lines found: %d", len(individualPassingTests))
	for _, test := range individualPassingTests {
		t.Logf("  BUG - Should not display: %s", test)
	}

	t.Logf("Individual failing test lines found: %d", len(individualFailingTests))
	for _, test := range individualFailingTests {
		t.Logf("  Correctly displayed: %s", test)
	}

	t.Logf("Individual skipped test lines found: %d", len(individualSkippedTests))
	for _, test := range individualSkippedTests {
		t.Logf("  Correctly displayed: %s", test)
	}

	// THE BUG: With show:failed, individual passing test lines should NOT be displayed
	assert.Equal(t, 0, len(individualPassingTests),
		"BUG: Individual passing test lines (✔ TestPassX) should NOT be shown with show:failed filter")

	// Failed and skipped individual test lines SHOULD be displayed
	assert.Equal(t, 2, len(individualFailingTests),
		"Individual failing test lines should be shown with show:failed")
	assert.Equal(t, 1, len(individualSkippedTests),
		"Individual skipped test lines should be shown with show:failed")

	// Verify that summary lines ARE shown (this is correct behavior)
	assert.Contains(t, outputStr, "tests failed",
		"Summary line 'X tests failed, Y passed' should be shown regardless of filter")
	assert.Contains(t, outputStr, "Test Results:",
		"Test Results summary should be shown regardless of filter")
	assert.Contains(t, outputStr, "Passed:",
		"Passed count in summary should be shown regardless of filter")
	assert.Contains(t, outputStr, "Failed:",
		"Failed count in summary should be shown regardless of filter")
}

// TestShowAllFilter_DisplaysEverything verifies that show:all displays all individual tests
func TestShowAllFilter_DisplaysEverything(t *testing.T) {
	tempDir := t.TempDir()

	// Create test file
	testFile := filepath.Join(tempDir, "all_test.go")
	testContent := `package main

import "testing"

func TestPass1(t *testing.T) {}
func TestPass2(t *testing.T) {}
func TestFail1(t *testing.T) { t.Fatal("fail") }
func TestSkip1(t *testing.T) { t.Skip("skip") }
`
	err := os.WriteFile(testFile, []byte(testContent), 0o644)
	require.NoError(t, err)

	// Create go.mod
	goModFile := filepath.Join(tempDir, "go.mod")
	err = os.WriteFile(goModFile, []byte("module testpkg\ngo 1.21\n"), 0o644)
	require.NoError(t, err)

	// Create .gotcha.yaml with show: all
	configFile := filepath.Join(tempDir, ".gotcha.yaml")
	configContent := `format: stream
show: all
packages:
  - "."
`
	err = os.WriteFile(configFile, []byte(configContent), 0o644)
	require.NoError(t, err)

	// Build and run gotcha
	gotchaBinary := filepath.Join(tempDir, "gotcha-test")
	gotchaDir, _ := filepath.Abs("..")

	buildCmd := exec.Command("go", "build", "-o", gotchaBinary, ".")
	buildCmd.Dir = gotchaDir
	buildOut, buildErr := buildCmd.CombinedOutput()
	require.NoError(t, buildErr, "Build failed: %s", buildOut)

	cmd := exec.Command(gotchaBinary, ".")
	cmd.Dir = tempDir

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	_ = cmd.Run()

	outputStr := output.String()
	t.Logf("Output with show:all:\n%s", outputStr)

	// With show:all, ALL individual test lines should be displayed
	assert.Contains(t, outputStr, "✔", "Should show passing tests with show:all")
	assert.Contains(t, outputStr, "✘", "Should show failing tests with show:all")
	assert.Contains(t, outputStr, "⊘", "Should show skipped tests with show:all")

	// Count individual test displays
	passCount := 0
	failCount := 0
	skipCount := 0

	lines := strings.Split(outputStr, "\n")
	for _, line := range lines {
		if strings.Contains(line, "✔") && strings.Contains(line, "TestPass") {
			passCount++
		}
		if strings.Contains(line, "✘") && strings.Contains(line, "TestFail") {
			failCount++
		}
		if strings.Contains(line, "⊘") && strings.Contains(line, "TestSkip") {
			skipCount++
		}
	}

	assert.Equal(t, 2, passCount, "Should show 2 individual passing tests with show:all")
	assert.Equal(t, 1, failCount, "Should show 1 individual failing test with show:all")
	assert.Equal(t, 1, skipCount, "Should show 1 individual skipped test with show:all")
}
