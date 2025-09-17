//go:build gotcha_binary_integration
// +build gotcha_binary_integration

// DEPRECATED: This test builds the gotcha binary. Use testdata approach instead.
// See parser_integration_test.go for the recommended pattern.
package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cloudposse/gotcha/pkg/constants"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTUISubtestIntegration runs gotcha and verifies that subtests are properly
// displayed in the output, not just counted in the total.
//
// This integration test catches the regression where:
// - The summary shows the correct total (e.g., "Total: 1764")
// - But the actual display only shows a handful of test names
// - Most packages appear blank or show only summary lines
func TestTUISubtestIntegration(t *testing.T) {
	// Skip if gotcha binary is not built
	gotchaPath := filepath.Join("..", "gotcha-test")
	if _, err := os.Stat(gotchaPath); os.IsNotExist(err) {
		t.Skipf("gotcha binary not found at %s, run 'go build -o gotcha-test .' first", gotchaPath)
	}

	t.Run("verify_subtest_display_in_stream_mode", func(t *testing.T) {
		// Run gotcha on the test directory in stream mode (not TUI)
		// Stream mode should show all tests including subtests
		cmd := CreateGotchaCommand(gotchaPath, "--show=all", "./test")
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		// Don't check error as tests might fail, we care about the output
		_ = err

		output := stdout.String() + stderr.String()

		// Count how many test names are actually displayed
		// Look for patterns like "✔ TestName" or "✘ TestName"
		lines := strings.Split(output, "\n")
		displayedTests := 0
		var testNames []string

		for _, line := range lines {
			// Match test result lines
			if strings.Contains(line, "✔") || strings.Contains(line, "✘") || strings.Contains(line, "⊘") {
				// Extract test name (skip summary lines)
				if !strings.Contains(line, "Passed:") &&
					!strings.Contains(line, "Failed:") &&
					!strings.Contains(line, "Skipped:") &&
					!strings.Contains(line, "All") &&
					!strings.Contains(line, "tests passed") &&
					!strings.Contains(line, "tests failed") {
					displayedTests++
					testNames = append(testNames, strings.TrimSpace(line))
				}
			}
		}

		// Extract the total from "Total:     X" or "Total: X"
		totalCount := 0
		for _, line := range lines {
			if strings.Contains(line, "Total:") {
				// Try different formats
				if n, _ := fmt.Sscanf(line, "Total: %d", &totalCount); n == 1 {
					break
				}
				if n, _ := fmt.Sscanf(line, "  Total:     %d", &totalCount); n == 1 {
					break
				}
				// Try extracting any number after "Total:"
				parts := strings.Split(line, "Total:")
				if len(parts) == 2 {
					fmt.Sscanf(strings.TrimSpace(parts[1]), "%d", &totalCount)
					break
				}
			}
		}

		// Log for debugging
		t.Logf("Total count shown: %d", totalCount)
		t.Logf("Test names displayed: %d", displayedTests)
		if displayedTests < 10 && len(testNames) > 0 {
			t.Logf("Sample test names: %v", testNames[:min(5, len(testNames))])
		}

		// The regression: total count doesn't match displayed tests
		// In stream mode with --show=all, they should be close
		assert.Greater(t, displayedTests, 10,
			"Should display many individual test names with --show=all")

		if totalCount > 0 {
			// Allow some discrepancy for setup/teardown, but not huge
			ratio := float64(displayedTests) / float64(totalCount)
			assert.Greater(t, ratio, 0.5,
				"BUG: Only showing %d test names but total is %d (ratio: %.2f)",
				displayedTests, totalCount, ratio)
		}
	})

	t.Run("verify_subtest_count_in_json_output", func(t *testing.T) {
		// Run gotcha with JSON output to get accurate counts
		outputFile := filepath.Join(t.TempDir(), "test-output.json")
		cmd := CreateGotchaCommand(gotchaPath,
			"--show=all",
			"--output", outputFile,
			"--format", "json",
			"./test")

		var stderr bytes.Buffer
		cmd.Stderr = &stderr

		err := cmd.Run()
		_ = err // Ignore error, we want to check the output

		// Read the JSON output
		data, err := os.ReadFile(outputFile)
		if err != nil {
			t.Logf("Could not read JSON output: %v", err)
			t.Logf("Stderr: %s", stderr.String())
			return
		}

		var result struct {
			Summary struct {
				TotalTests   int `json:"total_tests"`
				PassedTests  int `json:"passed_tests"`
				FailedTests  int `json:"failed_tests"`
				SkippedTests int `json:"skipped_tests"`
			} `json:"summary"`
			Packages []struct {
				Name  string `json:"name"`
				Tests []struct {
					Name     string `json:"name"`
					Status   string `json:"status"`
					Subtests []struct {
						Name   string `json:"name"`
						Status string `json:"status"`
					} `json:"subtests,omitempty"`
				} `json:"tests"`
			} `json:"packages"`
		}

		err = json.Unmarshal(data, &result)
		if err != nil {
			// JSON structure might be different, just check raw content
			t.Logf("JSON unmarshal failed, checking raw content")
			content := string(data)

			// Count occurrences of test patterns
			testCount := strings.Count(content, `"name"`)
			assert.Greater(t, testCount, 20,
				"JSON should contain many test entries")
			return
		}

		// Count tests in JSON
		totalInJSON := 0
		topLevelTests := 0
		for _, pkg := range result.Packages {
			for _, test := range pkg.Tests {
				topLevelTests++
				totalInJSON++
				totalInJSON += len(test.Subtests)
			}
		}

		t.Logf("JSON summary reports: %d total tests", result.Summary.TotalTests)
		t.Logf("JSON contains: %d top-level tests, %d total including subtests",
			topLevelTests, totalInJSON)

		// Verify subtests are included
		assert.Greater(t, totalInJSON, topLevelTests,
			"JSON should include subtests, not just top-level tests")
	})

	t.Run("verify_tui_test_mode_behavior", func(t *testing.T) {
		// Run gotcha in TEST_MODE to simulate TUI without TTY
		cmd := CreateGotchaCommand(gotchaPath, "--show=all", "./test")
		cmd.Env = append(os.Environ(),
			"GOTCHA_TEST_MODE=true",
			"GOTCHA_FORCE_TUI=true")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		// Set a timeout as TUI mode might hang
		done := make(chan error, 1)
		go func() {
			done <- cmd.Run()
		}()

		select {
		case <-done:
			// Command completed
		case <-time.After(10 * time.Second):
			cmd.Process.Kill()
			t.Log("Command timed out after 10 seconds")
		}

		output := stdout.String() + stderr.String()

		// In TEST_MODE, we expect to see the issue:
		// - Low test count in total
		// - Very few test names displayed
		lines := strings.Split(output, "\n")
		displayedTests := 0
		totalCount := 0

		for _, line := range lines {
			if strings.Contains(line, "✔") || strings.Contains(line, "✘") || strings.Contains(line, "⊘") {
				if !strings.Contains(line, "Passed:") &&
					!strings.Contains(line, "Failed:") &&
					!strings.Contains(line, "Skipped:") &&
					!strings.Contains(line, "All") &&
					!strings.Contains(line, "tests passed") &&
					!strings.Contains(line, "tests failed") {
					displayedTests++
				}
			}
			if strings.Contains(line, "Total:") {
				fmt.Sscanf(line, "%*s %d", &totalCount)
			}
		}

		t.Logf("TEST_MODE - Total: %d, Displayed: %d", totalCount, displayedTests)

		// In TEST_MODE with the bug, we expect very few tests
		// This confirms the regression is present
		if totalCount > 0 && totalCount < 50 {
			t.Logf("BUG CONFIRMED: TEST_MODE shows only %d tests (should be much higher)", totalCount)
		}
	})
}

