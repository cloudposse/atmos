package toolchain

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/github"
)

func TestCheckSHACacheStatus_NoBinary(t *testing.T) {
	tempDir := t.TempDir()
	cleanup := setupTestInstallPath(t, tempDir)
	defer cleanup()

	exists, path := CheckSHACacheStatus("abc1234def5678")
	assert.False(t, exists)
	assert.Empty(t, path)
}

func TestCheckSHACacheStatus_BinaryExists(t *testing.T) {
	tempDir := t.TempDir()
	cleanup := setupTestInstallPath(t, tempDir)
	defer cleanup()

	// Create the SHA directory structure with binary.
	shaDir := filepath.Join(tempDir, "bin", "cloudposse", "atmos", "sha-abc1234")
	require.NoError(t, os.MkdirAll(shaDir, 0o755))
	binaryPath := filepath.Join(shaDir, testBinaryName())
	require.NoError(t, os.WriteFile(binaryPath, []byte("fake binary"), 0o755))

	exists, path := CheckSHACacheStatus("abc1234def5678")
	assert.True(t, exists)
	assert.Equal(t, binaryPath, path)
}

func TestCheckSHACacheStatus_ShortSHA(t *testing.T) {
	tempDir := t.TempDir()
	cleanup := setupTestInstallPath(t, tempDir)
	defer cleanup()

	// Use a short SHA (less than 7 chars) - should use it as-is.
	shaDir := filepath.Join(tempDir, "bin", "cloudposse", "atmos", "sha-abc")
	require.NoError(t, os.MkdirAll(shaDir, 0o755))
	binaryPath := filepath.Join(shaDir, testBinaryName())
	require.NoError(t, os.WriteFile(binaryPath, []byte("fake binary"), 0o755))

	exists, path := CheckSHACacheStatus("abc")
	assert.True(t, exists)
	assert.Equal(t, binaryPath, path)
}

func TestSaveSHACacheMetadata(t *testing.T) {
	tempDir := t.TempDir()
	cleanup := setupTestInstallPath(t, tempDir)
	defer cleanup()

	// Create the SHA directory structure.
	shaDir := filepath.Join(tempDir, "bin", "cloudposse", "atmos", "sha-abc1234")
	require.NoError(t, os.MkdirAll(shaDir, 0o755))

	now := time.Now()
	meta := &SHACacheMetadata{
		SHA:       "abc1234def5678901234567890abcdef12345678",
		CheckedAt: now,
		RunID:     54321,
	}

	err := saveSHACacheMetadata("abc1234def5678901234567890abcdef12345678", meta)
	require.NoError(t, err)

	// Verify file was created.
	cacheFile := filepath.Join(shaDir, cacheMetadataFile)
	assert.FileExists(t, cacheFile)

	// Read back and verify.
	data, err := os.ReadFile(cacheFile)
	require.NoError(t, err)

	var loaded SHACacheMetadata
	require.NoError(t, json.Unmarshal(data, &loaded))
	assert.Equal(t, meta.SHA, loaded.SHA)
	assert.Equal(t, meta.RunID, loaded.RunID)
	assert.WithinDuration(t, now, loaded.CheckedAt, time.Second)
}

func TestSaveSHACacheMetadataAfterInstall(t *testing.T) {
	tempDir := t.TempDir()
	cleanup := setupTestInstallPath(t, tempDir)
	defer cleanup()

	sha := "abc1234def5678901234567890abcdef12345678"

	// Create the SHA directory structure.
	shaDir := filepath.Join(tempDir, "bin", "cloudposse", "atmos", "sha-abc1234")
	require.NoError(t, os.MkdirAll(shaDir, 0o755))

	info := &github.SHAArtifactInfo{
		HeadSHA:      sha,
		RunID:        99999,
		ArtifactID:   12345,
		ArtifactName: "build-artifacts-macos",
		SizeInBytes:  1024,
	}

	SaveSHACacheMetadataAfterInstall(sha, info)

	// Verify file was created.
	cacheFile := filepath.Join(shaDir, cacheMetadataFile)
	assert.FileExists(t, cacheFile)

	// Read back and verify.
	data, err := os.ReadFile(cacheFile)
	require.NoError(t, err)

	var loaded SHACacheMetadata
	require.NoError(t, json.Unmarshal(data, &loaded))
	assert.Equal(t, sha, loaded.SHA)
	assert.Equal(t, int64(99999), loaded.RunID)
}

