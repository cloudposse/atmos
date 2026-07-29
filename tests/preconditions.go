//nolint:forbidigo // Test helper package needs os.Getenv/Setenv for precondition checks
package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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

var testToolPathLocks sync.Map

type cachedTestTool struct {
	Repo    string
	Version string
	Binary  string
}

var cachedTestTools = []cachedTestTool{
	{Repo: "opentofu/opentofu", Version: "1.12.2", Binary: "tofu"},
	{Repo: "hashicorp/terraform", Version: "1.15.6", Binary: "terraform"},
	{Repo: "hashicorp/packer", Version: "1.14.2", Binary: "packer"},
	{Repo: "helmfile/helmfile", Version: "v1.1.0", Binary: "helmfile"},
	{Repo: "helm/helm", Version: "v3.19.2", Binary: "helm"},
}

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

// RequireAWSCredentials checks if AWS credentials are available through any standard method.
// It uses the AWS SDK to validate that credentials can be loaded from environment variables,
// shared credentials file, EC2 instance metadata, or any other credential source.
func RequireAWSCredentials(t *testing.T) {
	t.Helper()

	if !ShouldCheckPreconditions() {
		return
	}

	// Try to load the AWS config using the default credential chain
	ctx := context.Background()
	cfgOpts := []func(*config.LoadOptions) error{}
	cfg, err := config.LoadDefaultConfig(ctx, cfgOpts...)
	if err != nil {
		t.Skipf("AWS credentials not available: %v. Configure AWS credentials or set ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true", err)
	}

	// Loading the config succeeds even without credentials, so we need to actually
	// try to retrieve credentials to verify they exist
	_, err = cfg.Credentials.Retrieve(ctx)
	if err != nil {
		t.Skipf("AWS credentials not available: %v. Configure AWS credentials or set ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true", err)
	}
}

