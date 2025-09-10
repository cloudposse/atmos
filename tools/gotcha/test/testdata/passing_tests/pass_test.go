package passing_tests

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
	result := "hello"
	if result != "hello" {
		t.Errorf("Expected 'hello', got '%s'", result)
	}
}

func TestPass3(t *testing.T) {
	// Yet another passing test with a small delay
	time.Sleep(10 * time.Millisecond)
	// Success!
}

func TestPassWithSubtests(t *testing.T) {
	t.Run("Subtest1", func(t *testing.T) {
		// Passing subtest
	})
	t.Run("Subtest2", func(t *testing.T) {
		// Another passing subtest
	})
}
