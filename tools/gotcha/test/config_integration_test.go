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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConfigFile_ShowFilterIsRespected tests that the show filter from .gotcha.yaml is actually respected
// This is an integration test that runs the actual gotcha binary with a config file
func TestConfigFile_ShowFilterIsRespected(t *testing.T) {
	// Create a temporary directory for our test
	tempDir := t.TempDir()

	// Create a simple test file that has pass, fail, and skip tests
	testFile := filepath.Join(tempDir, "example_test.go")
	testContent := `package main

import (
	"testing"
)

func TestPass(t *testing.T) {
	// This test passes
}

func TestFail(t *testing.T) {
	t.Fatal("This test fails")
}

func TestSkip(t *testing.T) {
	t.Skip("This test is skipped")
}
`
	err := os.WriteFile(testFile, []byte(testContent), 0o644)
	require.NoError(t, err)

	// Create go.mod for the test package
	goModFile := filepath.Join(tempDir, "go.mod")
	goModContent := `module testpkg

go 1.21
`
	err = os.WriteFile(goModFile, []byte(goModContent), 0o644)
	require.NoError(t, err)

	// Create a .gotcha.yaml config file with show: failed
	configFile := filepath.Join(tempDir, ".gotcha.yaml")
	configContent := `# Test configuration
format: stream
show: failed
packages:
  - "."
`
	err = os.WriteFile(configFile, []byte(configContent), 0o644)
	require.NoError(t, err)

	// Build the gotcha binary if it doesn't exist
	gotchaBinary := filepath.Join("..", "gotcha-bin")
	if _, err := os.Stat(gotchaBinary); os.IsNotExist(err) {
		t.Logf("Building gotcha binary at %s", gotchaBinary)
		buildCmd := exec.Command("go", "build", "-o", gotchaBinary, "../cmd/gotcha")
		buildOut, buildErr := buildCmd.CombinedOutput()
		if buildErr != nil {
			t.Fatalf("Failed to build gotcha binary: %v\nOutput: %s", buildErr, buildOut)
		}
	}

	// Run gotcha in the temp directory
	cmd := exec.Command(gotchaBinary)
	cmd.Dir = tempDir

	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Run the command (we expect it to fail because we have a failing test)
	err = cmd.Run()
	// Don't check error - we expect non-zero exit due to test failure

	// Get the combined output
	output := stderr.String() + stdout.String()
	t.Logf("Gotcha output:\n%s", output)

	// Parse the output to see what tests were displayed
	lines := strings.Split(output, "\n")

	var sawTestPass, sawTestFail, sawTestSkip bool
	for _, line := range lines {
		// Look for test output patterns
		// Stream format shows: ✔ TestPass, ✘ TestFail, ⊘ TestSkip
		// or PASS TestPass, FAIL TestFail, SKIP TestSkip
		if strings.Contains(line, "TestPass") && (strings.Contains(line, "✔") || strings.Contains(line, "PASS")) {
			sawTestPass = true
			t.Logf("Found TestPass in output: %s", line)
		}
		if strings.Contains(line, "TestFail") && (strings.Contains(line, "✘") || strings.Contains(line, "FAIL")) {
			sawTestFail = true
			t.Logf("Found TestFail in output: %s", line)
		}
		if strings.Contains(line, "TestSkip") && (strings.Contains(line, "⊘") || strings.Contains(line, "SKIP")) {
			sawTestSkip = true
			t.Logf("Found TestSkip in output: %s", line)
		}
	}

	// With show: failed, we should see failed and skipped tests, but NOT passed tests
	assert.False(t, sawTestPass, "TestPass should NOT be shown with show: failed filter")
	assert.True(t, sawTestFail, "TestFail should be shown with show: failed filter")
	assert.True(t, sawTestSkip, "TestSkip should be shown with show: failed filter")

	// Also check that the config file was actually used
	// We can verify this by checking if there's any indication in debug output
	// or by the fact that the format is stream (which is set in config)
	assert.Contains(t, output, "✘", "Output should use stream format symbols from config")
}

// TestConfigFile_LoadedBeforeCommandExecution tests that config is loaded early enough
func TestConfigFile_LoadedBeforeCommandExecution(t *testing.T) {
	// Create a temporary directory for our test
	tempDir := t.TempDir()

	// Create a simple test file
	testFile := filepath.Join(tempDir, "simple_test.go")
	testContent := `package main

import "testing"

func TestSimple(t *testing.T) {
	t.Fatal("fail")
}
`
	err := os.WriteFile(testFile, []byte(testContent), 0o644)
	require.NoError(t, err)

	// Create go.mod
	goModFile := filepath.Join(tempDir, "go.mod")
	goModContent := `module testpkg
go 1.21
`
	err = os.WriteFile(goModFile, []byte(goModContent), 0o644)
	require.NoError(t, err)

	// Create a .gotcha.yaml with specific settings
	configFile := filepath.Join(tempDir, ".gotcha.yaml")
	configContent := `format: stream
show: failed
output: from-config.json
`
	err = os.WriteFile(configFile, []byte(configContent), 0o644)
	require.NoError(t, err)

	// Build gotcha binary
	gotchaBinary := filepath.Join("..", "gotcha-bin")
	if _, err := os.Stat(gotchaBinary); os.IsNotExist(err) {
		buildCmd := exec.Command("go", "build", "-o", gotchaBinary, "../cmd/gotcha")
		if out, err := buildCmd.CombinedOutput(); err != nil {
			t.Fatalf("Failed to build: %v\n%s", err, out)
		}
	}

	// Run with debug logging to see config loading
	cmd := exec.Command(gotchaBinary, "--log-level=debug")
	cmd.Dir = tempDir

	var output bytes.Buffer
	cmd.Stderr = &output
	cmd.Stdout = &output

	_ = cmd.Run() // Ignore error from test failure

	outputStr := output.String()
	t.Logf("Debug output:\n%s", outputStr)

	// Check if the output file from config was created
	outputFile := filepath.Join(tempDir, "from-config.json")
	_, err = os.Stat(outputFile)
	assert.NoError(t, err, "Output file from config should be created")

	// Verify show filter was applied
	assert.NotContains(t, outputStr, "✔", "Passed tests should not be shown with show: failed")
}

// TestCacheYaml_LogsAllTestsRegardlessOfFilter tests that cache.yaml logs all tests
func TestCacheYaml_LogsAllTestsRegardlessOfFilter(t *testing.T) {
	t.Skip("Cache functionality not yet implemented - will be fixed in next commit")

	// This test will verify that even with show: failed, the cache.yaml
	// contains information about ALL tests for estimation purposes
}
