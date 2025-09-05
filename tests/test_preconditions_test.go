//nolint:forbidigo // Test files need os.Getenv/Setenv for testing
package tests

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Test ShouldCheckPreconditions
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
			// Save original value
			orig := os.Getenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS")
			defer func() {
				if orig != "" {
					os.Setenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS", orig)
				} else {
					os.Unsetenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS")
				}
			}()

			if tt.envValue != "" {
				os.Setenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS", tt.envValue)
			} else {
				os.Unsetenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS")
			}

			got := ShouldCheckPreconditions()
			assert.Equal(t, tt.want, got)
		})
	}
}

// Test RequireAWSProfile with bypass
func TestRequireAWSProfile_WithBypass(t *testing.T) {
	// Set bypass
	os.Setenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS", "true")
	defer os.Unsetenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS")

	// Should not skip when bypass is set
	RequireAWSProfile(t, "non-existent-profile-that-does-not-exist-12345")
	// If we get here, it worked (didn't skip)
}

// Test RequireGitRepository with bypass
func TestRequireGitRepository_WithBypass(t *testing.T) {
	os.Setenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS", "true")
	defer os.Unsetenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS")

	repo := RequireGitRepository(t)
	assert.Nil(t, repo) // Should return nil when bypassed
}

// Test RequireGitRemoteWithValidURL with bypass
func TestRequireGitRemoteWithValidURL_WithBypass(t *testing.T) {
	os.Setenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS", "true")
	defer os.Unsetenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS")

	url := RequireGitRemoteWithValidURL(t)
	assert.Empty(t, url) // Should return empty when bypassed
}

// Test RequireNetworkAccess with bypass
func TestRequireNetworkAccess_WithBypass(t *testing.T) {
	os.Setenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS", "true")
	defer os.Unsetenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS")

	// Should not skip when bypass is set
	RequireNetworkAccess(t, "http://invalid-domain-that-does-not-exist-12345.example.com")
}

// Test RequireExecutable with bypass
func TestRequireExecutable_WithBypass(t *testing.T) {
	os.Setenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS", "true")
	defer os.Unsetenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS")

	RequireExecutable(t, "non-existent-binary-that-does-not-exist-12345", "testing")
}

// Test RequireEnvVar with bypass
func TestRequireEnvVar_WithBypass(t *testing.T) {
	os.Setenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS", "true")
	defer os.Unsetenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS")

	RequireEnvVar(t, "NON_EXISTENT_VAR_THAT_DOES_NOT_EXIST_12345", "testing")
}

// Test RequireFilePath with bypass
func TestRequireFilePath_WithBypass(t *testing.T) {
	os.Setenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS", "true")
	defer os.Unsetenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS")

	RequireFilePath(t, "/non/existent/path/that/does/not/exist/12345", "testing")
}

// Test RequireOCIAuthentication with bypass
func TestRequireOCIAuthentication_WithBypass(t *testing.T) {
	os.Setenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS", "true")
	defer os.Unsetenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS")

	// Ensure no token is set
	origGH := os.Getenv("GITHUB_TOKEN")
	origAtmos := os.Getenv("ATMOS_GITHUB_TOKEN")
	defer func() {
		if origGH != "" {
			os.Setenv("GITHUB_TOKEN", origGH)
		} else {
			os.Unsetenv("GITHUB_TOKEN")
		}
		if origAtmos != "" {
			os.Setenv("ATMOS_GITHUB_TOKEN", origAtmos)
		} else {
			os.Unsetenv("ATMOS_GITHUB_TOKEN")
		}
	}()
	os.Unsetenv("GITHUB_TOKEN")
	os.Unsetenv("ATMOS_GITHUB_TOKEN")

	RequireOCIAuthentication(t)
}

// Test LogPreconditionOverride
func TestLogPreconditionOverride(t *testing.T) {
	// Test with bypass enabled
	os.Setenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS", "true")
	defer os.Unsetenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS")

	// Should log a message (we can't easily test the log output, but ensure no panic)
	LogPreconditionOverride(t)

	// Test without bypass
	os.Unsetenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS")
	LogPreconditionOverride(t)
}

