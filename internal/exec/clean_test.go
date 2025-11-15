package exec

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/assert/v2"
	"github.com/golang/mock/gomock"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
)

// TestDeletePaths tests the public DeletePaths function using a real osFilesystem and capturing logs.
func TestDeletePaths(t *testing.T) {
	tests := []struct {
		name     string
		paths    []string
		setup    func(t *testing.T) string // Returns temp dir for cleanup
		expected string
	}{
		{
			name:  "Empty path",
			paths: []string{""},
			setup: func(t *testing.T) string { return "" },
		},
		{
			name:  "Non-existent path",
			paths: []string{"/nonexistent"},
			setup: func(t *testing.T) string { return "" },
		},
		{
			name:  "Successful deletion",
			paths: []string{"testfile.txt"},
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				file := dir + "/testfile.txt"
				err := os.WriteFile(file, []byte("content"), 0o644)
				assert.NoError(t, err, "Failed to create test file")
				return dir
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up test environment
			dir := tt.setup(t)
			paths := tt.paths
			if dir != "" {
				// Prepend temp dir to paths that need it
				for i, p := range paths {
					if p != "" && p != "nonexistent" {
						paths[i] = dir + "/" + p
					}
				}
			}

			// Run DeletePaths
			err := DeletePaths(paths)

			// Assert error
			if tt.expected != "" {
				assert.EqualError(t, err, tt.expected, "Unexpected error")
			} else {
				assert.NoError(t, err, "Expected no error")
			}
		})
	}
}

// mockFileInfo2 implements os.FileInfo.
type mockFileInfo2 struct {
	name    string
	size    int64
	mode    os.FileMode
	modTime time.Time
	isDir   bool
	sys     interface{}
}

func (m *mockFileInfo2) Name() string       { return m.name }
func (m *mockFileInfo2) Size() int64        { return m.size }
func (m *mockFileInfo2) Mode() os.FileMode  { return m.mode }
func (m *mockFileInfo2) ModTime() time.Time { return m.modTime }
func (m *mockFileInfo2) IsDir() bool        { return m.isDir }
func (m *mockFileInfo2) Sys() interface{}   { return m.sys }

// Test_deletePaths tests the private deletePaths function using mockgen mocks.
func Test_deletePaths(t *testing.T) {
	tests := []struct {
		name        string
		paths       []string
		setupMocks  func(fs *MockFilesystem)
		expectedErr error
	}{
		{
			name:  "Empty path",
			paths: []string{""},
			setupMocks: func(fs *MockFilesystem) {
			},
		},
		{
			name:  "Non-existent path",
			paths: []string{"/nonexistent"},
			setupMocks: func(fs *MockFilesystem) {
				fs.EXPECT().Lstat("/nonexistent").Return(nil, os.ErrNotExist)
			},
		},
		{
			name:  "Lstat error",
			paths: []string{"/error"},
			setupMocks: func(fs *MockFilesystem) {
				fs.EXPECT().Lstat("/error").Return(nil, errors.New("lstat error"))
			},
			expectedErr: &multierror.Error{Errors: []error{fmt.Errorf("stat /error: %w", errors.New("lstat error"))}},
		},
		{
			name:  "Successful deletion",
			paths: []string{"/file.txt"},
			setupMocks: func(fs *MockFilesystem) {
				fs.EXPECT().Lstat("/file.txt").Return(&mockFileInfo2{name: "file.txt", mode: 0o644}, nil)
				fs.EXPECT().RemoveAll("/file.txt").Return(nil)
				fs.EXPECT().Stat("/file.txt").Return(nil, os.ErrNotExist)
			},
		},
		{
			name:  "RemoveAll failure",
			paths: []string{"/fail"},
			setupMocks: func(fs *MockFilesystem) {
				fs.EXPECT().Lstat("/fail").Return(&mockFileInfo2{name: "fail", mode: 0o644}, nil)
				fs.EXPECT().RemoveAll("/fail").Return(errors.New("remove error"))
			},
			expectedErr: &multierror.Error{Errors: []error{fmt.Errorf("delete /fail: %w", errors.New("remove error"))}},
		},
		{
			name:  "File still exists after deletion",
			paths: []string{"/persistent"},
			setupMocks: func(fs *MockFilesystem) {
				fs.EXPECT().Lstat("/persistent").Return(&mockFileInfo2{name: "persistent", mode: 0o644}, nil)
				fs.EXPECT().RemoveAll("/persistent").Return(nil)
				fs.EXPECT().Stat("/persistent").Return(&mockFileInfo2{name: "persistent", mode: 0o644}, nil)
			},
			expectedErr: &multierror.Error{Errors: []error{fmt.Errorf("path /persistent still exists after deletion")}},
		},
		{
			name:  "Post-deletion stat error",
			paths: []string{"/stat-error"},
			setupMocks: func(fs *MockFilesystem) {
				fs.EXPECT().Lstat("/stat-error").Return(&mockFileInfo2{name: "stat-error", mode: 0o644}, nil)
				fs.EXPECT().RemoveAll("/stat-error").Return(nil)
				fs.EXPECT().Stat("/stat-error").Return(nil, errors.New("stat error"))
			},
			expectedErr: &multierror.Error{Errors: []error{fmt.Errorf("post-deletion stat /stat-error: %w", errors.New("stat error"))}},
		},
		{
			name:  "Multiple paths mixed cases",
			paths: []string{"", "/nonexistent", "/file.txt", "/error"},
			setupMocks: func(fs *MockFilesystem) {
				fs.EXPECT().Lstat("/nonexistent").Return(nil, os.ErrNotExist)
				fs.EXPECT().Lstat("/file.txt").Return(&mockFileInfo2{name: "file.txt", mode: 0o644}, nil)
				fs.EXPECT().RemoveAll("/file.txt").Return(nil)
				fs.EXPECT().Stat("/file.txt").Return(nil, os.ErrNotExist)
				fs.EXPECT().Lstat("/error").Return(nil, errors.New("lstat error"))
			},
			expectedErr: &multierror.Error{Errors: []error{fmt.Errorf("stat /error: %w", errors.New("lstat error"))}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockFs := NewMockFilesystem(ctrl)

			tt.setupMocks(mockFs)

			err := deletePaths(mockFs, tt.paths)

			if tt.expectedErr != nil {
				assert.EqualError(t, err, tt.expectedErr.Error(), "Unexpected error")
			} else {
				assert.NoError(t, err, "Expected no error")
			}
		})
	}
}

