package skipping_tests

import (
	"os"
	"testing"
)

func TestSkip1(t *testing.T) {
	t.Skip("This test is skipped intentionally")
}

func TestSkip2(t *testing.T) {
	t.Skip("Another skipped test")
}

func TestConditionalSkip(t *testing.T) {
	if os.Getenv("RUN_SLOW_TESTS") != "true" {
		t.Skip("Skipping slow test")
	}
	// Would run slow test here
}

func TestSkipWithReason(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping test in short mode")
	}
	// Regular test logic here
}