// RequireAzureCredentials checks if Azure credentials appear to be configured.
// This function looks for common Azure credential sources:
// - Environment variables (AZURE_TENANT_ID, AZURE_CLIENT_ID, AZURE_CLIENT_SECRET)
// - Azure CLI credentials (az login)
// Note: This does not validate that credentials are valid, only that they are present.
func RequireAzureCredentials(t *testing.T) {
	t.Helper()

	if !ShouldCheckPreconditions() {
		return
	}

	// Check for explicit Azure environment variables first
	tenantID := os.Getenv("AZURE_TENANT_ID")
	clientID := os.Getenv("AZURE_CLIENT_ID")
	clientSecret := os.Getenv("AZURE_CLIENT_SECRET")

	// If all three are set, we have service principal credentials
	if tenantID != "" && clientID != "" && clientSecret != "" {
		t.Logf("Azure credentials available via environment variables (service principal)")
		return
	}

	// Check if Azure CLI is configured
	cmd := exec.Command("az", "account", "show")
	output, err := cmd.Output()
	if err == nil && len(output) > 0 {
		t.Logf("Azure credentials available via Azure CLI (az login)")
		return
	}

	// No credentials found, skip the test
	t.Skipf("Azure credentials not available. Configure via: " +
		"(1) Environment variables: AZURE_TENANT_ID, AZURE_CLIENT_ID, AZURE_CLIENT_SECRET, " +
		"(2) Azure CLI: run 'az login', " +
		"(3) Set ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true to bypass")
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
// Unlike requireExecutablePath, a bypass (ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true)
// is a full no-op here: callers that only need a boolean gate, not the resolved
// path, don't need the tool to actually be resolvable when checks are disabled.
func RequireExecutable(t *testing.T, name string, purpose string) {
	t.Helper()

	prependCachedTestTool(name)

	if !ShouldCheckPreconditions() {
		return
	}

	if _, err := exec.LookPath(name); err != nil {
		t.Skipf("'%s' not found in PATH: required for %s. Install the tool or set ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true",
			name, purpose)
	}
}

// requireExecutablePath resolves and returns the executable's path, skipping
// the test if it is not found. Callers that need the resolved path (to, e.g.,
// copy the binary into an isolated per-test directory) should use this instead
// of a separate, independent exec.LookPath call afterward. Two independent
// LookPath calls create a TOCTOU race: the Windows acceptance job runs
// packages concurrently, and another package's test can mutate the shared
// Atmos toolchain cache directory (see prependCachedTestTool) between the two
// calls, turning a binary that existed a moment ago into a spurious
// "not found" error. Resolving the path exactly once here closes that window.
// These constants bound how long requireExecutablePath polls exec.LookPath
// before giving up. CI installs each toolchain binary and updates PATH in an
// earlier job step; on some runners (observed on Windows) that update has
// occasionally not been visible yet to the very first lookup here, so a poll
// avoids failing on a one-off resolution lag rather than the tool actually
// being absent. 15s (rather than a token couple of seconds) is deliberate:
// the Windows acceptance job runs many Go packages' test binaries
// concurrently on constrained runners, and this exact race recurred in CI
// even after closing one confirmed root cause (a test writing real installs
// into the shared toolchain cache dir, see the pkg/toolchain fix referenced
// in this repo's git history) -- so a short window risks masking further,
// not-yet-identified contention rather than tolerating genuine scheduling
// delay under load.
const (
	executablePathRetryTimeout  = 15 * time.Second
	executablePathRetryInterval = 100 * time.Millisecond
)

func requireExecutablePath(t *testing.T, name string, purpose string) string {
	t.Helper()

	prependCachedTestTool(name)

	deadline := time.Now().Add(executablePathRetryTimeout)
	var path string
	var err error
	for {
		path, err = exec.LookPath(name)
		if err == nil || time.Now().After(deadline) {
			break
		}
		time.Sleep(executablePathRetryInterval)
	}
	if err != nil {
		forensics := executableLookupForensics(name)
		if !ShouldCheckPreconditions() {
			// ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true (set for every CI
			// acceptance job) means the caller wants a hard failure instead of
			// a skip when a tool that CI is expected to provision is missing --
			// never silently hand back an empty path, which turns into a far
			// more confusing "open : file not found" error downstream.
			t.Fatalf("'%s' not found in PATH: required for %s. Precondition checks are disabled (ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true), so this is a hard failure rather than a skip: %v\n%s",
				name, purpose, err, forensics)
		}
		t.Skipf("'%s' not found in PATH: required for %s. Install the tool or set ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true\n%s",
			name, purpose, forensics)
	}
	return path
}

// executableLookupForensics reports the on-disk state of every toolchain-like
// PATH entry at the moment a binary lookup failed. The Windows acceptance job
// has a recurring "installed tool vanished mid-suite" failure whose root cause
// is still being pinned down (writer-side install/uninstall windows in the
// shared toolchain cache are the leading theory); this dump turns the next CI
// failure into direct evidence of WHICH mechanism fired: directory deleted
// (uninstall/reinstall), tree renamed aside (onedir .bak present), writer
// mid-flight (.lock sibling present), or PATH itself corrupted (no toolchain
// entries at all).
func executableLookupForensics(name string) string {
	var b strings.Builder
	entries := filepath.SplitList(os.Getenv("PATH"))
	fmt.Fprintf(&b, "-- lookup forensics for %q (%d PATH entries) --\n", name, len(entries))

	candidates := []string{name}
	if withExt := cachedTestToolBinaryNameForOS(name, runtime.GOOS); withExt != name {
		candidates = append(candidates, withExt)
	}

	toolchainEntries := 0
	for _, dir := range entries {
		if !strings.Contains(strings.ToLower(dir), "toolchain") {
			continue
		}
		toolchainEntries++
		describeToolchainPathEntry(&b, dir, candidates)
	}
	if toolchainEntries == 0 {
		b.WriteString("NO toolchain-like PATH entries found -- PATH itself lost the toolchain dirs\n")
	}
	return b.String()
}

// describeToolchainPathEntry appends one PATH entry's on-disk state (target
// presence, directory contents, writer-lock sibling) to the forensics report.
func describeToolchainPathEntry(b *strings.Builder, dir string, candidates []string) {
	present := false
	for _, cand := range candidates {
		// #nosec G703 -- dir comes from this process's own PATH and cand from a fixed test-tool list; read-only stat for diagnostics.
		if st, statErr := os.Stat(filepath.Join(dir, cand)); statErr == nil && !st.IsDir() {
			present = true
			break
		}
	}
	fmt.Fprintf(b, "%s -> target present=%v", dir, present)
	if dirEntries, readErr := os.ReadDir(dir); readErr == nil {
		names := make([]string, 0, len(dirEntries))
		for _, e := range dirEntries {
			names = append(names, e.Name())
		}
		fmt.Fprintf(b, "; contents(%d): %s", len(names), strings.Join(names, ", "))
	} else {
		fmt.Fprintf(b, "; dir unreadable: %v", readErr)
	}
	// The installer's writer lock is a ".lock" sibling of the version dir
	// (see pkg/toolchain/installer); its presence means a writer was active.
	// #nosec G703 -- dir comes from this process's own PATH; read-only stat for diagnostics.
	if _, lockErr := os.Stat(dir + ".lock"); lockErr == nil {
		b.WriteString("; WRITER LOCK PRESENT")
	}
	b.WriteString("\n")
}

func prependCachedTestTool(binary string) {
	tool, ok := cachedTestToolForBinary(binary)
	if !ok {
		return
	}

	lockValue, _ := testToolPathLocks.LoadOrStore(binary, &sync.Once{})
	lockValue.(*sync.Once).Do(func() {
		cacheDir, err := os.UserCacheDir()
		if err != nil {
			return
		}

		// Unit tests provision tools into test-toolchain, while CI uses the normal
		// Atmos toolchain cache. Check both so package-local integration tests can
		// use the tools installed by the acceptance workflow on every platform.
		for _, binDir := range cachedTestToolBinDirs(cacheDir, tool) {
			if _, ok := cachedTestToolBinaryPath(binDir, tool.Binary); !ok {
				continue
			}

			path := os.Getenv("PATH")
			if path == "" {
				os.Setenv("PATH", binDir)
				return
			}
			os.Setenv("PATH", binDir+string(os.PathListSeparator)+path)
			return
		}
	})
}

func cachedTestToolBinDirs(cacheDir string, tool cachedTestTool) []string {
	toolPath := filepath.Join(filepath.FromSlash(tool.Repo), tool.Version)
	return []string{
		filepath.Join(cacheDir, "atmos", "test-toolchain", "bin", toolPath),
		filepath.Join(cacheDir, "atmos", "toolchain", "bin", toolPath),
	}
}

// cachedTestToolBinaryPath returns the installed test-tool binary path, including
// the .exe artifact generated by the Windows toolchain.
func cachedTestToolBinaryPath(binDir string, binary string) (string, bool) {
	candidates := []string{filepath.Join(binDir, binary)}
	if filepath.Ext(binary) == "" {
		candidates = append(candidates, filepath.Join(binDir, binary+".exe"))
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, true
		}
	}

	return "", false
}