func TestIsValidStack(t *testing.T) {
	tests := []struct {
		name      string
		stackInfo any
		stackName string
		stack     string
		expected  bool
	}{
		{
			name:      "Valid stack with empty filter",
			stackInfo: map[string]any{},
			stackName: "stack1",
			stack:     "",
			expected:  true,
		},
		{
			name:      "Valid stack with matching filter",
			stackInfo: map[string]any{},
			stackName: "stack1",
			stack:     "stack1",
			expected:  true,
		},
		{
			name:      "Invalid stack with non-matching filter",
			stackInfo: map[string]any{},
			stackName: "stack1",
			stack:     "stack2",
			expected:  false,
		},
		{
			name:      "Nil stack info",
			stackInfo: nil,
			stackName: "stack1",
			stack:     "",
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidStack(tt.stackInfo, tt.stackName, tt.stack)
			if result != tt.expected {
				t.Errorf("isValidStack() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestExtractCleanPatterns(t *testing.T) {
	tests := []struct {
		name           string
		componentValue any
		setupFiles     map[string]string // path -> content (empty string for directories)
		want           []string
		wantErr        bool
	}{
		{
			name: "valid component with clean patterns matching log files",
			componentValue: map[string]any{
				"settings": map[string]any{
					"clean": map[string]any{
						"paths": []any{"*.log", "*.txt"},
					},
				},
			},
			setupFiles: map[string]string{
				"app.log":     "log content",
				"error.log":   "error content",
				"readme.txt":  "readme",
				"config.json": "{}",
			},
			want:    []string{"app.log", "error.log", "readme.txt"},
			wantErr: false,
		},
		{
			name: "patterns matching files in subdirectories",
			componentValue: map[string]any{
				"settings": map[string]any{
					"clean": map[string]any{
						"paths": []any{"temp/*", "cache/**/*.tmp"},
					},
				},
			},
			setupFiles: map[string]string{
				"temp/":              "",
				"temp/data.txt":      "temp data",
				"temp/old.log":       "old log",
				"cache/":             "",
				"cache/sub/":         "",
				"cache/file.tmp":     "cache",
				"cache/sub/test.tmp": "test",
			},
			want:    []string{filepath.Join("cache", "file.tmp"), filepath.Join("cache", "sub", "test.tmp"), "temp", filepath.Join("temp", "data.txt"), filepath.Join("temp", "old.log")},
			wantErr: false,
		},
		{
			name: "no matching files",
			componentValue: map[string]any{
				"settings": map[string]any{
					"clean": map[string]any{
						"paths": []any{"*.log"},
					},
				},
			},
			setupFiles: map[string]string{
				"readme.txt":  "readme",
				"config.json": "{}",
			},
			want:    nil,
			wantErr: false,
		},
		{
			name:           "nil component value",
			componentValue: nil,
			setupFiles:     map[string]string{},
			want:           nil,
			wantErr:        false,
		},
		{
			name:           "not a map",
			componentValue: "invalid",
			setupFiles:     map[string]string{},
			want:           nil,
			wantErr:        false,
		},
		{
			name: "missing settings",
			componentValue: map[string]any{
				"other": "value",
			},
			setupFiles: map[string]string{},
			want:       nil,
			wantErr:    false,
		},
		{
			name: "settings not a map",
			componentValue: map[string]any{
				"settings": "invalid",
			},
			setupFiles: map[string]string{},
			want:       nil,
			wantErr:    false,
		},
		{
			name: "missing clean setting",
			componentValue: map[string]any{
				"settings": map[string]any{
					"other": "value",
				},
			},
			setupFiles: map[string]string{},
			want:       nil,
			wantErr:    false,
		},
		{
			name: "clean setting not a map",
			componentValue: map[string]any{
				"settings": map[string]any{
					"clean": "invalid",
				},
			},
			setupFiles: map[string]string{},
			want:       nil,
			wantErr:    false,
		},
		{
			name: "missing paths",
			componentValue: map[string]any{
				"settings": map[string]any{
					"clean": map[string]any{
						"other": "value",
					},
				},
			},
			setupFiles: map[string]string{},
			want:       nil,
			wantErr:    false,
		},
		{
			name: "paths not an array",
			componentValue: map[string]any{
				"settings": map[string]any{
					"clean": map[string]any{
						"paths": "invalid",
					},
				},
			},
			setupFiles: map[string]string{},
			want:       nil,
			wantErr:    false,
		},
		{
			name: "empty paths array",
			componentValue: map[string]any{
				"settings": map[string]any{
					"clean": map[string]any{
						"paths": []any{},
					},
				},
			},
			setupFiles: map[string]string{
				"file.txt": "content",
			},
			want:    nil,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary directory for test
			tmpDir, err := os.MkdirTemp("", "test-extract-clean-patterns-*")
			if err != nil {
				t.Fatalf("failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(tmpDir)

			// Change to temp directory
			oldDir, err := os.Getwd()
			if err != nil {
				t.Fatalf("failed to get current dir: %v", err)
			}
			if err := os.Chdir(tmpDir); err != nil {
				t.Fatalf("failed to change dir: %v", err)
			}
			defer os.Chdir(oldDir)

			// Setup test files and directories
			for path, content := range tt.setupFiles {
				if strings.HasSuffix(path, "/") {
					// Create directory
					if err := os.MkdirAll(path, 0o755); err != nil {
						t.Fatalf("failed to create dir %s: %v", path, err)
					}
				} else {
					// Create file with parent directories
					dir := filepath.Dir(path)
					if dir != "." {
						if err := os.MkdirAll(dir, 0o755); err != nil {
							t.Fatalf("failed to create parent dir %s: %v", dir, err)
						}
					}
					if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
						t.Fatalf("failed to create file %s: %v", path, err)
					}
				}
			}

			// Execute function
			got, err := extractCleanPatterns(tt.componentValue)
			if (err != nil) != tt.wantErr {
				t.Errorf("extractCleanPatterns() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Convert absolute paths to relative paths
			var relativeGot []string
			for _, path := range got {
				relPath, err := filepath.Rel(tmpDir, path)
				if err != nil {
					t.Fatalf("failed to get relative path: %v", err)
				}
				relativeGot = append(relativeGot, relPath)
			}

			// Sort both slices for comparison
			sort.Strings(relativeGot)
			if tt.want != nil {
				sort.Strings(tt.want)
			}
			for i := range relativeGot {
				if !strings.Contains(relativeGot[i], tt.want[i]) {
					t.Errorf("extractCleanPatterns() got = %v, want %v", relativeGot, tt.want)
				}
			}
		})
	}
}