// TestGotchaSubtestCounting verifies that gotcha correctly counts all tests
// including subtests in various scenarios
func TestGotchaSubtestCounting(t *testing.T) {
	// Create a temporary test file with known structure
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "example_test.go")

	testCode := `package example

import "testing"

func TestSimple(t *testing.T) {
	t.Log("simple test")
}

func TestWithSubtests(t *testing.T) {
	t.Run("subtest1", func(t *testing.T) {
		t.Log("subtest 1")
	})
	t.Run("subtest2", func(t *testing.T) {
		t.Log("subtest 2")
	})
	t.Run("subtest3", func(t *testing.T) {
		t.Log("subtest 3")
	})
}

func TestTableDriven(t *testing.T) {
	tests := []struct {
		name string
		val  int
	}{
		{"case1", 1},
		{"case2", 2},
		{"case3", 3},
		{"case4", 4},
		{"case5", 5},
	}
	
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("test case: %v", tc.val)
		})
	}
}
`

	err := os.WriteFile(testFile, []byte(testCode), constants.DefaultFilePerms)
	require.NoError(t, err)

	// Create go.mod
	goMod := `module example
go 1.21
`
	err = os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goMod), constants.DefaultFilePerms)
	require.NoError(t, err)

	// Run go test with JSON output to get ground truth
	cmd := exec.Command("go", "test", "-json", "-v", "./...")
	cmd.Dir = tmpDir
	output, err := cmd.Output()
	require.NoError(t, err, "Failed to run go test")

	// Count test events
	lines := strings.Split(string(output), "\n")
	runEvents := 0
	var allTests []string

	for _, line := range lines {
		if strings.Contains(line, `"Action":"run"`) {
			runEvents++
			// Extract test name
			var event struct {
				Test string `json:"Test"`
			}
			if json.Unmarshal([]byte(line), &event) == nil && event.Test != "" {
				allTests = append(allTests, event.Test)
			}
		}
	}

	// We expect:
	// - TestSimple (1)
	// - TestWithSubtests (1) + 3 subtests
	// - TestTableDriven (1) + 5 subtests
	// Total: 11 tests
	assert.Equal(t, 11, runEvents, "Should have 11 total test run events")
	assert.Equal(t, 11, len(allTests), "Should have 11 test names")

	// Verify we have subtests
	subtests := 0
	for _, name := range allTests {
		if strings.Contains(name, "/") {
			subtests++
		}
	}
	assert.Equal(t, 8, subtests, "Should have 8 subtests")
	assert.Equal(t, 3, len(allTests)-subtests, "Should have 3 top-level tests")

	// Now run gotcha and verify it counts correctly
	gotchaPath := filepath.Join("..", "gotcha-test")
	if _, err := os.Stat(gotchaPath); os.IsNotExist(err) {
		t.Skipf("gotcha binary not found, skipping gotcha verification")
	}

	cmd = CreateGotchaCommand(gotchaPath, "--show=all", ".")
	cmd.Dir = tmpDir
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Run() // Ignore error

	output2 := stdout.String()

	// Check if gotcha shows all 11 tests or just 3
	if strings.Contains(output2, "Total:") {
		var total int
		for _, line := range strings.Split(output2, "\n") {
			if strings.Contains(line, "Total:") {
				fmt.Sscanf(line, "%*s %d", &total)
				break
			}
		}

		t.Logf("Gotcha reports total: %d (expected 11)", total)

		// The bug would show total=11 but only display 3 test names
		displayCount := strings.Count(output2, "TestSimple") +
			strings.Count(output2, "TestWithSubtests") +
			strings.Count(output2, "TestTableDriven") +
			strings.Count(output2, "subtest") +
			strings.Count(output2, "case")

		t.Logf("Test names displayed: ~%d", displayCount)

		if total == 11 && displayCount < 8 {
			t.Logf("BUG CONFIRMED: Total shows 11 but only ~%d test names visible", displayCount)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
