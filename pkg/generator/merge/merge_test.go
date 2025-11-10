package merge

import (
	"strings"
	"testing"
)

func TestNewThreeWayMerger(t *testing.T) {
	merger := NewThreeWayMerger(25)
	if merger.thresholdPercent != 25 {
		t.Errorf("Expected thresholdPercent to be 25, got %d", merger.thresholdPercent)
	}
}

func TestMerge_SimpleChanges(t *testing.T) {
	merger := NewThreeWayMerger(30) // Increased threshold for byte-based calculation

	existing := "line1\nline2\nline3"
	newContent := "line1\nline2\nline3\nline4"

	result, err := merger.Merge(existing, newContent, "test.txt")
	if err != nil {
		t.Fatalf("Expected no error for simple changes, got: %v", err)
	}

	expected := "line1\nline2\nline3\nline4"
	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestMerge_NoChanges(t *testing.T) {
	merger := NewThreeWayMerger(10)

	content := "line1\nline2\nline3"

	result, err := merger.Merge(content, content, "test.txt")
	if err != nil {
		t.Fatalf("Expected no error for identical content, got: %v", err)
	}

	if result != content {
		t.Errorf("Expected %q, got %q", content, result)
	}
}

func TestMerge_TooManyChanges(t *testing.T) {
	merger := NewThreeWayMerger(5) // 5% threshold

	existing := strings.Repeat("line1\n", 100)
	newContent := strings.Repeat("line2\n", 100)

	_, err := merger.Merge(existing, newContent, "test.txt")
	if err == nil {
		t.Fatal("Expected error for too many changes")
	}

	if !strings.Contains(err.Error(), "too many changes detected") {
		t.Errorf("Expected error about too many changes, got: %v", err)
	}

	if !strings.Contains(err.Error(), "%") {
		t.Errorf("Expected percentage in error message, got: %v", err)
	}
}

func TestMerge_WithConflicts(t *testing.T) {
	merger := NewThreeWayMerger(100) // High threshold to allow diff calculation

	existing := "line1\nline2\nline3"
	newContent := "line1\n<<<<<<< HEAD\nline2\n=======\nline2b\n>>>>>>> branch\nline3"

	// Since the merge package now returns an error for conflicts instead of resolving them
	_, err := merger.Merge(existing, newContent, "test.txt")
	if err == nil {
		t.Fatal("Expected error for merge conflicts, got none")
	}

	if !strings.Contains(err.Error(), "merge conflicts detected") {
		t.Errorf("Expected error about merge conflicts, got: %v", err)
	}
}

func TestMerge_YAMLExample(t *testing.T) {
	merger := NewThreeWayMerger(60) // Increased threshold for byte-based calculation

	existing := `# Custom configuration
base_path: "."
components:
  terraform:
    base_path: "components/terraform"
    apply_auto_approve: false
    deploy_run_init: true
    vars:
      enabled: true
      custom_setting: true`

	newContent := `# Atmos CLI Configuration
# https://atmos.tools/cli/configuration
base_path: "."
components:
  terraform:
    base_path: "components/terraform"
    apply_auto_approve: false
    deploy_run_init: true
    init_run_reconfigure: true
    auto_generate_backend_file: true
    plan:
      skip_planfile: false
  helmfile:
    base_path: "components/helmfile"`

	result, err := merger.Merge(existing, newContent, "atmos.yaml")
	if err != nil {
		t.Fatalf("Expected no error for YAML merge, got: %v", err)
	}

	// Should contain both old and new content (the merge algorithm is complex)
	if !strings.Contains(result, "base_path:") {
		t.Error("Expected to contain base_path configuration")
	}

	// Should add new template content
	if !strings.Contains(result, "init_run_reconfigure: true") {
		t.Error("Expected to add new template content")
	}
}

func TestMerge_ComplexYAMLConflict(t *testing.T) {
	merger := NewThreeWayMerger(5) // Low threshold to trigger rejection

	existing := strings.Repeat(`# Custom configuration
base_path: "."
components:
  terraform:
    base_path: "components/terraform"
    apply_auto_approve: false
    deploy_run_init: true
    vars:
      enabled: true
      custom_setting: true
`, 5)

	newContent := strings.Repeat(`# Atmos CLI Configuration
base_path: "."
components:
  terraform:
    base_path: "components/terraform"
    apply_auto_approve: true
    deploy_run_init: false
    init_run_reconfigure: true
    auto_generate_backend_file: true
`, 5)

	_, err := merger.Merge(existing, newContent, "atmos.yaml")
	if err == nil {
		t.Fatal("Expected error for complex YAML changes")
	}

	if !strings.Contains(err.Error(), "too many changes detected") {
		t.Errorf("Expected error about too many changes, got: %v", err)
	}
}

func BenchmarkMerge_Simple(b *testing.B) {
	merger := NewThreeWayMerger(10)
	existing := "line1\nline2\nline3"
	newContent := "line1\nline2\nline3\nline4"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := merger.Merge(existing, newContent, "test.txt")
		if err != nil {
			b.Fatalf("Unexpected error: %v", err)
		}
	}
}

func BenchmarkMerge_Complex(b *testing.B) {
	merger := NewThreeWayMerger(100)
	existing := strings.Repeat("line1\nline2\nline3\n", 100)
	newContent := strings.Repeat("line1\nline2\nline3\nline4\n", 100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := merger.Merge(existing, newContent, "test.txt")
		if err != nil {
			b.Fatalf("Unexpected error: %v", err)
		}
	}
}
