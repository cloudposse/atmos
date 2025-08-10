package merge

import (
	"strings"
	"testing"
)

func TestNewThreeWayMerger(t *testing.T) {
	merger := NewThreeWayMerger(5)
	if merger.maxChanges != 5 {
		t.Errorf("Expected maxChanges to be 5, got %d", merger.maxChanges)
	}
}

func TestMerge_SimpleChanges(t *testing.T) {
	merger := NewThreeWayMerger(10)

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
	merger := NewThreeWayMerger(5)

	existing := strings.Repeat("line1\n", 10)
	newContent := strings.Repeat("line2\n", 10)

	_, err := merger.Merge(existing, newContent, "test.txt")
	if err == nil {
		t.Fatal("Expected error for too many changes")
	}

	if !strings.Contains(err.Error(), "too many changes detected") {
		t.Errorf("Expected error about too many changes, got: %v", err)
	}
}

func TestMerge_WithConflicts(t *testing.T) {
	merger := NewThreeWayMerger(10)

	existing := "line1\nline2\nline3"
	newContent := "line1\n<<<<<<< HEAD\nline2\n=======\nline2b\n>>>>>>> branch\nline3"

	result, err := merger.Merge(existing, newContent, "test.txt")
	if err != nil {
		t.Fatalf("Expected no error for conflicts, got: %v", err)
	}

	if !strings.Contains(result, "# CONFLICT RESOLVED for test.txt") {
		t.Error("Expected conflict resolution marker in result")
	}
}

func TestResolveConflicts(t *testing.T) {
	merger := NewThreeWayMerger(10)

	content := `line1
<<<<<<< HEAD
line2
=======
line2b
>>>>>>> branch
line3`

	result := merger.resolveConflicts(content, "test.txt")

	if !strings.Contains(result, "# CONFLICT RESOLVED for test.txt") {
		t.Error("Expected conflict resolution marker")
	}

	if !strings.Contains(result, "line2") {
		t.Error("Expected to preserve conflict content")
	}
}

func TestResolveConflictBlock(t *testing.T) {
	merger := NewThreeWayMerger(10)

	conflictLines := []string{"line1", "line2", "line3"}
	result := merger.resolveConflictBlock(conflictLines, "test.txt")

	if len(result) == 0 {
		t.Error("Expected non-empty result")
	}

	if !strings.Contains(result[0], "# CONFLICT RESOLVED for test.txt") {
		t.Error("Expected conflict resolution marker")
	}

	// Check that all non-empty lines are preserved
	for _, line := range conflictLines {
		if strings.TrimSpace(line) != "" {
			found := false
			for _, resultLine := range result {
				if strings.Contains(resultLine, line) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected to find line %q in result", line)
			}
		}
	}
}

func TestMerge_YAMLExample(t *testing.T) {
	merger := NewThreeWayMerger(50) // Increase threshold to handle the YAML changes

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
