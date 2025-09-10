package mixed_tests

import (
	"testing"
	"time"
)

func TestPass1(t *testing.T) {
	// This passes
	if 1+1 == 2 {
		// All good
	}
}

func TestPass2(t *testing.T) {
	// Another passing test
	time.Sleep(10 * time.Millisecond)
}

func TestPass3(t *testing.T) {
	// Yet another passing test
	result := 42
	if result != 42 {
		t.Errorf("Expected 42, got %d", result)
	}
}

func TestFail1(t *testing.T) {
	t.Fatal("This test fails intentionally")
}

func TestFail2(t *testing.T) {
	t.Error("This test also fails")
}

func TestSkip1(t *testing.T) {
	t.Skip("This test is skipped")
}

func TestSkip2(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}
	// Would run test here
}