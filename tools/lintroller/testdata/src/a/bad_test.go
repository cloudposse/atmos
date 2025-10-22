package testdata

import (
	"os"
	"testing"
)

// Test t.Setenv in defer (tsetenv-in-defer rule).
func TestBad(t *testing.T) {
	orig := os.Getenv("FOO")
	defer func() {
		t.Setenv("FOO", orig) // want "t.Setenv should not be called inside defer blocks; t.Setenv handles cleanup automatically. Use os.Setenv for manual restoration in defer"
	}()

	t.Setenv("FOO", "bar")
}

// Test t.Setenv in t.Cleanup (tsetenv-in-defer rule).
func TestBadCleanup(t *testing.T) {
	orig := os.Getenv("BAR")
	t.Cleanup(func() {
		t.Setenv("BAR", orig) // want "t.Setenv should not be called inside t.Cleanup; t.Setenv handles cleanup automatically. Use os.Setenv for manual restoration in cleanup functions"
	})

	t.Setenv("BAR", "baz")
}

// Test os.Setenv in test file (os-setenv-in-test rule).
func TestBadOsSetenv(t *testing.T) {
	os.Setenv("PATH", "/test/path") // want "os.Setenv should not be used in test files; use t.Setenv instead for automatic cleanup \\(os.Setenv is allowed inside defer/t.Cleanup blocks and benchmark functions for manual restoration\\)"
}

// Test os.Setenv allowed in defer.
func TestGoodOsSetenvInDefer(t *testing.T) {
	orig := os.Getenv("PATH")
	defer func() {
		os.Setenv("PATH", orig) // OK: os.Setenv is allowed in defer blocks.
	}()
}

// Test os.MkdirTemp in test file (os-mkdirtemp-in-test rule).
func TestBadMkdirTemp(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test-*") // want "os.MkdirTemp should not be used in test files; use t.TempDir instead for automatic cleanup \\(os.MkdirTemp is allowed in benchmark functions\\)"
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)
}

// Test os.MkdirTemp allowed in benchmark.
func BenchmarkGoodMkdirTemp(b *testing.B) {
	tempDir, _ := os.MkdirTemp("", "bench-*") // OK: os.MkdirTemp is allowed in benchmarks.
	defer os.RemoveAll(tempDir)
}

// Test os.Setenv allowed in benchmark.
func BenchmarkGoodOsSetenv(b *testing.B) {
	os.Setenv("PATH", "/bench/path") // OK: os.Setenv is allowed in benchmarks.
}

// Test os.Args in test file (os-args-in-test rule).
func TestBadOsArgs(t *testing.T) {
	oldArgs := os.Args // want "os.Args should not be used in test files; use cmd.SetArgs\\(\\) instead to set command arguments \\(os.Args is allowed in benchmark functions\\)"
	defer func() {
		os.Args = oldArgs // want "os.Args should not be used in test files; use cmd.SetArgs\\(\\) instead to set command arguments \\(os.Args is allowed in benchmark functions\\)"
	}()

	os.Args = []string{"test", "arg"} // want "os.Args should not be used in test files; use cmd.SetArgs\\(\\) instead to set command arguments \\(os.Args is allowed in benchmark functions\\)"
}

// Test os.Args allowed in benchmark.
func BenchmarkGoodOsArgs(b *testing.B) {
	oldArgs := os.Args // OK: os.Args is allowed in benchmarks.
	defer func() {
		os.Args = oldArgs // OK: os.Args is allowed in benchmarks.
	}()

	os.Args = []string{"bench", "arg"} // OK: os.Args is allowed in benchmarks.
}

// TestDocumentationOnly is a bad test with only logging and no assertions (test-no-assertions rule).
func TestDocumentationOnly(t *testing.T) { // want "Test function 'TestDocumentationOnly' contains only t.Log\\(\\) calls with no assertions.*"
	t.Log("This test documents the expected behavior")
	t.Logf("Some value: %s", "test")
}

// TestUnconditionalSkip is a bad test that always skips (test-no-assertions rule).
func TestUnconditionalSkip(t *testing.T) {
	t.Skipf("This test always skips") // want "Test function 'TestUnconditionalSkip' unconditionally skips.*"
}

// TestUnconditionalSkipWithTrue is a bad test with if (true) t.Skip (test-no-assertions rule).
func TestUnconditionalSkipWithTrue(t *testing.T) {
	if true {
		t.Skipf("This also always skips") // want "Test function 'TestUnconditionalSkipWithTrue' unconditionally skips.*"
	}
}
