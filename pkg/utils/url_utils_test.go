package utils

import (
	"os"
	"testing"
)

// TestOpenUrl_SkipsWhenGO_TESTIsSet ensures that setting GO_TEST=1 triggers early return without attempting to spawn a browser.
func TestOpenUrl_SkipsWhenGO_TESTIsSet(t *testing.T) {
	old := os.Getenv("GO_TEST")
	_ = os.Setenv("GO_TEST", "1")
	defer os.Setenv("GO_TEST", old)

	if err := OpenUrl("https://example.com"); err \!= nil {
		t.Fatalf("expected nil error in test mode (GO_TEST=1), got %v", err)
	}
}