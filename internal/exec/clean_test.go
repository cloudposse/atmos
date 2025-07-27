package exec

import (
	"fmt"
	"os"
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
				err := os.WriteFile(file, []byte("content"), 0644)
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

// mockFileInfo2 implements os.FileInfo
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
				fs.EXPECT().Lstat("/file.txt").Return(&mockFileInfo2{name: "file.txt", mode: 0644}, nil)
				fs.EXPECT().RemoveAll("/file.txt").Return(nil)
				fs.EXPECT().Stat("/file.txt").Return(nil, os.ErrNotExist)
			},
		},
		{
			name:  "RemoveAll failure",
			paths: []string{"/fail"},
			setupMocks: func(fs *MockFilesystem) {
				fs.EXPECT().Lstat("/fail").Return(&mockFileInfo2{name: "fail", mode: 0644}, nil)
				fs.EXPECT().RemoveAll("/fail").Return(errors.New("remove error"))
			},
			expectedErr: &multierror.Error{Errors: []error{fmt.Errorf("delete /fail: %w", errors.New("remove error"))}},
		},
		{
			name:  "File still exists after deletion",
			paths: []string{"/persistent"},
			setupMocks: func(fs *MockFilesystem) {
				fs.EXPECT().Lstat("/persistent").Return(&mockFileInfo2{name: "persistent", mode: 0644}, nil)
				fs.EXPECT().RemoveAll("/persistent").Return(nil)
				fs.EXPECT().Stat("/persistent").Return(&mockFileInfo2{name: "persistent", mode: 0644}, nil)
			},
			expectedErr: &multierror.Error{Errors: []error{fmt.Errorf("path /persistent still exists after deletion")}},
		},
		{
			name:  "Post-deletion stat error",
			paths: []string{"/stat-error"},
			setupMocks: func(fs *MockFilesystem) {
				fs.EXPECT().Lstat("/stat-error").Return(&mockFileInfo2{name: "stat-error", mode: 0644}, nil)
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
				fs.EXPECT().Lstat("/file.txt").Return(&mockFileInfo2{name: "file.txt", mode: 0644}, nil)
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
