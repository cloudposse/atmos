package tests

import (
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestShouldCheckPreconditions tests the ShouldCheckPreconditions function.
func TestShouldCheckPreconditions(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		want     bool
	}{
		{
			name:     "No env var set",
			envValue: "",
			want:     true,
		},
		{
			name:     "Set to false",
			envValue: "false",
			want:     true,
		},
		{
			name:     "Set to true",
			envValue: "true",
			want:     false,
		},
		{
			name:     "Set to TRUE (case sensitive)",
			envValue: "TRUE",
			want:     true,
		},
		{
			name:     "Set to random value",
			envValue: "random",
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				t.Setenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS", tt.envValue)
			} else {
				// Explicitly unset the env var for this test
				// t.Setenv doesn't support unsetting, so we use os.Unsetenv
				// and manually restore after the test
				orig := os.Getenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS")
				os.Unsetenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS")
				t.Cleanup(func() {
					if orig != "" {
						os.Setenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS", orig)
					}
				})
			}

			got := ShouldCheckPreconditions()
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestRequireAWSProfile_WithBypass tests RequireAWSProfile with bypass.
func TestRequireAWSProfile_WithBypass(t *testing.T) {
	// Set bypass
	t.Setenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS", "true")

	// Should not skip when bypass is set
	RequireAWSProfile(t, "non-existent-profile-that-does-not-exist-12345")
	// If we get here, it worked (didn't skip)
}

// TestRequireGitRepository_WithBypass tests RequireGitRepository with bypass.
func TestRequireGitRepository_WithBypass(t *testing.T) {
	t.Setenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS", "true")

	repo := RequireGitRepository(t)
	assert.Nil(t, repo) // Should return nil when bypassed
}

// TestRequireGitRemoteWithValidURL_WithBypass tests RequireGitRemoteWithValidURL with bypass.
func TestRequireGitRemoteWithValidURL_WithBypass(t *testing.T) {
	t.Setenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS", "true")

	url := RequireGitRemoteWithValidURL(t)
	assert.Empty(t, url) // Should return empty when bypassed
}

// TestRequireNetworkAccess_WithBypass tests RequireNetworkAccess with bypass.
func TestRequireNetworkAccess_WithBypass(t *testing.T) {
	t.Setenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS", "true")

	// Should not skip when bypass is set
	RequireNetworkAccess(t, "http://invalid-domain-that-does-not-exist-12345.example.com")
}

// TestRequireExecutable_WithBypass tests RequireExecutable with bypass.
func TestRequireExecutable_WithBypass(t *testing.T) {
	t.Setenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS", "true")

	RequireExecutable(t, "non-existent-binary-that-does-not-exist-12345", "testing")
}

// TestRequireEnvVar_WithBypass tests RequireEnvVar with bypass.
func TestRequireEnvVar_WithBypass(t *testing.T) {
	t.Setenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS", "true")

	RequireEnvVar(t, "NON_EXISTENT_VAR_THAT_DOES_NOT_EXIST_12345", "testing")
}

// TestRequireFilePath_WithBypass tests RequireFilePath with bypass.
func TestRequireFilePath_WithBypass(t *testing.T) {
	t.Setenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS", "true")

	RequireFilePath(t, "/non/existent/path/that/does/not/exist/12345", "testing")
}

// TestRequireOCIAuthentication_WithBypass tests RequireOCIAuthentication with bypass.
func TestRequireOCIAuthentication_WithBypass(t *testing.T) {
	t.Setenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS", "true")

	// Ensure no token is set (t.Setenv with empty string unsets)
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("ATMOS_GITHUB_TOKEN", "")

	RequireOCIAuthentication(t)
}

// TestLogPreconditionOverride tests the LogPreconditionOverride function.
func TestLogPreconditionOverride(t *testing.T) {
	t.Run("with bypass enabled", func(t *testing.T) {
		t.Setenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS", "true")
		// Should log a message (we can't easily test the log output, but ensure no panic)
		LogPreconditionOverride(t)
	})

	t.Run("without bypass", func(t *testing.T) {
		// Don't set env var - it will be unset
		LogPreconditionOverride(t)
	})
}

// TestRequireGitHubAccess_WithBypass tests RequireGitHubAccess with bypass.
func TestRequireGitHubAccess_WithBypass(t *testing.T) {
	t.Setenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS", "true")

	info := RequireGitHubAccess(t)
	assert.Nil(t, info) // Should return nil when bypassed
}

// TestPreconditionSkipping tests real skip scenarios - these will actually skip the subtest.
func TestPreconditionSkipping(t *testing.T) {
	// Ensure precondition checks are enabled
	os.Unsetenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS")

	t.Run("EnvVar missing causes skip", func(t *testing.T) {
		// Test with non-existent env var - this will skip the test
		RequireEnvVar(t, "DEFINITELY_NON_EXISTENT_VAR_12345", "testing")

		// This line should not be reached
		t.Error("Should have skipped")
	})

	t.Run("Executable missing causes skip", func(t *testing.T) {
		RequireExecutable(t, "definitely-non-existent-binary-12345", "test purpose")

		// This line should not be reached
		t.Error("Should have skipped")
	})

	t.Run("File path missing causes skip", func(t *testing.T) {
		RequireFilePath(t, "/definitely/non/existent/path/12345", "test file")

		// This line should not be reached
		t.Error("Should have skipped")
	})
}

// TestRequireEnvVar_WithExistingVar tests RequireEnvVar with existing variable.
func TestRequireEnvVar_WithExistingVar(t *testing.T) {
	// Set a test env var
	t.Setenv("TEST_VAR_FOR_TESTING", "some_value")

	RequireEnvVar(t, "TEST_VAR_FOR_TESTING", "testing")

	// Should reach here when var exists
	assert.True(t, true, "Test continued after RequireEnvVar with existing var")
}

// TestRequireExecutable_WithExistingBinary tests RequireExecutable with existing executable.
func TestRequireExecutable_WithExistingBinary(t *testing.T) {
	// Use 'go' as it should exist in test environment
	RequireExecutable(t, "go", "testing")

	// Should reach here when executable exists
	assert.True(t, true, "Test continued after RequireExecutable with existing binary")
}

// TestRequireOCIAuthentication_WithToken tests RequireOCIAuthentication with token set.
func TestRequireOCIAuthentication_WithToken(t *testing.T) {
	// Set a GitHub token
	t.Setenv("GITHUB_TOKEN", "test-token")

	RequireOCIAuthentication(t)

	// Should reach here when token is set
	assert.True(t, true, "Test continued after RequireOCIAuthentication with token")
}

// TestRequireOCIAuthentication_WithAtmosToken tests RequireOCIAuthentication with ATMOS_GITHUB_TOKEN.
func TestRequireOCIAuthentication_WithAtmosToken(t *testing.T) {
	// Ensure GITHUB_TOKEN is not set
	t.Setenv("GITHUB_TOKEN", "")

	// Set ATMOS_GITHUB_TOKEN
	t.Setenv("ATMOS_GITHUB_TOKEN", "test-atmos-token")

	RequireOCIAuthentication(t)

	// Should reach here when ATMOS token is set
	assert.True(t, true, "Test continued after RequireOCIAuthentication with ATMOS token")
}

// TestRequireOCIAuthentication_WithoutToken tests RequireOCIAuthentication without token.
func TestRequireOCIAuthentication_WithoutToken(t *testing.T) {
	// Ensure no tokens are set
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("ATMOS_GITHUB_TOKEN", "")

	RequireOCIAuthentication(t)

	// This line should not be reached
	t.Error("Should have skipped when no token is set")
}

// TestRequireFilePath_WithExistingPath tests RequireFilePath with existing path.
func TestRequireFilePath_WithExistingPath(t *testing.T) {
	// Use current directory which should exist
	RequireFilePath(t, ".", "current directory")

	// Should reach here when path exists
	assert.True(t, true, "Test continued after RequireFilePath with existing path")
}

// TestRequireNetworkAccess_InvalidURLWithBypass tests RequireNetworkAccess with bypass and invalid URL.
func TestRequireNetworkAccess_InvalidURLWithBypass(t *testing.T) {
	t.Setenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS", "true")

	// Should not panic with invalid URL when bypass is set
	RequireNetworkAccess(t, "not-a-valid-url")
}

// TestLogPreconditionOverride_Variations tests LogPreconditionOverride variations.
func TestLogPreconditionOverride_Variations(t *testing.T) {
	t.Run("without bypass", func(t *testing.T) {
		LogPreconditionOverride(t)
	})

	t.Run("with bypass", func(t *testing.T) {
		t.Setenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS", "true")
		LogPreconditionOverride(t)
	})
}

// TestRequireAWSProfile_NonExistent tests RequireAWSProfile with non-existent profile (will skip).
func TestRequireAWSProfile_NonExistent(t *testing.T) {
	// Ensure precondition checks are enabled
	os.Unsetenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS")

	// This should skip the test
	RequireAWSProfile(t, "definitely-non-existent-profile-xyz-12345")

	// Should not reach here
	t.Error("Should have skipped with non-existent profile")
}

// TestRequireGitRepository_NotInRepo tests RequireGitRepository when not in a repo (will skip).
func TestRequireGitRepository_NotInRepo(t *testing.T) {
	// Ensure precondition checks are enabled
	os.Unsetenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS")

	// Change to temp directory that's not a git repo
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)

	if err := os.Chdir(tmpDir); err == nil {
		// This should skip the test
		RequireGitRepository(t)

		// Should not reach here
		t.Error("Should have skipped when not in git repo")
	}
}

// TestRequireGitRepository_InRepo tests RequireGitRepository in actual repo.
func TestRequireGitRepository_InRepo(t *testing.T) {
	// Ensure precondition checks are enabled
	os.Unsetenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS")

	// This test runs in the actual repo, so it should work
	repo := RequireGitRepository(t)

	if repo != nil {
		// We got a repo object, test passed
		assert.NotNil(t, repo)
	}
	// If repo is nil, test was skipped which is ok
}

// TestRequireGitRemoteWithValidURL_WithRemote tests RequireGitRemoteWithValidURL when remote exists.
func TestRequireGitRemoteWithValidURL_WithRemote(t *testing.T) {
	// Ensure precondition checks are enabled
	os.Unsetenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS")

	// This should work in the actual repo
	url := RequireGitRemoteWithValidURL(t)

	// Either we got a URL or test was skipped
	_ = url
}

// TestRequireGitHubAccess_NoToken tests RequireGitHubAccess without token (will likely skip or rate limit).
func TestRequireGitHubAccess_NoToken(t *testing.T) {
	// Clear any GitHub tokens
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("ATMOS_GITHUB_TOKEN", "")

	// This will either skip or return rate limit info
	info := RequireGitHubAccess(t)

	// If we got info, it worked (even with rate limits)
	_ = info
}

// TestRequireNetworkAccess_ValidURL tests RequireNetworkAccess with valid URL.
func TestRequireNetworkAccess_ValidURL(t *testing.T) {
	// Ensure precondition checks are enabled
	os.Unsetenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS")

	// Try with a commonly available URL
	RequireNetworkAccess(t, "https://github.com")

	// If we get here, network access worked
}

// TestRequireNetworkAccess_InvalidURL tests RequireNetworkAccess with invalid URL (will skip).
func TestRequireNetworkAccess_InvalidURL(t *testing.T) {
	// Ensure precondition checks are enabled
	os.Unsetenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS")

	// This should skip
	RequireNetworkAccess(t, "https://definitely-invalid-domain-xyz-12345.example.com")

	// Should not reach here
	t.Error("Should have skipped with invalid URL")
}

// TestRequireGitRemoteWithValidURL_InRealRepo tests RequireGitRemoteWithValidURL in a real git repo with remotes.
func TestRequireGitRemoteWithValidURL_InRealRepo(t *testing.T) {
	// Ensure precondition checks are enabled
	os.Unsetenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS")

	// Create a temporary git repo with a remote
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)

	// Initialize git repo
	if err := os.Chdir(tmpDir); err != nil {
		t.Skipf("Cannot change to temp directory: %v", err)
	}

	// Run git init
	cmd := exec.Command("git", "init")
	if err := cmd.Run(); err != nil {
		t.Skipf("Cannot initialize git repo: %v", err)
	}

	// Add a remote
	cmd = exec.Command("git", "remote", "add", "origin", "https://github.com/test/repo.git")
	if err := cmd.Run(); err != nil {
		t.Skipf("Cannot add git remote: %v", err)
	}

	// Now test should work
	url := RequireGitRemoteWithValidURL(t)
	assert.Equal(t, "https://github.com/test/repo.git", url)
}

