//nolint:forbidigo // Test helper package needs os.Getenv/Setenv for precondition checks
package tests

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/go-git/go-git/v5"
	giturl "github.com/kubescape/go-git-url"
)

// Constants for network and API limits.
const (
	// HttpTimeout is the timeout for HTTP requests in seconds.
	httpTimeout = 5 * time.Second
	// HttpErrorStatusThreshold is the minimum HTTP status code considered an error.
	httpErrorStatusThreshold = 400
	// HttpOKStatus is the HTTP status code for successful requests.
	httpOKStatus = 200
	// GithubAPIRateLimitWarning is the threshold for warning about low GitHub API rate limits.
	githubAPIRateLimitWarning = 10
	// EnvAWSProfile is the environment variable for AWS profile.
	envAWSProfile = "AWS_PROFILE"
)

// ShouldCheckPreconditions returns true if precondition checks should be performed.
// Set ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true to bypass all precondition checks.
func ShouldCheckPreconditions() bool {
	return os.Getenv("ATMOS_TEST_SKIP_PRECONDITION_CHECKS") != "true"
}

// setAWSProfileEnv temporarily sets the AWS_PROFILE environment variable.
func setAWSProfileEnv(profileName string) func() {
	if profileName == "" {
		return func() {}
	}

	currentProfile := os.Getenv(envAWSProfile)
	if currentProfile == profileName {
		return func() {}
	}

	// Set the new profile
	oldProfile := os.Getenv(envAWSProfile)
	os.Setenv(envAWSProfile, profileName)

	// Return cleanup function
	return func() {
		if oldProfile != "" {
			os.Setenv(envAWSProfile, oldProfile)
		} else {
			os.Unsetenv(envAWSProfile)
		}
	}
}

// RequireAWSProfile checks if AWS can be configured with the given profile.
// It uses the AWS SDK to validate that the profile can be loaded.
func RequireAWSProfile(t *testing.T, profileName string) {
	t.Helper()

	if !ShouldCheckPreconditions() {
		return
	}

	// Set the profile if needed and defer cleanup
	cleanup := setAWSProfileEnv(profileName)
	defer cleanup()

	// Try to load the AWS config
	ctx := context.Background()
	cfgOpts := []func(*config.LoadOptions) error{}
	_, err := config.LoadDefaultConfig(ctx, cfgOpts...)
	if err != nil {
		t.Skipf("AWS profile '%s' not available: %v. Configure AWS credentials or set ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true", profileName, err)
	}
}

// RequireGitRepository checks if we're in a valid Git repository.
func RequireGitRepository(t *testing.T) *git.Repository {
	t.Helper()

	if !ShouldCheckPreconditions() {
		// Return nil - tests should handle this
		return nil
	}

	// Use PlainOpenWithOptions with DetectDotGit to find the repository
	// and EnableDotGitCommonDir to properly handle worktrees
	repo, err := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{
		DetectDotGit:          true,
		EnableDotGitCommonDir: true, // Critical for worktree support
	})
	if err != nil {
		t.Skipf("Not in a Git repository: %v. Initialize a Git repo or set ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true", err)
	}

	return repo
}

// RequireGitRemoteWithValidURL checks for valid Git remotes that can be parsed.
func RequireGitRemoteWithValidURL(t *testing.T) string {
	t.Helper()

	if !ShouldCheckPreconditions() {
		return ""
	}

	repo := RequireGitRepository(t)
	if repo == nil {
		return ""
	}

	config, err := repo.Config()
	if err != nil {
		t.Skipf("Cannot read Git config: %v. Check Git repository or set ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true", err)
	}

	// Find a valid remote URL
	var repoUrl string
	for _, remote := range config.Remotes {
		if len(remote.URLs) > 0 && remote.URLs[0] != "" {
			repoUrl = remote.URLs[0]
			// Try to parse it
			_, err := giturl.NewGitURL(repoUrl)
			if err == nil {
				return repoUrl
			}
		}
	}

	t.Skipf("No valid parseable Git remote URL found. Add a remote or set ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true")
	return ""
}