func TestHandleSHAArtifactError(t *testing.T) {
	tests := []struct {
		name         string
		inputErr     error
		sha          string
		expectedType error // Sentinel error to check with errors.Is.
	}{
		{
			name:         "PR not found error maps to ErrToolNotFound",
			inputErr:     fmt.Errorf("%w: #123", github.ErrPRNotFound),
			sha:          "abc1234def5678",
			expectedType: errUtils.ErrToolNotFound,
		},
		{
			name:         "no workflow run error maps to ErrToolNotFound",
			inputErr:     fmt.Errorf("%w: no run", github.ErrNoWorkflowRunFound),
			sha:          "abc1234def5678",
			expectedType: errUtils.ErrToolNotFound,
		},
		{
			name:         "no artifact error maps to ErrToolNotFound",
			inputErr:     fmt.Errorf("%w: no artifact", github.ErrNoArtifactFound),
			sha:          "abc1234def5678",
			expectedType: errUtils.ErrToolNotFound,
		},
		{
			name:         "platform error maps to ErrToolPlatformNotSupported",
			inputErr:     fmt.Errorf("%w: darwin/amd64", github.ErrNoArtifactForPlatform),
			sha:          "abc1234def5678",
			expectedType: errUtils.ErrToolPlatformNotSupported,
		},
		{
			name:         "unsupported platform error maps to ErrToolPlatformNotSupported",
			inputErr:     fmt.Errorf("%w: plan9/amd64", errUtils.ErrUnsupportedPlatform),
			sha:          "abc1234def5678",
			expectedType: errUtils.ErrToolPlatformNotSupported,
		},
		{
			name:         "generic error maps to ErrToolInstall",
			inputErr:     assert.AnError,
			sha:          "abc1234def5678",
			expectedType: errUtils.ErrToolInstall,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handleSHAArtifactError(tt.inputErr, tt.sha)
			assert.Error(t, err)
			assert.ErrorIs(t, err, tt.expectedType)
		})
	}
}

func TestBuildSHANotFoundError(t *testing.T) {
	err := buildSHANotFoundError("abc1234", "https://github.com/cloudposse/atmos/commit/abc1234")
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrToolNotFound)
}

func TestBuildNoWorkflowForSHAError(t *testing.T) {
	err := buildNoWorkflowForSHAError("abc1234", "https://github.com/cloudposse/atmos/commit/abc1234")
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrToolNotFound)
}

func TestBuildNoArtifactForSHAError(t *testing.T) {
	err := buildNoArtifactForSHAError("abc1234", "https://github.com/cloudposse/atmos/commit/abc1234")
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrToolNotFound)
}

func TestBuildPlatformNotSupportedError(t *testing.T) {
	err := buildPlatformNotSupportedError()
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrToolPlatformNotSupported)
}

func TestBuildGenericSHAError(t *testing.T) {
	cause := fmt.Errorf("network timeout")
	err := buildGenericSHAError("abc1234", "https://github.com/cloudposse/atmos/commit/abc1234", cause)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrToolInstall)
}

func TestIsNotFoundError(t *testing.T) {
	assert.True(t, isNotFoundError(github.ErrPRNotFound))
	assert.True(t, isNotFoundError(fmt.Errorf("wrapped: %w", github.ErrPRNotFound)))
	assert.False(t, isNotFoundError(assert.AnError))
}

func TestIsNoWorkflowError(t *testing.T) {
	assert.True(t, isNoWorkflowError(github.ErrNoWorkflowRunFound))
	assert.True(t, isNoWorkflowError(fmt.Errorf("wrapped: %w", github.ErrNoWorkflowRunFound)))
	assert.False(t, isNoWorkflowError(assert.AnError))
}

func TestIsNoArtifactError(t *testing.T) {
	assert.True(t, isNoArtifactError(github.ErrNoArtifactFound))
	assert.True(t, isNoArtifactError(fmt.Errorf("wrapped: %w", github.ErrNoArtifactFound)))
	assert.False(t, isNoArtifactError(assert.AnError))
}

func TestIsPlatformError(t *testing.T) {
	assert.True(t, isPlatformError(github.ErrNoArtifactForPlatform))
	assert.True(t, isPlatformError(fmt.Errorf("wrapped: %w", github.ErrNoArtifactForPlatform)))
	assert.True(t, isPlatformError(errUtils.ErrUnsupportedPlatform))
	assert.False(t, isPlatformError(assert.AnError))
}