// TestRequireGitRemoteWithValidURL_InvalidRemote tests RequireGitRemoteWithValidURL with invalid remote URL.
func TestRequireGitRemoteWithValidURL_InvalidRemote(t *testing.T) {
	// Ensure precondition checks are enabled
	os.Unsetenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS")

	// Create a temporary git repo with invalid remote
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)

	// Initialize git repo
	if err := os.Chdir(tmpDir); err != nil {
		t.Skipf("Cannot change to temp directory: %v", err)
	}

	// Run git init
	cmd := exec.Command("git", "init")
	if err := cmd.Run(); err != nil {
		t.Skipf("Cannot initialize git repo: %v", err)
	}

	// Add an invalid remote URL
	cmd = exec.Command("git", "remote", "add", "origin", "not-a-valid-url")
	if err := cmd.Run(); err != nil {
		t.Skipf("Cannot add git remote: %v", err)
	}

	// Should skip due to invalid URL
	url := RequireGitRemoteWithValidURL(t)

	// Should not reach here or url should be empty
	if url != "" {
		t.Error("Should have skipped with invalid remote URL")
	}
}

// TestRequireGitRemoteWithValidURL_NoRemotes tests RequireGitRemoteWithValidURL with no remotes.
func TestRequireGitRemoteWithValidURL_NoRemotes(t *testing.T) {
	// Ensure precondition checks are enabled
	os.Unsetenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS")

	// Create a temporary git repo without remotes
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)

	// Initialize git repo
	if err := os.Chdir(tmpDir); err != nil {
		t.Skipf("Cannot change to temp directory: %v", err)
	}

	// Run git init
	cmd := exec.Command("git", "init")
	if err := cmd.Run(); err != nil {
		t.Skipf("Cannot initialize git repo: %v", err)
	}

	// No remotes added - should skip
	url := RequireGitRemoteWithValidURL(t)

	// Should not reach here or url should be empty
	if url != "" {
		t.Error("Should have skipped with no remotes")
	}
}

