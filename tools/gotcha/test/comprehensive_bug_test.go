//go:build gotcha_binary_integration
// +build gotcha_binary_integration

// DEPRECATED: This test builds the gotcha binary. Use testdata approach instead.
// See parser_integration_test.go for the recommended pattern.
package test

import (
	"github.com/cloudposse/atmos/tools/gotcha/pkg/constants"
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestShowFailedFilterBug_Comprehensive tests all aspects of the show:failed bug
func TestShowFailedFilterBug_Comprehensive(t *testing.T) {
	// Create a temporary directory for our test
	tempDir := t.TempDir()

	// Create test files covering all scenarios
	testFile1 := filepath.Join(tempDir, "suite1_test.go")
	testContent1 := `package main

import (
	"github.com/cloudposse/atmos/tools/gotcha/pkg/constants"
	"testing"
	"time"
)

func TestSuite1_Pass1(t *testing.T) {
	// Passing test
}

func TestSuite1_Pass2(t *testing.T) {
	// Another passing test
	time.Sleep(5 * time.Millisecond)
}

func TestSuite1_Pass3(t *testing.T) {
	// Yet another passing test
}

func TestSuite1_Fail1(t *testing.T) {
	t.Fatal("This test fails")
}

func TestSuite1_Skip1(t *testing.T) {
	t.Skip("This test is skipped")
}
`
	err := os.WriteFile(testFile1, []byte(testContent1),constants.DefaultFilePerms)
	require.NoError(t, err)

	testFile2 := filepath.Join(tempDir, "suite2_test.go")
	testContent2 := `package main

import "testing"

func TestSuite2_Pass1(t *testing.T) {}
func TestSuite2_Pass2(t *testing.T) {}
func TestSuite2_Pass3(t *testing.T) {}
func TestSuite2_Pass4(t *testing.T) {}
func TestSuite2_Pass5(t *testing.T) {}
func TestSuite2_Fail1(t *testing.T) { t.Error("fail") }
func TestSuite2_Fail2(t *testing.T) { t.Fatal("fail") }
func TestSuite2_Skip1(t *testing.T) { t.Skip("skip") }
`
	err = os.WriteFile(testFile2, []byte(testContent2),constants.DefaultFilePerms)
	require.NoError(t, err)

	// Create go.mod
	goModFile := filepath.Join(tempDir, "go.mod")
	goModContent := `module testpkg
go 1.21
`
	err = os.WriteFile(goModFile, []byte(goModContent),constants.DefaultFilePerms)
	require.NoError(t, err)

	// Create .gotcha.yaml with exact user configuration
	configFile := filepath.Join(tempDir, ".gotcha.yaml")
	configContent := `# Gotcha Configuration File
format: stream
packages:
  - "./..."
testargs: "-timeout 40m"
show: failed
output: test-results.json
coverprofile: coverage.out
exclude-mocks: true
alert: false
filter:
  include:
    - ".*"
  exclude: []
`
	err = os.WriteFile(configFile, []byte(configContent),constants.DefaultFilePerms)
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

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	_ = cmd.Run() // Ignore error from failed tests

	output := stderr.String() + stdout.String()

	t.Run("show_failed_filter", func(t *testing.T) {
		t.Logf("Full output:\n%s", output)

		// Count individual test displays
		lines := strings.Split(output, "\n")
		var displayedTests []string
		for _, line := range lines {
			// Look for individual test results (not summary)
			if strings.Contains(line, "TestSuite") {
				if strings.Contains(line, "✔") {
					displayedTests = append(displayedTests, "PASS: "+line)
				} else if strings.Contains(line, "✘") {
					displayedTests = append(displayedTests, "FAIL: "+line)
				} else if strings.Contains(line, "⊘") {
					displayedTests = append(displayedTests, "SKIP: "+line)
				}
			}
		}

		t.Logf("Individual tests displayed: %d", len(displayedTests))
		for _, test := range displayedTests {
			t.Logf("  %s", test)
		}

		// Count by type
		var passCount, failCount, skipCount int
		for _, test := range displayedTests {
			if strings.HasPrefix(test, "PASS:") {
				passCount++
			} else if strings.HasPrefix(test, "FAIL:") {
				failCount++
			} else if strings.HasPrefix(test, "SKIP:") {
				skipCount++
			}
		}

		// BUG CHECK: With show:failed, NO passing tests should be displayed individually
		if passCount > 0 {
			t.Errorf("BUG: %d passing tests shown individually with 'show: failed' filter", passCount)
		}

		// Failed and skipped tests SHOULD be shown
		assert.Equal(t, 3, failCount, "All 3 failed tests should be shown")
		assert.Equal(t, 2, skipCount, "All 2 skipped tests should be shown")

		// Check for the "All X tests passed" summary line (should not appear with show:failed)
		if strings.Contains(output, "All") && strings.Contains(output, "tests passed") {
			t.Error("BUG: Shows 'All X tests passed' summary with 'show: failed' filter")
		}

		// Check if it shows "X tests failed, Y passed" which is acceptable
		if strings.Contains(output, "tests failed") && strings.Contains(output, "passed") {
			t.Log("Shows 'X tests failed, Y passed' summary - this is acceptable")
		}
	})

	t.Run("test_results_json", func(t *testing.T) {
		// Check that test-results.json was created and contains ALL tests
		resultsFile := filepath.Join(tempDir, "test-results.json")
		data, err := os.ReadFile(resultsFile)
		if err != nil {
			t.Errorf("test-results.json not created: %v", err)
			return
		}

		var results map[string]interface{}
		err = json.Unmarshal(data, &results)
		require.NoError(t, err, "test-results.json should be valid JSON")

		// The JSON should contain ALL tests, not just filtered ones
		resultsStr := string(data)
		assert.Contains(t, resultsStr, "TestSuite1_Pass1", "JSON should contain all tests including passed")
		assert.Contains(t, resultsStr, "TestSuite1_Fail1", "JSON should contain failed tests")
		assert.Contains(t, resultsStr, "TestSuite1_Skip1", "JSON should contain skipped tests")

		t.Logf("test-results.json contains %d bytes of data", len(data))
	})

	t.Run("cache_yaml", func(t *testing.T) {
		// Check if cache.yaml was created/updated
		cacheDir := filepath.Join(tempDir, ".gotcha")
		cacheFile := filepath.Join(cacheDir, "cache.yaml")

		if _, err := os.Stat(cacheFile); os.IsNotExist(err) {
			t.Error("BUG: cache.yaml was not created")
			return
		}

		data, err := os.ReadFile(cacheFile)
		require.NoError(t, err)

		var cache map[string]interface{}
		err = yaml.Unmarshal(data, &cache)
		require.NoError(t, err, "cache.yaml should be valid YAML")

		// Check if it contains test discovery information
		if discovery, ok := cache["discovery"].(map[string]interface{}); ok {
			if testCounts, ok := discovery["test_counts"].(map[string]interface{}); ok {
				t.Logf("cache.yaml contains test_counts: %v", testCounts)

				// Should have cached the test count for current pattern
				if pkgInfo, ok := testCounts["."].(map[string]interface{}); ok {
					if count, ok := pkgInfo["count"].(int); ok {
						assert.Equal(t, 13, count, "Should cache all 13 tests regardless of filter")
					}
				}
			} else {
				t.Error("cache.yaml missing test_counts section")
			}
		} else {
			t.Error("cache.yaml missing discovery section")
		}

		t.Logf("cache.yaml contents:\n%s", string(data))
	})

	t.Run("coverage_file", func(t *testing.T) {
		// Check if coverage.out was created
		coverageFile := filepath.Join(tempDir, "coverage.out")
		if _, err := os.Stat(coverageFile); err == nil {
			data, _ := os.ReadFile(coverageFile)
			t.Logf("coverage.out created with %d bytes", len(data))
		} else {
			t.Log("coverage.out not created (may be expected if no coverage)")
		}
	})
}

// TestAllPassingWithShowFailed specifically tests the case where all tests pass
func TestAllPassingWithShowFailed(t *testing.T) {
	tempDir := t.TempDir()

	// Create test file with ONLY passing tests
	testFile := filepath.Join(tempDir, "allpass_test.go")
	testContent := `package main
import "testing"
func TestA(t *testing.T) {}
func TestB(t *testing.T) {}
func TestC(t *testing.T) {}
func TestD(t *testing.T) {}
func TestE(t *testing.T) {}
`
	err := os.WriteFile(testFile, []byte(testContent),constants.DefaultFilePerms)
	require.NoError(t, err)

	// Create go.mod
	goModFile := filepath.Join(tempDir, "go.mod")
	err = os.WriteFile(goModFile, []byte("module testpkg\ngo 1.21\n"),constants.DefaultFilePerms)
	require.NoError(t, err)

	// Create .gotcha.yaml with show: failed
	configFile := filepath.Join(tempDir, ".gotcha.yaml")
	configContent := `format: stream
show: failed
packages: ["."]
`
	err = os.WriteFile(configFile, []byte(configContent),constants.DefaultFilePerms)
	require.NoError(t, err)

	// Build and run gotcha
	gotchaBinary := filepath.Join(tempDir, "gotcha-test")
	gotchaDir, _ := filepath.Abs("..")

	buildCmd := CreateGotchaCommand("go", "build", "-o", gotchaBinary, ".")
	buildCmd.Dir = gotchaDir
	buildOut, buildErr := buildCmd.CombinedOutput()
	require.NoError(t, buildErr, "Build failed: %s", buildOut)

	cmd := CreateGotchaCommand(gotchaBinary, ".")
	cmd.Dir = tempDir

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	err = cmd.Run()
	require.NoError(t, err, "Should succeed when all tests pass")

	outputStr := output.String()
	t.Logf("Output with all passing and show:failed:\n%s", outputStr)

	// THE BUG: With show:failed and all tests passing,
	// it should NOT show "All 5 tests passed"
	if strings.Contains(outputStr, "All") && strings.Contains(outputStr, "tests passed") {
		t.Error("BUG CONFIRMED: Shows 'All X tests passed' even with show:failed when all tests pass")
		t.Log("Expected behavior: With show:failed, when all tests pass, don't show the 'All X tests passed' line")
	}

	// Should still show the summary statistics
	assert.Contains(t, outputStr, "Test Results:", "Should show test results summary")
	assert.Contains(t, outputStr, "Passed:", "Should show passed count in summary")
}
