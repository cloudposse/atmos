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

// TestGotchaHandlesTestMainFailure tests that gotcha properly handles TestMain failures.
func TestGotchaHandlesTestMainFailure(t *testing.T) {
	// Build gotcha binary for testing.
	gotchaBinary := buildGotcha(t)

	// Create a test package that has TestMain calling log.Fatal instead of os.Exit.
	testPkg := createTestPackageWithTestMainFailure(t)
	defer os.RemoveAll(testPkg)

	// Run gotcha on the test package.
	outputFile := filepath.Join(t.TempDir(), "test-output.json")
	cmd := CreateGotchaCommand(gotchaBinary, "stream", "--format=json", "--output="+outputFile, "./...")
	cmd.Dir = testPkg // Set working directory to the test package directory

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Run should fail.
	err := cmd.Run()

	// Debug: Print output to understand what's happening.
	t.Logf("Stdout output:\n%s", stdout.String())
	t.Logf("Stderr output:\n%s", stderr.String())
	t.Logf("Command error: %v", err)

	require.Error(t, err, "gotcha should exit with non-zero when TestMain fails")

	// Check stderr output for better error message.
	stderrOutput := stderr.String()

	// Current behavior: gotcha detects the issue and reports it.
	// Check that gotcha provides a meaningful error message.
	if strings.Contains(stderrOutput, "tests failed with exit code 1") &&
		!strings.Contains(stderrOutput, "possible build/compilation issue") &&
		!strings.Contains(stderrOutput, "TestMain") {
		t.Error("Error message is not descriptive enough - just says 'tests failed with exit code 1'")
	}

	// Check that gotcha attempts to explain the issue.
	// We now show an enhanced error message
	assert.Contains(t, stderrOutput, "Test process failed with exit code 1 (no test failures detected", "Should detect when tests pass but exit code is non-zero")
}

// TestGotchaHandlesInitPanic tests that gotcha properly handles init() panics.
func TestGotchaHandlesInitPanic(t *testing.T) {
	// Build gotcha binary for testing.
	gotchaBinary := buildGotcha(t)

	// Create a test package with an init() that panics.
	testPkg := createTestPackageWithInitPanic(t)
	defer os.RemoveAll(testPkg)

	// Run gotcha on the test package.
	outputFile := filepath.Join(t.TempDir(), "test-output.json")
	cmd := CreateGotchaCommand(gotchaBinary, "stream", "--format=json", "--output="+outputFile, "./...")
	cmd.Dir = testPkg // Set working directory to the test package directory
	// Clear GITHUB_STEP_SUMMARY to prevent test output from polluting CI summary
	cmd.Env = append(os.Environ(), "GITHUB_STEP_SUMMARY=")

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	// Run should fail.
	err := cmd.Run()
	require.Error(t, err, "gotcha should exit with non-zero when init panics")

	// Check stderr output.
	stderrOutput := stderr.String()

	// Should capture panic information.
	assert.Contains(t, stderrOutput, "panic", "Should capture panic output")

	// Should NOT just say "tests failed with exit code 1".
	assert.NotContains(t, stderrOutput, "tests failed with exit code 1", "Error message should be more descriptive")
}

// TestGotchaHandlesBuildError tests that gotcha properly handles build errors.
func TestGotchaHandlesBuildError(t *testing.T) {
	// Build gotcha binary for testing.
	gotchaBinary := buildGotcha(t)

	// Create a test package with a compilation error.
	testPkg := createTestPackageWithBuildError(t)
	defer os.RemoveAll(testPkg)

	// Run gotcha on the test package.
	outputFile := filepath.Join(t.TempDir(), "test-output.json")
	cmd := CreateGotchaCommand(gotchaBinary, "stream", "--format=json", "--output="+outputFile, "./...")
	cmd.Dir = testPkg // Set working directory to the test package directory
	// Clear GITHUB_STEP_SUMMARY to prevent test output from polluting CI summary
	cmd.Env = append(os.Environ(), "GITHUB_STEP_SUMMARY=")

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	// Run should fail.
	err := cmd.Run()
	require.Error(t, err, "gotcha should exit with non-zero when build fails")

	// Check stderr output.
	stderrOutput := stderr.String()

	// Should capture build error - look for either "build failed" or our error message
	buildErrorDetected := strings.Contains(stderrOutput, "build failed") ||
		strings.Contains(stderrOutput, "Test process failed with exit code")
	assert.True(t, buildErrorDetected, "Should detect build error")
}