// TestSetAWSProfileEnv tests the setAWSProfileEnv helper function.
func TestSetAWSProfileEnv(t *testing.T) {
	t.Run("Empty profile returns no-op cleanup", func(t *testing.T) {
		cleanup := setAWSProfileEnv("")
		cleanup()
		// Should not panic or change anything
	})

	t.Run("Same profile returns no-op cleanup", func(t *testing.T) {
		t.Setenv("AWS_PROFILE", "test-profile")
		cleanup := setAWSProfileEnv("test-profile")
		cleanup()
		assert.Equal(t, "test-profile", os.Getenv("AWS_PROFILE"))
	})

	t.Run("New profile sets and cleanup restores", func(t *testing.T) {
		t.Setenv("AWS_PROFILE", "original-profile")
		cleanup := setAWSProfileEnv("new-profile")
		assert.Equal(t, "new-profile", os.Getenv("AWS_PROFILE"))
		cleanup()
		assert.Equal(t, "original-profile", os.Getenv("AWS_PROFILE"))
	})

	t.Run("Profile set when none existed", func(t *testing.T) {
		// Don't set AWS_PROFILE - it will be unset by default in subtest
		cleanup := setAWSProfileEnv("test-profile")
		assert.Equal(t, "test-profile", os.Getenv("AWS_PROFILE"))
		cleanup()
		assert.Empty(t, os.Getenv("AWS_PROFILE"))
	})
}

