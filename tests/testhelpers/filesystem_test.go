package testhelpers

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestNewMockFS verifies that NewMockFS creates a MockFSBuilder.
func TestNewMockFS(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	builder := NewMockFS(ctrl)

	assert.NotNil(t, builder)
	assert.NotNil(t, builder.fs)
}

// TestMockFSBuilder_WithTempDir verifies WithTempDir sets up expectation.
func TestMockFSBuilder_WithTempDir(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFS := NewMockFS(ctrl).WithTempDir("/tmp/test").Build()

	// Test that expectation was set.
	dir, err := mockFS.MkdirTemp("", "test-")
	assert.NoError(t, err)
	assert.Equal(t, "/tmp/test", dir)
}

// TestMockFSBuilder_WithTempDirError verifies WithTempDirError sets up error expectation.
func TestMockFSBuilder_WithTempDirError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	testErr := errors.New("temp dir error")
	mockFS := NewMockFS(ctrl).WithTempDirError(testErr).Build()

	// Test that error expectation was set.
	dir, err := mockFS.MkdirTemp("", "test-")
	assert.Error(t, err)
	assert.Equal(t, testErr, err)
	assert.Empty(t, dir)
}

// TestMockFSBuilder_WithFile verifies WithFile sets up file read expectation.
func TestMockFSBuilder_WithFile(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	content := []byte("test content")
	mockFS := NewMockFS(ctrl).WithFile("/test.txt", content).Build()

	// Test that expectation was set.
	data, err := mockFS.ReadFile("/test.txt")
	assert.NoError(t, err)
	assert.Equal(t, content, data)
}

// TestMockFSBuilder_WithWriteFile verifies WithWriteFile sets up write expectation.
func TestMockFSBuilder_WithWriteFile(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	content := []byte("test content")
	mockFS := NewMockFS(ctrl).WithWriteFile("/test.txt", content, 0o644).Build()

	// Test that expectation was set.
	err := mockFS.WriteFile("/test.txt", content, 0o644)
	assert.NoError(t, err)
}

// TestMockFSBuilder_Chaining verifies that methods can be chained.
func TestMockFSBuilder_Chaining(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFS := NewMockFS(ctrl).
		WithTempDir("/tmp/test").
		WithFile("/tmp/test/config.yaml", []byte("test: data")).
		WithRemoveAll("/tmp/test", nil).
		Build()

	// Test chained expectations.
	dir, err := mockFS.MkdirTemp("", "test-")
	assert.NoError(t, err)
	assert.Equal(t, "/tmp/test", dir)

	data, err := mockFS.ReadFile("/tmp/test/config.yaml")
	assert.NoError(t, err)
	assert.Equal(t, []byte("test: data"), data)

	err = mockFS.RemoveAll("/tmp/test")
	assert.NoError(t, err)
}

// TestMockFSBuilder_WithMkdirAll verifies WithMkdirAll sets up directory creation expectation.
func TestMockFSBuilder_WithMkdirAll(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFS := NewMockFS(ctrl).WithMkdirAll("/test/dir", 0o755).Build()

	err := mockFS.MkdirAll("/test/dir", 0o755)
	assert.NoError(t, err)
}

// TestMockFSBuilder_WithStat verifies WithStat sets up stat expectation.
func TestMockFSBuilder_WithStat(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create a mock FileInfo (we'll use nil for simplicity in this test).
	mockFS := NewMockFS(ctrl).WithStat("/test.txt", nil, os.ErrNotExist).Build()

	info, err := mockFS.Stat("/test.txt")
	assert.Error(t, err)
	assert.Equal(t, os.ErrNotExist, err)
	assert.Nil(t, info)
}
