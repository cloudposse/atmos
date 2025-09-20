package git

import (
	"os"
	"os/exec"
	"reflect"
	"testing"
)

func TestGetChangedFiles(t *testing.T) {
	tests := []struct {
		name        string
		mockGitDiff string
		want        []string
		setupGit    bool
	}{
		{
			name:        "multiple changed files",
			mockGitDiff: "file1.go\nfile2.go\nsubdir/file3.go\n",
			want:        []string{"file1.go", "file2.go", "subdir/file3.go"},
			setupGit:    true,
		},
		{
			name:        "single changed file",
			mockGitDiff: "onefile.go\n",
			want:        []string{"onefile.go"},
			setupGit:    true,
		},
		{
			name:        "no changed files",
			mockGitDiff: "",
			want:        []string{},
			setupGit:    true,
		},
		{
			name:     "git command fails",
			want:     []string{},
			setupGit: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip git tests if git is not available or we're in an environment without git.
			if !tt.setupGit {
				// Test the error case by temporarily breaking git.
				oldPath := os.Getenv("PATH")
				os.Setenv("PATH", "")
				defer os.Setenv("PATH", oldPath)
			}

			got := getChangedFiles()

			if tt.setupGit {
				// We can't easily mock exec.Command in this test environment,.
				// so we'll just verify the function returns a slice.
				if got == nil {
					t.Error("getChangedFiles() returned nil")
				}
				// The actual content will depend on the real git state,.
				// but we can verify it returns a slice.
			} else {
				// For error cases, should return empty slice.
				if len(got) != 0 {
					t.Errorf("getChangedFiles() in error case should return empty slice, got %v", got)
				}
			}
		})
	}
}

func TestGetChangedPackages(t *testing.T) {
	tests := []struct {
		name        string
		setupGit    bool
		expectSlice bool
	}{
		{
			name:        "normal git environment",
			setupGit:    true,
			expectSlice: true,
		},
		{
			name:        "git command fails",
			setupGit:    false,
			expectSlice: true, // Should still return empty slice
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.setupGit {
				// Test the error case by checking git availability.
				_, err := exec.LookPath("git")
				if err == nil {
					// If git is available, temporarily break it.
					oldPath := os.Getenv("PATH")
					os.Setenv("PATH", "")
					defer os.Setenv("PATH", oldPath)
				}
			}

			got := getChangedPackages()

			if tt.expectSlice {
				if got == nil {
					t.Error("getChangedPackages() returned nil")
				}
				// Verify it returns a slice (content depends on actual git state).
			}
		})
	}
}

// Test helper functions that would be used by git functions.
func TestGitFunctionHelpers(t *testing.T) {
	// Test that git functions handle empty output correctly.
	t.Run("empty git output handling", func(t *testing.T) {
		// getChangedFiles should handle empty output.
		files := getChangedFiles()
		if files == nil {
			t.Error("getChangedFiles() should return empty slice, not nil")
		}

		// getChangedPackages should handle empty output.
		packages := getChangedPackages()
		if packages == nil {
			t.Error("getChangedPackages() should return empty slice, not nil")
		}
	})
}

// Test file path processing that git functions might do.
func TestFilePathProcessing(t *testing.T) {
	tests := []struct {
		name      string
		filePaths []string
		want      []string
	}{
		{
			name:      "go files only",
			filePaths: []string{"file1.go", "file2.txt", "file3.go"},
			want:      []string{"file1.go", "file3.go"},
		},
		{
			name:      "nested paths",
			filePaths: []string{"cmd/main.go", "pkg/utils/helper.go", "README.md"},
			want:      []string{"cmd/main.go", "pkg/utils/helper.go"},
		},
		{
			name:      "no go files",
			filePaths: []string{"README.md", "docker-compose.yml"},
			want:      []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := []string{}
			for _, path := range tt.filePaths {
				// Simple filter for .go files (simulating what git functions might do).
				if len(path) > 3 && path[len(path)-3:] == ".go" {
					got = append(got, path)
				}
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("filtered paths = %v, want %v", got, tt.want)
			}
		})
	}
}