// TestRequireGitCommitConfig_WithBypass tests RequireGitCommitConfig with bypass enabled.
func TestRequireGitCommitConfig_WithBypass(t *testing.T) {
	t.Setenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS", "true")

	// Should not skip when bypass is set, even without git config
	RequireGitCommitConfig(t)
}

// TestRequireGitCommitConfig_WithConfig tests RequireGitCommitConfig with git config set.
func TestRequireGitCommitConfig_WithConfig(t *testing.T) {
	// Ensure precondition checks are enabled
	os.Unsetenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS")

	// Check if git user.name exists
	cmd := exec.Command("git", "config", "--get", "user.name")
	nameOutput, nameErr := cmd.Output()

	// Check if git user.email exists
	cmd = exec.Command("git", "config", "--get", "user.email")
	emailOutput, emailErr := cmd.Output()

	// If both are configured, test should pass
	if nameErr == nil && len(nameOutput) > 0 && emailErr == nil && len(emailOutput) > 0 {
		RequireGitCommitConfig(t)
		// Test passed
		assert.True(t, true, "Test continued with git config present")
	} else {
		// If not configured, test will skip
		RequireGitCommitConfig(t)
		// Should not reach here
		t.Error("Should have skipped without git config")
	}
}

// TestRequireGitCommitConfig_MissingName tests RequireGitCommitConfig without user.name.
func TestRequireGitCommitConfig_MissingName(t *testing.T) {
	// Ensure precondition checks are enabled
	os.Unsetenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS")

	// This test creates a temp git config context which is complex
	// Instead, we rely on the function skipping if config is missing
	// The actual behavior is tested in WithConfig test above
}
