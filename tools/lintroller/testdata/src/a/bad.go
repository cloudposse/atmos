package testdata

import (
	"os"
	"testing"
)

func TestBad(t *testing.T) {
	orig := os.Getenv("FOO")
	defer func() {
		t.Setenv("FOO", orig) // want "t.Setenv should not be called inside defer blocks; t.Setenv handles cleanup automatically. Use os.Setenv for manual restoration in defer"
	}()

	t.Setenv("FOO", "bar")
}

func TestBadCleanup(t *testing.T) {
	orig := os.Getenv("BAR")
	t.Cleanup(func() {
		t.Setenv("BAR", orig) // want "t.Setenv should not be called inside t.Cleanup; t.Setenv handles cleanup automatically. Use os.Setenv for manual restoration in cleanup functions"
	})

	t.Setenv("BAR", "baz")
}