// cachedTestToolBinaryName returns the expected on-disk binary name for the
// current OS. Used by test fixture setup, which needs the exact deterministic
// name a real install would produce -- unlike cachedTestToolBinaryPath, which
// tries multiple candidates when looking up an existing cached binary.
func cachedTestToolBinaryName(binary string) string {
	return cachedTestToolBinaryNameForOS(binary, runtime.GOOS)
}

func cachedTestToolBinaryNameForOS(binary, goos string) string {
	if goos == "windows" {
		return binary + ".exe"
	}
	return binary
}

func cachedTestToolBinaryExists(binDir, binary string) bool {
	_, err := os.Stat(filepath.Join(binDir, cachedTestToolBinaryName(binary)))
	return err == nil
}

func cachedTestToolForBinary(binary string) (cachedTestTool, bool) {
	for _, tool := range cachedTestTools {
		if tool.Binary == binary {
			return tool, true
		}
	}
	return cachedTestTool{}, false
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

// RequireTerraformPath is like RequireTerraform but also returns the resolved
// terraform binary path. Prefer this over calling RequireTerraform followed by
// a separate exec.LookPath("terraform") -- see requireExecutablePath's doc
// comment for why two independent lookups are unsafe.
func RequireTerraformPath(t *testing.T) string {
	t.Helper()
	return requireExecutablePath(t, "terraform", "terraform operations")
}

// RequireTofu checks if tofu (OpenTofu) is installed and available in PATH.
// The CLI test suite standardizes on OpenTofu (see ATMOS_COMPONENTS_TERRAFORM_COMMAND
// in cli_test.go), so terraform-invoking tests gate on this rather than terraform.
func RequireTofu(t *testing.T) {
	t.Helper()
	RequireExecutable(t, "tofu", "OpenTofu operations")
}

// RequireTofuPath is like RequireTofu but also returns the resolved tofu
// binary path. Prefer this over calling RequireTofu followed by a separate
// exec.LookPath("tofu") -- see requireExecutablePath's doc comment for why two
// independent lookups are unsafe.
func RequireTofuPath(t *testing.T) string {
	t.Helper()
	return requireExecutablePath(t, "tofu", "OpenTofu operations")
}

// RequireTerraformOrTofu checks if terraform or tofu is installed and available in PATH.
// Use this for tests whose fixture atmos.yaml sets command: tofu, or that work with either binary.
func RequireTerraformOrTofu(t *testing.T) {
	t.Helper()

	if !ShouldCheckPreconditions() {
		return
	}

	_, terraformErr := exec.LookPath("terraform")
	_, tofuErr := exec.LookPath("tofu")
	if terraformErr != nil && tofuErr != nil {
		t.Skipf("Neither 'terraform' nor 'tofu' found in PATH: required for terraform operations. Install one or set ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true")
	}
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

	// Token exists — probe ghcr.io to verify this specific token can actually pull images.
	// A bot token may exist but not have access to the ghcr.io registry used by tests.
	client := &http.Client{Timeout: httpTimeout}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://ghcr.io/v2/", nil)
	if err != nil {
		t.Logf("Warning: Could not create ghcr.io request: %v", err)
	} else {
		req.Header.Set("Authorization", "Bearer "+githubToken)
		resp, reqErr := client.Do(req) //nolint:gosec // URL is a hardcoded constant (ghcr.io), not user input.
		if reqErr != nil {
			t.Skipf("Cannot reach ghcr.io: %v. OCI registry access required. Set ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true to skip.", reqErr)
		}
		resp.Body.Close()
		// 401 = unauthorized (wrong/expired token); 403 = forbidden (no read:packages scope).
		// Both indicate the token cannot pull OCI images from ghcr.io.
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			t.Skipf("GitHub token exists but is not authorized for ghcr.io (HTTP %d). OCI integration test requires a token with 'read:packages' scope.", resp.StatusCode)
		}
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

const (
	// Container runtime names.
	containerRuntimeDocker = "docker"
	containerRuntimePodman = "podman"
)

// RequireContainerRuntime checks if a container runtime (Docker or Podman) is available.
// It prefers Docker but will accept Podman if Docker is not available.
// Returns the name of the available runtime ("docker" or "podman").
func RequireContainerRuntime(t *testing.T) string {
	t.Helper()

	if !ShouldCheckPreconditions() {
		return containerRuntimeDocker // Default assumption when checks are disabled
	}

	// Try Docker first
	if cmd := exec.Command(containerRuntimeDocker, "version"); cmd.Run() == nil {
		t.Logf("Container runtime available: Docker")
		return containerRuntimeDocker
	}

	// Try Podman as fallback
	if cmd := exec.Command(containerRuntimePodman, "version"); cmd.Run() == nil {
		t.Logf("Container runtime available: Podman")
		return containerRuntimePodman
	}

	t.Skipf("No container runtime available. Install Docker (https://docs.docker.com/get-docker/) or Podman (https://podman.io/getting-started/installation), or set ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true")
	return ""
}

// RequireDocker checks if Docker is available and running.
// Use this for tests that specifically require Docker (not Podman).
func RequireDocker(t *testing.T) {
	t.Helper()

	if !ShouldCheckPreconditions() {
		return
	}

	// Check if docker command exists
	_, err := exec.LookPath(containerRuntimeDocker)
	if err != nil {
		t.Skipf("Docker not found in PATH. Install Docker (https://docs.docker.com/get-docker/) or set ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true")
	}

	// Check if Docker daemon is running
	cmd := exec.Command(containerRuntimeDocker, "info")
	if err := cmd.Run(); err != nil {
		t.Skipf("Docker daemon not running. Start Docker or set ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true")
	}

	t.Logf("Docker is available and running")
}

// RequirePodman checks if Podman is available and running.
// Use this for tests that specifically require Podman (not Docker).
func RequirePodman(t *testing.T) {
	t.Helper()

	if !ShouldCheckPreconditions() {
		return
	}

	// Check if podman command exists
	_, err := exec.LookPath(containerRuntimePodman)
	if err != nil {
		t.Skipf("Podman not found in PATH. Install Podman (https://podman.io/getting-started/installation) or set ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true")
	}

	// Check if Podman is working
	cmd := exec.Command(containerRuntimePodman, "info")
	if err := cmd.Run(); err != nil {
		t.Skipf("Podman not working properly. Check Podman installation or set ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true")
	}

	t.Logf("Podman is available and working")
}

// terraformRegistryErrorSignatures are substrings emitted by Terraform/OpenTofu when a
// provider install fails because of a transient registry problem (HTTP 429 rate limiting,
// registry unreachable) rather than a defect in the code under test.
var terraformRegistryErrorSignatures = []string{
	"Failed to install provider",
	"Failed to query available provider packages",
	"could not query provider registry",
	"could not connect to registry.terraform.io",
	"lookup registry.terraform.io",
	"Too Many Requests",
}

// SkipIfTerraformRegistryError skips the test when the captured Terraform/OpenTofu output
// shows a transient provider-registry failure (for example, an HTTP 429 rate limit from
// registry.terraform.io). These failures are environmental, not regressions, so the repo
// treats them as skips, mirroring the Packer init network-failure handling. Pass the
// captured command output (stdout/stderr) collected while running the Terraform subcommand.
func SkipIfTerraformRegistryError(t *testing.T, output string) {
	t.Helper()

	for _, sig := range terraformRegistryErrorSignatures {
		if strings.Contains(output, sig) {
			t.Skipf("Skipping: transient Terraform provider registry error detected in output (%q)", sig)
		}
	}
}