// GitHubRateLimitInfo contains GitHub API rate limit information.
type GitHubRateLimitInfo struct {
	Limit     int
	Remaining int
	Reset     time.Time
}

// checkGitHubRateLimit checks GitHub API rate limits and handles the response.
func checkGitHubRateLimit(t *testing.T, client *http.Client) *GitHubRateLimitInfo {
	t.Helper()

	apiResp, err := client.Get("https://api.github.com/rate_limit")
	if err != nil {
		t.Logf("Warning: Cannot check GitHub API rate limits: %v", err)
		return nil
	}
	defer apiResp.Body.Close()

	if apiResp.StatusCode != httpOKStatus {
		return nil
	}

	var rateLimitResponse struct {
		Rate struct {
			Limit     int   `json:"limit"`
			Remaining int   `json:"remaining"`
			Reset     int64 `json:"reset"`
		} `json:"rate"`
	}

	body, err := io.ReadAll(apiResp.Body)
	if err != nil {
		return nil
	}

	err = json.Unmarshal(body, &rateLimitResponse)
	if err != nil {
		return nil
	}

	info := &GitHubRateLimitInfo{
		Limit:     rateLimitResponse.Rate.Limit,
		Remaining: rateLimitResponse.Rate.Remaining,
		Reset:     time.Unix(rateLimitResponse.Rate.Reset, 0),
	}

	// Skip if rate limited
	if info.Remaining == 0 {
		waitTime := time.Until(info.Reset)
		t.Skipf("GitHub API rate limit exceeded. Resets at %s (in %v). Use authenticated requests or set ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true",
			info.Reset.Format(time.RFC3339), waitTime)
	}

	// Warn if getting close to limit
	if info.Remaining < githubAPIRateLimitWarning {
		t.Logf("Warning: Only %d GitHub API requests remaining (resets at %s)",
			info.Remaining, info.Reset.Format(time.RFC3339))
	}

	return info
}

// RequireGitHubAccess checks network connectivity and rate limits for GitHub.
func RequireGitHubAccess(t *testing.T) *GitHubRateLimitInfo {
	t.Helper()

	if !ShouldCheckPreconditions() {
		return nil
	}

	client := &http.Client{
		Timeout: httpTimeout,
	}

	// First check basic connectivity
	resp, err := client.Head("https://github.com")
	if err != nil {
		t.Skipf("Cannot reach github.com: %v. Check network connection or set ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true", err)
	}
	resp.Body.Close()

	if resp.StatusCode >= httpErrorStatusThreshold {
		t.Skipf("GitHub returned status %d. Check service status or set ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true", resp.StatusCode)
	}

	// Check API rate limits
	return checkGitHubRateLimit(t, client)
}

// RequireNetworkAccess checks general network connectivity to a URL.
func RequireNetworkAccess(t *testing.T, url string) {
	t.Helper()

	if !ShouldCheckPreconditions() {
		return
	}

	client := &http.Client{
		Timeout: httpTimeout,
	}

	resp, err := client.Head(url)
	if err != nil {
		t.Skipf("Cannot reach %s: %v. Check network connection or set ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= httpErrorStatusThreshold {
		t.Skipf("%s returned status %d. Check service availability or set ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true",
			url, resp.StatusCode)
	}
}

// RequireExecutable checks if an executable is available in PATH.
func RequireExecutable(t *testing.T, name string, purpose string) {
	t.Helper()

	if !ShouldCheckPreconditions() {
		return
	}

	_, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("'%s' not found in PATH: required for %s. Install the tool or set ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true",
			name, purpose)
	}
}

// RequireEnvVar checks if an environment variable is set.
func RequireEnvVar(t *testing.T, name string, purpose string) {
	t.Helper()

	if !ShouldCheckPreconditions() {
		return
	}

	value := os.Getenv(name)
	if value == "" {
		t.Skipf("Environment variable '%s' not set: required for %s. Set the variable or set ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true",
			name, purpose)
	}
}

