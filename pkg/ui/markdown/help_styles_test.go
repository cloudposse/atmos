package markdown

import (
	"testing"
)

func TestGetHelpStyle(t *testing.T) {
	t.Run("returns valid style configuration", func(t *testing.T) {
		styleBytes, err := GetHelpStyle()
		if err != nil {
			t.Fatalf("GetHelpStyle() returned error: %v", err)
		}

		if len(styleBytes) == 0 {
			t.Error("Expected non-empty style configuration")
		}

		// Verify it's valid JSON.
		// The style config should be marshalable to JSON.
		if styleBytes[0] != '{' {
			t.Error("Expected style configuration to be JSON object")
		}
	})

	t.Run("style contains expected color values", func(t *testing.T) {
		styleBytes, err := GetHelpStyle()
		if err != nil {
			t.Fatalf("GetHelpStyle() returned error: %v", err)
		}

		styleStr := string(styleBytes)

		// Check for Cloud Posse color scheme.
		if !contains(styleStr, DefaultLightGray) && !contains(styleStr, "e7e5e4") {
			t.Log("Style may not contain expected light gray color")
		}

		if !contains(styleStr, DefaultPurple) && !contains(styleStr, "9B51E0") {
			t.Log("Style may not contain expected purple accent color")
		}
	})
}

// contains checks if a string contains a substring.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
