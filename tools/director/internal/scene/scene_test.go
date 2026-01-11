package scene

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCalculateTapeDuration(t *testing.T) {
	// Create a temporary tape file.
	tmpDir := t.TempDir()
	tapePath := filepath.Join(tmpDir, "test.tape")

	tapeContent := `# Test tape
Output test.gif

Sleep 2s
Type "hello"
Sleep 500ms
Enter
Sleep 3s
Type "world"
Sleep 1.5s
`
	if err := os.WriteFile(tapePath, []byte(tapeContent), 0o644); err != nil {
		t.Fatalf("failed to write test tape: %v", err)
	}

	duration, err := CalculateTapeDuration(tapePath)
	if err != nil {
		t.Fatalf("CalculateTapeDuration failed: %v", err)
	}

	// Expected: 2s + 0.5s + 3s + 1.5s = 7s.
	expected := 7.0
	if duration != expected {
		t.Errorf("expected duration %.1f, got %.1f", expected, duration)
	}
}

func TestCalculateTapeDuration_VendorPull(t *testing.T) {
	// Test with actual vendor pull tape file if it exists.
	tapePath := "../../../../demos/scenes/vendor/pull.tape"
	if _, err := os.Stat(tapePath); os.IsNotExist(err) {
		t.Skip("vendor pull tape not found")
	}

	duration, err := CalculateTapeDuration(tapePath)
	if err != nil {
		t.Fatalf("CalculateTapeDuration failed: %v", err)
	}

	t.Logf("Vendor pull tape duration: %.1f seconds", duration)

	// Should be at least 20 seconds based on the tape content.
	if duration < 20 {
		t.Errorf("expected duration >= 20s, got %.1f", duration)
	}
}