// RequireFilePath checks if a file or directory exists.
func RequireFilePath(t *testing.T, path string, purpose string) {
	t.Helper()

	if !ShouldCheckPreconditions() {
		return
	}

	_, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skipf("Path '%s' does not exist: required for %s. Create the file/directory or set ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true",
				path, purpose)
		}
		t.Skipf("Cannot access path '%s': %v. Check permissions or set ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true",
			path, err)
	}
}

// LogPreconditionOverride logs when precondition checks are disabled.
func LogPreconditionOverride(t *testing.T) {
	t.Helper()

	if !ShouldCheckPreconditions() {
		t.Logf("Note: Precondition checks are disabled (ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true)")
	}
}

// RequireTerraform checks if terraform is installed and available in PATH.
// This is a convenience function that uses RequireExecutable specifically for terraform.
func RequireTerraform(t *testing.T) {
	t.Helper()
	RequireExecutable(t, "terraform", "terraform operations")
}

// RequirePacker checks if packer is installed and available in PATH.
// This is a convenience function that uses RequireExecutable specifically for packer.
func RequirePacker(t *testing.T) {
	t.Helper()
	RequireExecutable(t, "packer", "packer operations")
}

// RequireHelmfile checks if helmfile is installed and available in PATH.
// This is a convenience function that uses RequireExecutable specifically for helmfile.
func RequireHelmfile(t *testing.T) {
	t.Helper()
	RequireExecutable(t, "helmfile", "helmfile operations")
}

// RequireOCIAuthentication checks if authentication is configured for GitHub API access.
// This is required for pulling OCI images from ghcr.io, cloning from github.com, and avoiding GitHub API rate limits.
// This is typically provided by GITHUB_TOKEN or ATMOS_GITHUB_TOKEN environment variables.
func RequireOCIAuthentication(t *testing.T) {
	t.Helper()

	if !ShouldCheckPreconditions() {
		return
	}

	// Check for GitHub token in various standard locations
	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken == "" {
		githubToken = os.Getenv("ATMOS_GITHUB_TOKEN")
	}

	if githubToken == "" {
		t.Skipf("GitHub token not configured: required for GitHub API access (OCI images, cloning repos, avoiding rate limits). Set GITHUB_TOKEN or ATMOS_GITHUB_TOKEN environment variable, or set ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true")
	}

	// Token exists, log that authentication is available
	t.Logf("GitHub authentication available via token")
}

// RequireGitCommitConfig checks if Git is configured for making commits.
// This checks for user.name and user.email configuration which are required for commits.
func RequireGitCommitConfig(t *testing.T) {
	t.Helper()

	if !ShouldCheckPreconditions() {
		return
	}

	// Check for git user.name
	cmd := exec.Command("git", "config", "--get", "user.name")
	output, err := cmd.Output()
	if err != nil || len(output) == 0 {
		t.Skipf("Git user.name not configured: required for creating commits. Run 'git config user.name \"Your Name\"' or set ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true")
	}

	// Check for git user.email
	cmd = exec.Command("git", "config", "--get", "user.email")
	output, err = cmd.Output()
	if err != nil || len(output) == 0 {
		t.Skipf("Git user.email not configured: required for creating commits. Run 'git config user.email \"your.email@example.com\"' or set ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true")
	}

	t.Logf("Git commit configuration available")
}

// SkipIfShort skips the test if running in short mode (go test -short).
// Use this for tests that take more than 2 seconds (network I/O, heavy processing, Git operations, etc.).
func SkipIfShort(t *testing.T) {
	t.Helper()

	if testing.Short() {
		t.Skipf("Skipping long-running test in short mode (use 'go test' without -short to run)")
	}
}

// SkipOnDarwinARM64 skips the test if running on darwin/arm64 (macOS ARM).
// Use this for tests that are incompatible with ARM64 macOS, such as tests using gomonkey
// which causes fatal SIGBUS errors due to memory protection on ARM64.
func SkipOnDarwinARM64(t *testing.T, reason string) {
	t.Helper()

	if !ShouldCheckPreconditions() {
		return
	}

	// Check if we're on darwin/arm64
	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		t.Skipf("Skipping on darwin/arm64: %s. Set ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true to override", reason)
	}
}