// TestGotchaDistinguishesFailureTypes tests that gotcha can distinguish between test and process failures.
func TestGotchaDistinguishesFailureTypes(t *testing.T) {
	// Build gotcha binary for testing.
	gotchaBinary := buildGotcha(t)

	t.Run("actual test failure", func(t *testing.T) {
		// Create a package with a failing test.
		testPkg := createTestPackageWithFailingTest(t)
		defer os.RemoveAll(testPkg)

		// Run gotcha on the test package.
		outputFile := filepath.Join(t.TempDir(), "test-output.json")
		cmd := CreateGotchaCommand(gotchaBinary, "stream", "--format=json", "--output="+outputFile, "./...")
		cmd.Dir = testPkg // Set working directory to the test package directory

		var stderr bytes.Buffer
		cmd.Stderr = &stderr

		// Run should fail.
		err := cmd.Run()
		require.Error(t, err)

		// Check stderr output.
		stderrOutput := stderr.String()

		// Should indicate test failure.
		assert.Contains(t, stderrOutput, "1 test(s) failed", "Should report test failure correctly")
	})

	t.Run("process failure with no test failures", func(t *testing.T) {
		// Create a package with TestMain that doesn't call os.Exit.
		testPkg := createTestPackageWithTestMainNoExit(t)
		defer os.RemoveAll(testPkg)

		// Run gotcha on the test package.
		outputFile := filepath.Join(t.TempDir(), "test-output.json")
		cmd := CreateGotchaCommand(gotchaBinary, "stream", "--format=json", "--output="+outputFile, "./...")
		cmd.Dir = testPkg // Set working directory to the test package directory

		var stderr bytes.Buffer
		cmd.Stderr = &stderr

		// Run should fail.
		err := cmd.Run()
		require.Error(t, err)

		// Check stderr output.
		stderrOutput := stderr.String()

		// Should NOT report test failure when tests actually passed.
		assert.NotContains(t, stderrOutput, "test(s) failed", "Should not report test failure when tests passed")

		// Should indicate process issue (our enhanced error doesn't always mention TestMain specifically)
		assert.Contains(t, stderrOutput, "Test process failed with exit code", "Should mention process failure")
	})
}

// Helper functions to create test packages with various failure modes.

func buildGotcha(t *testing.T) string {
	// Build gotcha binary in temp directory.
	tmpDir := t.TempDir()
	gotchaBinary := filepath.Join(tmpDir, "gotcha")

	// Build gotcha from its root module (where main.go is).
	cmd := exec.Command("go", "build", "-o", gotchaBinary, "github.com/cloudposse/atmos/tools/gotcha")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to build gotcha: %v\nOutput: %s", err, output)
	}

	// Verify it's an executable.
	info, err := os.Stat(gotchaBinary)
	if err != nil {
		t.Fatalf("Failed to stat gotcha binary: %v", err)
	}
	t.Logf("Built gotcha binary: %s (size: %d bytes)", gotchaBinary, info.Size())

	return gotchaBinary
}

func createTestPackageWithTestMainFailure(t *testing.T) string {
	// Create temp directory outside the project tree to avoid it being picked up by ./...
	dir, err := os.MkdirTemp("", "gotcha-test-*")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })

	// Create go.mod.
	modContent := `module testpkg
go 1.21
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(modContent), 0o644))

	// Create test file with TestMain that calls log.Fatal.
	testContent := `package testpkg

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	code := m.Run()
	// Intentionally exit with 1 even though tests might pass
	// This simulates a TestMain that has additional checks that fail
	if code == 0 {
		os.Exit(1) // Force exit 1 even when tests pass
	}
	os.Exit(code)
}

func TestPass(t *testing.T) {
	// This test passes
}
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main_test.go"), []byte(testContent), 0o644))

	return dir
}

func createTestPackageWithInitPanic(t *testing.T) string {
	// Create temp directory outside the project tree to avoid it being picked up by ./...
	dir, err := os.MkdirTemp("", "gotcha-test-*")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })

	// Create go.mod.
	modContent := `module testpkg
go 1.21
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(modContent), 0o644))

	// Create test file with init that panics.
	testContent := `package testpkg

import "testing"

func init() {
	panic("initialization failed")
}

func TestNeverRuns(t *testing.T) {
	t.Log("This test never runs due to init panic")
}
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "init_test.go"), []byte(testContent), 0o644))

	return dir
}

func createTestPackageWithBuildError(t *testing.T) string {
	// Create temp directory outside the project tree to avoid it being picked up by ./...
	dir, err := os.MkdirTemp("", "gotcha-test-*")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })

	// Create go.mod.
	modContent := `module testpkg
go 1.21
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(modContent), 0o644))

	// Create test file with compilation error.
	testContent := `package testpkg

import "testing"

func TestWithBuildError(t *testing.T) {
	// This references an undefined variable
	_ = undefinedVariable
}
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "build_test.go"), []byte(testContent), 0o644))

	return dir
}

func createTestPackageWithFailingTest(t *testing.T) string {
	// Create temp directory outside the project tree to avoid it being picked up by ./...
	dir, err := os.MkdirTemp("", "gotcha-test-*")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })

	// Create go.mod.
	modContent := `module testpkg
go 1.21
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(modContent), 0o644))

	// Create test file with failing test.
	testContent := `package testpkg

import "testing"

func TestFail(t *testing.T) {
	t.Fatal("This test fails")
}
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "fail_test.go"), []byte(testContent), 0o644))

	return dir
}

func createTestPackageWithTestMainNoExit(t *testing.T) string {
	// Create temp directory outside the project tree to avoid it being picked up by ./...
	dir, err := os.MkdirTemp("", "gotcha-test-*")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })

	// Create go.mod.
	modContent := `module testpkg
go 1.21
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(modContent), 0o644))

	// Create test file with TestMain that exits with 1 even though tests pass.
	testContent := `package testpkg

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	code := m.Run()
	// Always exit with 1 to simulate TestMain detecting an issue
	// even though the tests themselves pass
	_ = code
	os.Exit(1)
}

func TestPass(t *testing.T) {
	// This test passes
}
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "noexit_test.go"), []byte(testContent), 0o644))

	return dir
}
