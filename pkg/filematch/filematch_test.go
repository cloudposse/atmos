package filematch

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/charmbracelet/log"
)

func setupTestFixtures(baseDir string) error {
	// Create directory structure:
	// baseDir/
	// ├── file1.txt
	// ├── file2.txt
	// └── subdirectory/
	//     ├── error.log
	//     ├── access.log
	//     └── nested/
	//         └── debug.log

	// Files in root
	files := map[string][]string{
		filepath.Join(baseDir): {
			"file1.txt",
			"file2.txt",
		},
		filepath.Join(baseDir, "subdirectory"): {
			"error.log",
			"access.log",
			"config.yaml",
			"config1.yml",
		},
		filepath.Join(baseDir, "subdirectory", "nested"): {
			"debug.log",
		},
	}

	for dir, filenames := range files {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("failed to create directory %s: %v", dir, err)
		}
		for _, filename := range filenames {
			filePath := filepath.Join(dir, filename)
			if err := os.WriteFile(filePath, []byte("test content"), 0o644); err != nil {
				return fmt.Errorf("failed to create file %s: %v", filePath, err)
			}
		}
	}

	return nil
}

func TestMatchFiles(t *testing.T) {
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get original working directory: %v", err)
	}

	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "matchfiles_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	os.Chdir(tempDir)
	defer os.Chdir(originalDir)
	// Set up the test directory structure
	err = setupTestFixtures(tempDir)
	if err != nil {
		t.Fatalf("failed to set up fixtures: %v", err)
	}

	tests := []struct {
		name     string
		patterns []string
		want     []string
		wantErr  bool
	}{
		{
			name:     "simple wildcard in root",
			patterns: []string{"*.txt"},
			want: []string{
				filepath.Join(tempDir, "file1.txt"),
				filepath.Join(tempDir, "file2.txt"),
			},
			wantErr: false,
		},
		{
			name:     "nested directory with wildcard",
			patterns: []string{"subdirectory" + string(os.PathSeparator) + "*.log"},
			want: []string{
				filepath.Join(tempDir, "subdirectory", "error.log"),
				filepath.Join(tempDir, "subdirectory", "access.log"),
			},
			wantErr: false,
		},
		{
			name:     "absolute path with trailing double star",
			patterns: []string{tempDir + "/subdirectory/**/*.log"},
			want: []string{
				filepath.Join(tempDir, "subdirectory", "error.log"),
				filepath.Join(tempDir, "subdirectory", "access.log"),
				filepath.Join(tempDir, "subdirectory", "nested", "debug.log"),
			},
			wantErr: false,
		},
		{
			name:     "non-existent directory",
			patterns: []string{"nonexistent/*.txt"},
			want:     nil,
			wantErr:  true,
		},
		{
			name:     "multiple patterns",
			patterns: []string{"*.txt", "subdirectory/*.log"},
			want: []string{
				filepath.Join(tempDir, "file1.txt"),
				filepath.Join(tempDir, "file2.txt"),
				filepath.Join(tempDir, "subdirectory", "error.log"),
				filepath.Join(tempDir, "subdirectory", "access.log"),
			},
			wantErr: false,
		},
		{
			name:     "pattern with double star",
			patterns: []string{"**/*.log"},
			want: []string{
				filepath.Join(tempDir, "subdirectory", "error.log"),
				filepath.Join(tempDir, "subdirectory", "access.log"),
				filepath.Join(tempDir, "subdirectory", "nested", "debug.log"),
			},
			wantErr: false,
		},

		{
			name:     "pattern with double star directory",
			patterns: []string{"**/nested/*.log"},
			want: []string{
				filepath.Join(tempDir, "subdirectory", "nested", "debug.log"),
			},
			wantErr: false,
		},
		{
			name:     "pattern that accepts multiple yaml file types",
			patterns: []string{"**/*.{yaml,yml}"},
			want: []string{
				filepath.Join(tempDir, "subdirectory", "config.yaml"),
				filepath.Join(tempDir, "subdirectory", "config1.yml"),
			},
		},
	}

	log.SetLevel(log.DebugLevel)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			globMatcher := NewGlobMatcher()
			got, err := globMatcher.MatchFiles(tt.patterns)
			if (err != nil) != tt.wantErr {
				t.Errorf("MatchFiles() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			sort.Strings(got)
			sort.Strings(tt.want)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("MatchFiles() = %v, want %v", got, tt.want)
			}
		})
	}
}
