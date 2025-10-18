package testdata

import (
	"testing"
)

// TestWithError is a good test using t.Error.
func TestWithError(t *testing.T) {
	t.Log("Checking something")
	if 1+1 != 2 {
		t.Error("Math is broken")
	}
}

// TestWithFatal is a good test using t.Fatal.
func TestWithFatal(t *testing.T) {
	t.Log("Critical test")
	if false {
		t.Fatal("This would fail")
	}
}

// TestWithConditionalSkip is a good test that conditionally skips based on environment.
func TestWithConditionalSkip(t *testing.T) {
	condition := false // This could be runtime.GOOS, env var, etc.
	if condition {
		t.Skipf("Skipping due to condition")
	}
	// Test continues if not skipped.
	if 1+1 != 2 {
		t.Error("Math broken")
	}
}