// Test RequireGitHubAccess with bypass
func TestRequireGitHubAccess_WithBypass(t *testing.T) {
	os.Setenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS", "true")
	defer os.Unsetenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS")

	info := RequireGitHubAccess(t)
	assert.Nil(t, info) // Should return nil when bypassed
}

// Test real skip scenarios - these will actually skip the subtest
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

// Test RequireEnvVar with existing variable
func TestRequireEnvVar_WithExistingVar(t *testing.T) {
	// Set a test env var
	os.Setenv("TEST_VAR_FOR_TESTING", "some_value")
	defer os.Unsetenv("TEST_VAR_FOR_TESTING")

	RequireEnvVar(t, "TEST_VAR_FOR_TESTING", "testing")

	// Should reach here when var exists
	assert.True(t, true, "Test continued after RequireEnvVar with existing var")
}

// Test RequireExecutable with existing executable
func TestRequireExecutable_WithExistingBinary(t *testing.T) {
	// Use 'go' as it should exist in test environment
	RequireExecutable(t, "go", "testing")

	// Should reach here when executable exists
	assert.True(t, true, "Test continued after RequireExecutable with existing binary")
}

// Test RequireOCIAuthentication with token set
func TestRequireOCIAuthentication_WithToken(t *testing.T) {
	// Set a GitHub token
	os.Setenv("GITHUB_TOKEN", "test-token")
	defer os.Unsetenv("GITHUB_TOKEN")

	RequireOCIAuthentication(t)

	// Should reach here when token is set
	assert.True(t, true, "Test continued after RequireOCIAuthentication with token")
}

// Test RequireOCIAuthentication with ATMOS_GITHUB_TOKEN
func TestRequireOCIAuthentication_WithAtmosToken(t *testing.T) {
	// Ensure GITHUB_TOKEN is not set
	origGH := os.Getenv("GITHUB_TOKEN")
	os.Unsetenv("GITHUB_TOKEN")
	defer func() {
		if origGH != "" {
			os.Setenv("GITHUB_TOKEN", origGH)
		}
	}()

	// Set ATMOS_GITHUB_TOKEN
	os.Setenv("ATMOS_GITHUB_TOKEN", "test-atmos-token")
	defer os.Unsetenv("ATMOS_GITHUB_TOKEN")

	RequireOCIAuthentication(t)

	// Should reach here when ATMOS token is set
	assert.True(t, true, "Test continued after RequireOCIAuthentication with ATMOS token")
}

// Test RequireOCIAuthentication without token
func TestRequireOCIAuthentication_WithoutToken(t *testing.T) {
	// Ensure no tokens are set
	origGH := os.Getenv("GITHUB_TOKEN")
	origAtmos := os.Getenv("ATMOS_GITHUB_TOKEN")
	os.Unsetenv("GITHUB_TOKEN")
	os.Unsetenv("ATMOS_GITHUB_TOKEN")
	defer func() {
		if origGH != "" {
			os.Setenv("GITHUB_TOKEN", origGH)
		}
		if origAtmos != "" {
			os.Setenv("ATMOS_GITHUB_TOKEN", origAtmos)
		}
	}()

	RequireOCIAuthentication(t)

	// This line should not be reached
	t.Error("Should have skipped when no token is set")
}

// Test RequireFilePath with existing path
func TestRequireFilePath_WithExistingPath(t *testing.T) {
	// Use current directory which should exist
	RequireFilePath(t, ".", "current directory")
	
	// Should reach here when path exists
	assert.True(t, true, "Test continued after RequireFilePath with existing path")
}

// Test RequireNetworkAccess with bypass and invalid URL
func TestRequireNetworkAccess_InvalidURL(t *testing.T) {
	os.Setenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS", "true")
	defer os.Unsetenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS")
	
	// Should not panic with invalid URL when bypass is set
	RequireNetworkAccess(t, "not-a-valid-url")
}

// Test LogPreconditionOverride variations
func TestLogPreconditionOverride_Variations(t *testing.T) {
	// Test without bypass first
	os.Unsetenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS")
	LogPreconditionOverride(t)
	
	// Test with bypass
	os.Setenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS", "true")
	defer os.Unsetenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS")
	LogPreconditionOverride(t)
}