package github

import (
	"errors"
	"fmt"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetArtifactNameForPlatform(t *testing.T) {
	// This test verifies the platform mapping logic.
	// The actual result depends on the runtime platform.

	artifactName, err := getArtifactNameForPlatform()

	switch runtime.GOOS {
	case "linux":
		if runtime.GOARCH == "amd64" {
			assert.NoError(t, err)
			assert.Equal(t, "build-artifacts-linux", artifactName)
		} else {
			assert.Error(t, err)
			assert.ErrorIs(t, err, ErrNoArtifactForPlatform)
		}
	case "darwin":
		if runtime.GOARCH == "arm64" {
			assert.NoError(t, err)
			assert.Equal(t, "build-artifacts-macos", artifactName)
		} else {
			assert.Error(t, err)
			assert.ErrorIs(t, err, ErrNoArtifactForPlatform)
		}
	case "windows":
		if runtime.GOARCH == "amd64" {
			assert.NoError(t, err)
			assert.Equal(t, "build-artifacts-windows", artifactName)
		} else {
			assert.Error(t, err)
			assert.ErrorIs(t, err, ErrNoArtifactForPlatform)
		}
	default:
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrUnsupportedPlatform)
	}
}

func TestSupportedPRPlatforms(t *testing.T) {
	platforms := SupportedPRPlatforms()

	assert.Len(t, platforms, 3)
	assert.Contains(t, platforms, "linux/amd64")
	assert.Contains(t, platforms, "darwin/arm64")
	assert.Contains(t, platforms, "windows/amd64")
}

func TestIsNotFoundError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "direct ErrPRNotFound",
			err:      ErrPRNotFound,
			expected: true,
		},
		{
			name:     "wrapped ErrPRNotFound",
			err:      fmt.Errorf("context: %w", ErrPRNotFound),
			expected: true,
		},
		{
			name:     "unrelated error",
			err:      errors.New("something else"),
			expected: false,
		},
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsNotFoundError(tt.err))
		})
	}
}

func TestIsNoWorkflowError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "direct ErrNoWorkflowRunFound",
			err:      ErrNoWorkflowRunFound,
			expected: true,
		},
		{
			name:     "wrapped ErrNoWorkflowRunFound",
			err:      fmt.Errorf("context: %w", ErrNoWorkflowRunFound),
			expected: true,
		},
		{
			name:     "unrelated error",
			err:      errors.New("something else"),
			expected: false,
		},
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsNoWorkflowError(tt.err))
		})
	}
}

func TestIsNoArtifactError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "direct ErrNoArtifactFound",
			err:      ErrNoArtifactFound,
			expected: true,
		},
		{
			name:     "wrapped ErrNoArtifactFound",
			err:      fmt.Errorf("context: %w", ErrNoArtifactFound),
			expected: true,
		},
		{
			name:     "unrelated error",
			err:      errors.New("something else"),
			expected: false,
		},
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsNoArtifactError(tt.err))
		})
	}
}

func TestIsPlatformError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "direct ErrNoArtifactForPlatform",
			err:      ErrNoArtifactForPlatform,
			expected: true,
		},
		{
			name:     "wrapped ErrNoArtifactForPlatform",
			err:      fmt.Errorf("context: %w", ErrNoArtifactForPlatform),
			expected: true,
		},
		{
			name:     "direct ErrUnsupportedPlatform",
			err:      ErrUnsupportedPlatform,
			expected: true,
		},
		{
			name:     "wrapped ErrUnsupportedPlatform",
			err:      fmt.Errorf("context: %w", ErrUnsupportedPlatform),
			expected: true,
		},
		{
			name:     "unrelated error",
			err:      errors.New("something else"),
			expected: false,
		},
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsPlatformError(tt.err))
		})
	}
}

// Note: Full integration tests for GetPRArtifactInfo require a real GitHub token.
// and network access. Those would be in an integration test file with appropriate
// skip conditions.
