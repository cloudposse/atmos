package pro

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestExecuteCommit_LoopPrevention(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	atmosConfig.Settings.GithubUsername = AtmosProBotActor

	err := ExecuteCommit(atmosConfig, "test", "", "", false)
	assert.NoError(t, err)
}

func TestValidateCommitInputs(t *testing.T) {
	t.Run("valid inputs", func(t *testing.T) {
		err := validateCommitInputs("fix formatting", "looks good")
		assert.NoError(t, err)
	})

	t.Run("empty message", func(t *testing.T) {
		err := validateCommitInputs("", "")
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrCommitMessageRequired)
	})

	t.Run("message too long", func(t *testing.T) {
		longMsg := make([]byte, 501)
		for i := range longMsg {
			longMsg[i] = 'a'
		}
		err := validateCommitInputs(string(longMsg), "")
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrCommitMessageTooLong)
	})

	t.Run("message at max length", func(t *testing.T) {
		maxMsg := make([]byte, 500)
		for i := range maxMsg {
			maxMsg[i] = 'a'
		}
		err := validateCommitInputs(string(maxMsg), "")
		assert.NoError(t, err)
	})

	t.Run("comment too long", func(t *testing.T) {
		longComment := make([]byte, 2001)
		for i := range longComment {
			longComment[i] = 'a'
		}
		err := validateCommitInputs("fix", string(longComment))
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrCommentTooLong)
	})

	t.Run("comment at max length", func(t *testing.T) {
		maxComment := make([]byte, 2000)
		for i := range maxComment {
			maxComment[i] = 'a'
		}
		err := validateCommitInputs("fix", string(maxComment))
		assert.NoError(t, err)
	})
}

func TestValidateBranch(t *testing.T) {
	t.Run("valid branches", func(t *testing.T) {
		validBranches := []string{
			"main",
			"feature/my-branch",
			"fix/bug-123",
			"release/v1.0.0",
			"user.name/feature",
			"feature_test",
		}
		for _, branch := range validBranches {
			err := validateBranch(branch)
			assert.NoError(t, err, "branch %q should be valid", branch)
		}
	})

	t.Run("invalid branch characters", func(t *testing.T) {
		err := validateBranch("feature branch")
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrBranchInvalid)
	})

	t.Run("branch too long", func(t *testing.T) {
		longBranch := make([]byte, 257)
		for i := range longBranch {
			longBranch[i] = 'a'
		}
		err := validateBranch(string(longBranch))
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrBranchInvalid)
	})

	t.Run("branch at max length", func(t *testing.T) {
		maxBranch := make([]byte, 256)
		for i := range maxBranch {
			maxBranch[i] = 'a'
		}
		err := validateBranch(string(maxBranch))
		assert.NoError(t, err)
	})
}

func TestValidatePath(t *testing.T) {
	t.Run("valid paths", func(t *testing.T) {
		validPaths := []string{
			"main.tf",
			"modules/vpc/main.tf",
			"stacks/dev.yaml",
		}
		for _, p := range validPaths {
			err := validatePath(p)
			assert.NoError(t, err, "path %q should be valid", p)
		}
	})

	t.Run("absolute path rejected", func(t *testing.T) {
		err := validatePath("/etc/passwd")
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrCommitInvalidFilePath)
	})

	t.Run("path traversal rejected", func(t *testing.T) {
		err := validatePath("../secret.tf")
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrCommitInvalidFilePath)
	})

	t.Run("github path rejected", func(t *testing.T) {
		err := validatePath(".github/workflows/ci.yml")
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrCommitInvalidFilePath)
	})

	t.Run("github path rejected case insensitive", func(t *testing.T) {
		err := validatePath(".GitHub/workflows/ci.yml")
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrCommitInvalidFilePath)
	})
}

func TestFilterPaths(t *testing.T) {
	t.Run("filters invalid paths", func(t *testing.T) {
		paths := []string{
			"main.tf",
			".github/workflows/ci.yml",
			"modules/vpc/main.tf",
			"../escape.tf",
		}
		result := filterPaths(paths)
		assert.Equal(t, []string{"main.tf", "modules/vpc/main.tf"}, result)
	})

	t.Run("all valid paths pass through", func(t *testing.T) {
		paths := []string{"a.tf", "b.tf"}
		result := filterPaths(paths)
		assert.Equal(t, paths, result)
	})

	t.Run("empty input", func(t *testing.T) {
		result := filterPaths([]string{})
		assert.Empty(t, result)
	})
}

func TestCollectFileContents(t *testing.T) {
	t.Run("reads and encodes files", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "test.tf")
		content := []byte("resource \"aws_vpc\" \"main\" {}")
		err := os.WriteFile(filePath, content, 0o644)
		require.NoError(t, err)

		result, err := collectFileContents([]string{filePath})
		require.NoError(t, err)
		require.Len(t, result, 1)
		assert.Equal(t, filePath, result[0].Path)

		decoded, err := base64.StdEncoding.DecodeString(result[0].Contents)
		require.NoError(t, err)
		assert.Equal(t, content, decoded)
	})

	t.Run("skips files exceeding size limit", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "large.bin")
		// Create a file just over 2 MiB.
		largeContent := make([]byte, maxFileSizeBytes+1)
		err := os.WriteFile(filePath, largeContent, 0o644)
		require.NoError(t, err)

		result, err := collectFileContents([]string{filePath})
		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("file at max size is included", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "exact.bin")
		content := make([]byte, maxFileSizeBytes)
		err := os.WriteFile(filePath, content, 0o644)
		require.NoError(t, err)

		result, err := collectFileContents([]string{filePath})
		require.NoError(t, err)
		require.Len(t, result, 1)
	})

	t.Run("nonexistent file returns error", func(t *testing.T) {
		result, err := collectFileContents([]string{filepath.Join(t.TempDir(), "nope.tf")})
		require.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("empty input", func(t *testing.T) {
		result, err := collectFileContents([]string{})
		require.NoError(t, err)
		assert.Empty(t, result)
	})
}
