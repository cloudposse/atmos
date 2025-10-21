package testhelpers

import (
	"os"

	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/filesystem"
)

// MockFSBuilder provides a fluent interface for building mock filesystem expectations.
// This simplifies test setup by allowing method chaining for common filesystem operations.
//
// Example:
//
//	mockFS := NewMockFS(ctrl).
//	    WithTempDir("/tmp/test-12345").
//	    WithFile("/tmp/test-12345/config.yaml", []byte("test: data")).
//	    WithRemoveAll("/tmp/test-12345", nil).
//	    Build()
type MockFSBuilder struct {
	fs *filesystem.MockFileSystem
}

// NewMockFS creates a new MockFSBuilder.
func NewMockFS(ctrl *gomock.Controller) *MockFSBuilder {
	return &MockFSBuilder{
		fs: filesystem.NewMockFileSystem(ctrl),
	}
}

// WithTempDir sets up an expectation for MkdirTemp to return the specified path.
func (b *MockFSBuilder) WithTempDir(path string) *MockFSBuilder {
	b.fs.EXPECT().MkdirTemp(gomock.Any(), gomock.Any()).Return(path, nil)
	return b
}

// WithTempDirError sets up an expectation for MkdirTemp to return an error.
func (b *MockFSBuilder) WithTempDirError(err error) *MockFSBuilder {
	b.fs.EXPECT().MkdirTemp(gomock.Any(), gomock.Any()).Return("", err)
	return b
}

// WithFile sets up an expectation for ReadFile to return the specified content.
func (b *MockFSBuilder) WithFile(path string, content []byte) *MockFSBuilder {
	b.fs.EXPECT().ReadFile(path).Return(content, nil)
	return b
}

// WithFileError sets up an expectation for ReadFile to return an error.
func (b *MockFSBuilder) WithFileError(path string, err error) *MockFSBuilder {
	b.fs.EXPECT().ReadFile(path).Return(nil, err)
	return b
}

// WithWriteFile sets up an expectation for WriteFile to succeed.
func (b *MockFSBuilder) WithWriteFile(path string, content []byte, perm os.FileMode) *MockFSBuilder {
	b.fs.EXPECT().WriteFile(path, content, perm).Return(nil)
	return b
}

// WithWriteFileError sets up an expectation for WriteFile to return an error.
func (b *MockFSBuilder) WithWriteFileError(path string, content []byte, perm os.FileMode, err error) *MockFSBuilder {
	b.fs.EXPECT().WriteFile(path, content, perm).Return(err)
	return b
}

// WithMkdirAll sets up an expectation for MkdirAll to succeed.
func (b *MockFSBuilder) WithMkdirAll(path string, perm os.FileMode) *MockFSBuilder {
	b.fs.EXPECT().MkdirAll(path, perm).Return(nil)
	return b
}

// WithMkdirAllError sets up an expectation for MkdirAll to return an error.
func (b *MockFSBuilder) WithMkdirAllError(path string, perm os.FileMode, err error) *MockFSBuilder {
	b.fs.EXPECT().MkdirAll(path, perm).Return(err)
	return b
}

// WithRemoveAll sets up an expectation for RemoveAll to succeed or fail.
func (b *MockFSBuilder) WithRemoveAll(path string, err error) *MockFSBuilder {
	b.fs.EXPECT().RemoveAll(path).Return(err)
	return b
}

// WithStat sets up an expectation for Stat to return the specified FileInfo.
func (b *MockFSBuilder) WithStat(path string, info os.FileInfo, err error) *MockFSBuilder {
	b.fs.EXPECT().Stat(path).Return(info, err)
	return b
}

// Build returns the configured MockFileSystem.
func (b *MockFSBuilder) Build() *filesystem.MockFileSystem {
	return b.fs
}
