package github

import (
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

// Note: Full integration tests for GetPRArtifactInfo require a real GitHub token.
// and network access. Those would be in an integration test file with appropriate
// skip conditions.
