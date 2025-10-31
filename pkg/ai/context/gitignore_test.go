package context

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGitignoreFilter_BasicPatterns(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		path     string
		ignored  bool
	}{
		{
			name:     "simple filename",
			patterns: []string{"*.log"},
			path:     "debug.log",
			ignored:  true,
		},
		{
			name:     "no match",
			patterns: []string{"*.log"},
			path:     "main.go",
			ignored:  false,
		},
		{
			name:     "directory pattern",
			patterns: []string{"node_modules"},
			path:     "node_modules/package.json",
			ignored:  true,
		},
		{
			name:     "nested path",
			patterns: []string{"*.log"},
			path:     "logs/debug.log",
			ignored:  true,
		},
		{
			name:     "negation pattern",
			patterns: []string{"*.log", "!important.log"},
			path:     "important.log",
			ignored:  false,
		},
		{
			name:     "negation override",
			patterns: []string{"*.log", "!important.log"},
			path:     "debug.log",
			ignored:  true,
		},
		{
			name:     "rooted pattern",
			patterns: []string{"/root.txt"},
			path:     "root.txt",
			ignored:  true,
		},
		{
			name:     "rooted pattern no match",
			patterns: []string{"/root.txt"},
			path:     "subdir/root.txt",
			ignored:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := &GitignoreFilter{
				basePath: "/test",
				patterns: make([]gitignorePattern, 0),
			}

			// Add patterns.
			for _, p := range tt.patterns {
				negate := false
				if len(p) > 0 && p[0] == '!' {
					negate = true
					p = p[1:]
				}
				filter.patterns = append(filter.patterns, gitignorePattern{
					pattern: p,
					negate:  negate,
				})
			}

			result := filter.IsIgnored(tt.path)
			if result != tt.ignored {
				t.Errorf("Expected IsIgnored(%q) = %v, got %v", tt.path, tt.ignored, result)
			}
		})
	}
}

func TestGitignoreFilter_LoadFile(t *testing.T) {
	// Create temporary directory with .gitignore.
	tmpDir := t.TempDir()

	gitignorePath := filepath.Join(tmpDir, ".gitignore")
	content := `# Comment
*.log
!important.log
node_modules
/root.txt
`
	if err := os.WriteFile(gitignorePath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create .gitignore: %v", err)
	}

	// Load filter.
	filter, err := NewGitignoreFilter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create filter: %v", err)
	}

	// Test patterns were loaded correctly.
	tests := []struct {
		path    string
		ignored bool
	}{
		{"debug.log", true},
		{"important.log", false},
		{"node_modules/pkg.json", true},
		{"root.txt", true},
		{"main.go", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := filter.IsIgnored(tt.path)
			if result != tt.ignored {
				t.Errorf("Expected IsIgnored(%q) = %v, got %v", tt.path, tt.ignored, result)
			}
		})
	}
}

func TestGitignoreFilter_NoFile(t *testing.T) {
	tmpDir := t.TempDir()

	// No .gitignore exists.
	filter, err := NewGitignoreFilter(tmpDir)
	if err != nil {
		t.Fatalf("Expected no error when .gitignore missing, got: %v", err)
	}

	// Should have no patterns.
	if len(filter.patterns) != 0 {
		t.Errorf("Expected 0 patterns, got %d", len(filter.patterns))
	}

	// Should not ignore anything.
	if filter.IsIgnored("test.log") {
		t.Error("Expected file not to be ignored with no patterns")
	}
}

func TestGitignoreFilter_EmptyLines(t *testing.T) {
	tmpDir := t.TempDir()

	gitignorePath := filepath.Join(tmpDir, ".gitignore")
	content := `

*.log

# Comment

node_modules

`
	if err := os.WriteFile(gitignorePath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create .gitignore: %v", err)
	}

	filter, err := NewGitignoreFilter(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create filter: %v", err)
	}

	// Should only have 2 patterns (*.log and node_modules).
	if len(filter.patterns) != 2 {
		t.Errorf("Expected 2 patterns, got %d", len(filter.patterns))
	}
}
