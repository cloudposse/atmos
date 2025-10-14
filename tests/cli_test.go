package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath" // For resolving absolute paths
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/creack/pty"
	"github.com/go-git/go-git/v5"
	"github.com/hexops/gotextdiff"
	"github.com/hexops/gotextdiff/myers"
	"github.com/hexops/gotextdiff/span"
	"github.com/muesli/termenv"
	"github.com/otiai10/copy"
	"github.com/sergi/go-diff/diffmatchpatch"
	"github.com/stretchr/testify/assert"
	"go.yaml.in/yaml/v3"
	"golang.org/x/term"

	"github.com/adrg/xdg"
	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/telemetry"
	"github.com/cloudposse/atmos/tests/testhelpers"
)

// Command-line flag for regenerating snapshots.
var (
	regenerateSnapshots = flag.Bool("regenerate-snapshots", false, "Regenerate all golden snapshots")
	startingDir         string
	snapshotBaseDir     string
	repoRoot            string                   // Repository root directory for path normalization
	skipReason          string                   // Package-level variable to track why tests should be skipped
	atmosRunner         *testhelpers.AtmosRunner // Global runner for executing Atmos with coverage support (lazy initialized)
	coverDir            string                   // GOCOVERDIR environment variable value
	sandboxRegistry     = make(map[string]*testhelpers.SandboxEnvironment)
	sandboxMutex        sync.RWMutex
)

// Define styles using lipgloss.
var (
	addedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))  // Green
	removedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("160")) // Red
)
var logger *log.AtmosLogger

type Expectation struct {
	Stdout        []MatchPattern            `yaml:"stdout"`          // Expected stdout output (non-TTY mode)
	Stderr        []MatchPattern            `yaml:"stderr"`          // Expected stderr output (non-TTY mode)
	Tty           []MatchPattern            `yaml:"tty"`             // Expected TTY output (TTY mode - combined stdout+stderr)
	ExitCode      int                       `yaml:"exit_code"`       // Expected exit code
	FileExists    []string                  `yaml:"file_exists"`     // Files to validate
	FileNotExists []string                  `yaml:"file_not_exists"` // Files that should not exist
	FileContains  map[string][]MatchPattern `yaml:"file_contains"`   // File contents to validate (file to patterns map)
	Diff          []string                  `yaml:"diff"`            // Acceptable differences in snapshot
	Timeout       string                    `yaml:"timeout"`         // Maximum execution time as a string, e.g., "1s", "1m", "1h", or a number (seconds)
	Valid         []string                  `yaml:"valid"`           // Format validations: "yaml", "json"
}
type TestCase struct {
	Name          string            `yaml:"name"`          // Name of the test
	Description   string            `yaml:"description"`   // Description of the test
	Enabled       bool              `yaml:"enabled"`       // Enable or disable the test
	Workdir       string            `yaml:"workdir"`       // Working directory for the command
	Command       string            `yaml:"command"`       // Command to run
	Args          []string          `yaml:"args"`          // Command arguments
	Env           map[string]string `yaml:"env"`           // Environment variables
	Expect        Expectation       `yaml:"expect"`        // Expected output
	Tty           bool              `yaml:"tty"`           // Enable TTY simulation
	Snapshot      bool              `yaml:"snapshot"`      // Enable snapshot comparison
	Clean         bool              `yaml:"clean"`         // Removes untracked files in work directory
	Sandbox       interface{}       `yaml:"sandbox"`       // bool (true=random) or string (named) or false (no sandbox)
	Short         *bool             `yaml:"short"`         // If false, skip when -short flag is passed (defaults to true)
	Preconditions []string          `yaml:"preconditions"` // Required preconditions for test execution
	Skip          struct {
		OS MatchPattern `yaml:"os"`
	} `yaml:"skip"`
}

type TestSuite struct {
	Tests []TestCase `yaml:"tests"`
}

type MatchPattern struct {
	Pattern string
	Negate  bool
}

// GetOrCreateNamedSandbox returns an existing named sandbox or creates a new one.
// Named sandboxes are shared across tests and cleaned up by TestMain.
// Workdir must be an absolute path.
func getOrCreateNamedSandbox(t *testing.T, name string, workdir string) *testhelpers.SandboxEnvironment {
	sandboxMutex.Lock()
	defer sandboxMutex.Unlock()

	if env, exists := sandboxRegistry[name]; exists {
		t.Logf("Reusing existing sandbox %q", name)
		return env
	}

	t.Logf("Creating new sandbox %q", name)
	env, err := testhelpers.SetupSandbox(t, workdir)
	if err != nil {
		t.Fatalf("Failed to setup sandbox %q: %v", name, err)
	}
	sandboxRegistry[name] = env
	return env
}

// CreateIsolatedSandbox creates a new isolated sandbox for a single test.
// Not added to registry, caller must clean up.
// Workdir must be an absolute path.
func createIsolatedSandbox(t *testing.T, workdir string) *testhelpers.SandboxEnvironment {
	t.Logf("Creating isolated sandbox")
	env, err := testhelpers.SetupSandbox(t, workdir)
	if err != nil {
		t.Fatalf("Failed to setup isolated sandbox: %v", err)
	}
	return env
}

// cleanupSandboxes cleans up all registered sandboxes.
func cleanupSandboxes() {
	sandboxMutex.Lock()
	defer sandboxMutex.Unlock()

	for name, env := range sandboxRegistry {
		env.Cleanup()
		delete(sandboxRegistry, name)
	}
}

func (m *MatchPattern) UnmarshalYAML(value *yaml.Node) error {
	switch value.Tag {
	case "!!str": // Regular string
		m.Pattern = value.Value
		m.Negate = false
	case "!not": // Negated pattern
		m.Pattern = value.Value
		m.Negate = true
	default:
		return fmt.Errorf("unsupported tag %q", value.Tag)
	}
	return nil
}

func parseTimeout(timeoutStr string) (time.Duration, error) {
	if timeoutStr == "" {
		return 0, nil // No timeout specified
	}

	// Try parsing as a duration string
	duration, err := time.ParseDuration(timeoutStr)
	if err == nil {
		return duration, nil
	}

	// If parsing failed, try interpreting as a number (seconds)
	seconds, err := strconv.Atoi(timeoutStr)
	if err != nil {
		return 0, fmt.Errorf("invalid timeout format: %s", timeoutStr)
	}

	return time.Duration(seconds) * time.Second, nil
}

func loadTestSuite(filePath string) (*TestSuite, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var suite TestSuite
	err = yaml.Unmarshal(data, &suite)
	if err != nil {
		return nil, err
	}

	// Default `diff` and `snapshot` if not present
	for i := range suite.Tests {
		testCase := &suite.Tests[i]

		// Ensure defaults for optional fields
		if testCase.Expect.Diff == nil {
			testCase.Expect.Diff = []string{}
		}
		if !testCase.Snapshot {
			testCase.Snapshot = false
		}

		// Default short to true if not specified
		if testCase.Short == nil {
			defaultShort := true
			testCase.Short = &defaultShort
		}

		if testCase.Env == nil {
			testCase.Env = make(map[string]string)
		}

		// Convey to atmos that it's running in a test environment
		testCase.Env["GO_TEST"] = "1"

		// Dynamically set GITHUB_TOKEN if not already set, to avoid rate limits
		if token, exists := os.LookupEnv("GITHUB_TOKEN"); exists {
			if _, alreadySet := testCase.Env["GITHUB_TOKEN"]; !alreadySet {
				testCase.Env["GITHUB_TOKEN"] = token
			}
		}

		// Dynamically set TTY-related environment variables if `Tty` is true
		if testCase.Tty {
			// Set TTY-specific environment variables
			testCase.Env["TERM"] = "xterm-256color" // Simulates terminal support
		}
	}

	return &suite, nil
}

func init() {
	// Initialize with default settings.
	logger = log.New()
	logger.SetOutput(os.Stdout)
	logger.SetLevel(log.InfoLevel)

	// Ensure that Lipgloss uses terminal colors for tests
	lipgloss.SetColorProfile(termenv.TrueColor)

	styles := log.DefaultStyles()
	styles.Levels[log.ErrorLevel] = lipgloss.NewStyle().
		SetString("ERROR").
		Padding(0, 0, 0, 0).
		Background(lipgloss.Color("204")).
		Foreground(lipgloss.Color("0"))
	styles.Levels[log.FatalLevel] = lipgloss.NewStyle().
		SetString("FATAL").
		Padding(0, 0, 0, 0).
		Background(lipgloss.Color("204")).
		Foreground(lipgloss.Color("0"))
	// Add a custom style for key `err`
	styles.Keys["err"] = lipgloss.NewStyle().Foreground(lipgloss.Color("204"))
	styles.Values["err"] = lipgloss.NewStyle().Bold(true)
	logger = log.New()
	logger.SetOutput(os.Stderr)
	logger.SetStyles(styles)
	logger.SetColorProfile(termenv.TrueColor)
	logger.Info("Smoke tests for atmos CLI starting")
}

// Determine if running in a CI environment.
func isCIEnvironment() bool {
	// Check for common CI environment variables
	// Note, that the CI variable has many possible truthy values, so we check for any non-empty value that is not "false".
	return (os.Getenv("CI") != "" && os.Getenv("CI") != "false") || os.Getenv("GITHUB_ACTIONS") == "true"
}

// collapseExtraSlashes replaces multiple consecutive slashes with a single slash.
func collapseExtraSlashes(s string) string {
	// Normalize the protocol to have exactly two slashes after http: or https:
	protocolRegex := regexp.MustCompile(`(?i)(https?):/*`)
	s = protocolRegex.ReplaceAllString(s, "$1://")

	// Split into protocol and the rest of the URL
	parts := regexp.MustCompile(`(?i)^(https?://)(.*)$`).FindStringSubmatch(s)
	if len(parts) == 3 {
		protocol := parts[1]
		rest := parts[2]
		// Collapse multiple slashes in the rest part
		rest = regexp.MustCompile(`/+`).ReplaceAllString(rest, "/")
		// Remove any leading slashes after the protocol to avoid triple slashes
		rest = strings.TrimLeft(rest, "/")
		return protocol + rest
	}

	// If no protocol, collapse all slashes
	return regexp.MustCompile(`/+`).ReplaceAllString(s, "/")
}

// sanitizeOutput replaces occurrences of the repository's absolute path in the output
// with the placeholder "/absolute/path/to/repo". It first normalizes both the repository root
// and the output to use forward slashes, ensuring that the replacement works reliably.
// An error is returned if the repository root cannot be determined.
// Convert something like:
//
//	D:\\a\atmos\atmos\examples\demo-stacks\stacks\deploy\**\*
//	   --> /absolute/path/to/repo/examples/demo-stacks/stacks/deploy/**/*
//	/home/runner/work/atmos/atmos/examples/demo-stacks/stacks/deploy/**/*
//	   --> /absolute/path/to/repo/examples/demo-stacks/stacks/deploy/**/*
func sanitizeOutput(output string) (string, error) {
	// 1. Get the repository root.
	repoRoot, err := findGitRepoRoot(startingDir)
	if err != nil {
		return "", err
	}

	if repoRoot == "" {
		return "", errors.New("failed to determine repository root")
	}

	// 2. Normalize the repository root:
	//    - Clean the path (which may not collapse all extra slashes after the drive letter, etc.)
	//    - Convert to forward slashes,
	//    - And explicitly collapse extra slashes.
	normalizedRepoRoot := collapseExtraSlashes(filepath.ToSlash(filepath.Clean(repoRoot)))
	// Also normalize the output to use forward slashes.
	// Note: filepath.ToSlash() on Windows converts path separators; on Unix it does nothing.
	// We also need to handle Windows-style paths that may appear in test output even on Unix (for testing).
	// Replace backslashes with forward slashes, EXCEPT those that are escape sequences (\n, \t, \r, etc.).
	// Since actual CLI output has escape sequences already processed (they appear as actual newlines/tabs),
	// we can safely replace backslashes that are followed by path-like characters.
	normalizedOutput := filepath.ToSlash(output)
	// Replace backslashes that look like path separators (followed by alphanumeric, ., -, _, *, etc.)
	// This regex matches backslash followed by path-like characters, not escape sequences.
	normalizedOutput = regexp.MustCompile(`\\([a-zA-Z0-9._*\-/])`).ReplaceAllString(normalizedOutput, "/$1")

	// 3. Build a regex that matches the repository root even if extra slashes appear.
	//    First, escape any regex metacharacters in the normalized repository root.
	quoted := regexp.QuoteMeta(normalizedRepoRoot)
	// Replace each literal "/" with the regex token "/+" so that e.g. "a/b/c" becomes "a/+b/+c".
	patternBody := strings.ReplaceAll(quoted, "/", "/+")
	// Allow for extra trailing slashes.
	// Use case-insensitive matching to handle Windows drive letters (D: vs d:) and path differences.
	pattern := "(?i)" + patternBody + "/*"
	repoRootRegex, err := regexp.Compile(pattern)
	if err != nil {
		return "", err
	}

	// 4. Replace any occurrence of the repository root (with extra slashes) with a fixed placeholder.
	//    The placeholder will end with exactly one slash.
	placeholder := "/absolute/path/to/repo/"
	replaced := repoRootRegex.ReplaceAllString(normalizedOutput, placeholder)

	// 5. Now collapse extra slashes in the remainder of file paths that start with the placeholder.
	//    We use a regex to find segments that start with the placeholder followed by some path characters.
	//    (We assume that file paths appear in quotes or other delimited contexts, and that URLs won't match.)
	fixRegex := regexp.MustCompile(`(/absolute/path/to/repo)([^",]+)`)
	result := fixRegex.ReplaceAllStringFunc(replaced, func(match string) string {
		// The regex has two groups: group 1 is the placeholder, group 2 is the remainder.
		groups := fixRegex.FindStringSubmatch(match)
		if len(groups) < 3 {
			return match
		}
		// Collapse extra slashes in the remainder.
		fixedRemainder := collapseExtraSlashes(groups[2])
		return groups[1] + fixedRemainder
	})

	// 6. Handle URLs in the output to ensure they are normalized.
	//    Use a regex to find URLs and collapse extra slashes while preserving the protocol.
	urlRegex := regexp.MustCompile(`(https?:/+[^\s]+)`)
	result = urlRegex.ReplaceAllStringFunc(result, collapseExtraSlashes)

	// 6b. Redact volatile request IDs to avoid snapshot flakiness.
	requestIDRegex1 := regexp.MustCompile(`(?i)\bRequestI[Dd]\s*:\s*[A-Za-z0-9-]+`)
	requestIDRegex2 := regexp.MustCompile(`(?i)\bX-Amzn-RequestId\s*:\s*[A-Za-z0-9-]+`)
	result = requestIDRegex1.ReplaceAllString(result, "RequestID: <REDACTED>")
	result = requestIDRegex2.ReplaceAllString(result, "RequestID: <REDACTED>")

	// 7. Remove the random number added to file name like `atmos-import-454656846`
	filePathRegex := regexp.MustCompile(`file_path=[^ ]+/atmos-import-\d+/atmos-import-\d+\.yaml`)
	result = filePathRegex.ReplaceAllString(result, "file_path=/atmos-import/atmos-import.yaml")

	// 8. Mask PostHog tokens to prevent real tokens from appearing in snapshots.
	// Match any token starting with phc_ followed by alphanumeric characters and underscores.
	posthogTokenRegex := regexp.MustCompile(`phc_[a-zA-Z0-9_]+`)
	result = posthogTokenRegex.ReplaceAllString(result, "phc_TEST_TOKEN_PLACEHOLDER")

	return result, nil
}

// sanitizeTestName converts t.Name() into a valid filename.
func sanitizeTestName(name string) string {
	// Replace slashes with underscores
	name = strings.ReplaceAll(name, "/", "_")

	// Remove or replace other problematic characters
	invalidChars := regexp.MustCompile(`[<>:"/\\|?*\x00-\x1F]`) // Matches invalid filename characters
	name = invalidChars.ReplaceAllString(name, "_")

	// Trim trailing periods and spaces (Windows-specific issue)
	name = strings.TrimRight(name, " .")

	return name
}

// Drop any lines matched by the ignore patterns so they do not affect the comparison.
func applyIgnorePatterns(input string, patterns []string) string {
	lines := strings.Split(input, "\n") // Split input into lines
	var filteredLines []string          // Store lines that don't match the patterns

	for _, line := range lines {
		shouldIgnore := false
		for _, pattern := range patterns {
			re := regexp.MustCompile(pattern)
			if re.MatchString(line) { // Check if the line matches the pattern
				shouldIgnore = true
				break // No need to check further patterns for this line
			}
		}
		if !shouldIgnore {
			filteredLines = append(filteredLines, line) // Add non-matching lines
		}
	}

	return strings.Join(filteredLines, "\n") // Join the filtered lines back into a string
}

// simulateTtyCommand executes a command in a pseudo-terminal (PTY) environment.
//
// IMPORTANT: PTY behavior merges stderr and stdout into a single stream!
// This is not a bug - it's how terminals work. A terminal display shows all output
// in one place; there's no separate "stderr screen" and "stdout screen".
//
// This means:
// - All output (stdout + stderr) will be captured together.
// - The returned string contains both streams merged.
// - This matches real terminal behavior where users see everything in one stream.
//
// For tests that need separate stderr/stdout streams, use non-TTY execution instead.
func simulateTtyCommand(t *testing.T, cmd *exec.Cmd, input string) (string, error) {
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return "", fmt.Errorf("failed to start TTY: %v", err)
	}
	defer func() { _ = ptmx.Close() }()

	// t.Logf("PTY Fd: %d, IsTerminal: %v", ptmx.Fd(), term.IsTerminal(int(ptmx.Fd())))

	if input != "" {
		go func() {
			_, _ = ptmx.Write([]byte(input))
			_ = ptmx.Close() // Ensure we close the input after writing
		}()
	}

	var buffer bytes.Buffer
	done := make(chan error, 1)
	go func() {
		// Use ReadFrom which properly handles EOF.
		_, err := buffer.ReadFrom(ptmx)
		done <- ptyError(err) // Wrap the error handling
	}()

	err = cmd.Wait()
	if err != nil {
		logger.Info("Command execution error", "err", err)
	}

	if readErr := <-done; readErr != nil {
		return "", fmt.Errorf("failed to read PTY output: %v", readErr)
	}

	output := buffer.String()
	// t.Logf("Captured Output:\n%s", output)

	return output, nil
}

// Linux kernel return EIO when attempting to read from a master pseudo
// terminal which no longer has an open slave. So ignore error here.
// See https://github.com/creack/pty/issues/21
// See https://github.com/owenthereal/upterm/pull/11
func ptyError(err error) error {
	if pathErr, ok := err.(*os.PathError); !ok || pathErr.Err != syscall.EIO {
		return err
	}
	return nil
}

// loadTestSuites loads and merges all .yaml files from the test-cases directory.
func loadTestSuites(testCasesDir string) (*TestSuite, error) {
	var mergedSuite TestSuite

	entries, err := os.ReadDir(testCasesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read test-cases directory: %v", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".yaml") {
			filePath := filepath.Join(testCasesDir, entry.Name())
			suite, err := loadTestSuite(filePath)
			if err != nil {
				return nil, fmt.Errorf("failed to load %s: %v", filePath, err)
			}
			mergedSuite.Tests = append(mergedSuite.Tests, suite.Tests...)
		}
	}

	return &mergedSuite, nil
}

// Entry point for tests to parse flags and handle setup/teardown.
func TestMain(m *testing.M) {
	// Declare err in the function's scope
	var err error

	// Capture the starting working directory
	startingDir, err = os.Getwd()
	if err != nil {
		logger.Fatal("failed to get the current working directory", err)
	}

	// Find the root of the Git repository
	repoRoot, err = findGitRepoRoot(startingDir)
	if err != nil {
		logger.Fatal("failed to locate git repository", "dir", startingDir)
	}

	// Check if we should collect coverage
	coverDir = os.Getenv("GOCOVERDIR")
	if coverDir != "" {
		logger.Info("Coverage collection enabled", "GOCOVERDIR", coverDir)
	}

	logger.Info("Starting directory", "dir", startingDir)
	// Define the base directory for snapshots relative to startingDir
	snapshotBaseDir = filepath.Join(startingDir, "snapshots")

	flag.Parse()        // Parse command-line flags
	exitCode := m.Run() // ALWAYS run tests so they can skip properly

	// Clean up sandboxes.
	cleanupSandboxes()

	// Clean up the temporary binary if we built one
	if atmosRunner != nil {
		atmosRunner.Cleanup()
	}

	errUtils.Exit(exitCode)
}

// checkPreconditions checks if all required preconditions for a test are met.
// If any precondition is not met, the test is skipped with an appropriate message.
func checkPreconditions(t *testing.T, preconditions []string) {
	t.Helper()

	// Map of precondition names to their check functions
	preconditionChecks := map[string]func(*testing.T){
		"github_token": RequireOCIAuthentication,
	}

	// Check each precondition
	for _, precondition := range preconditions {
		checkFunc, exists := preconditionChecks[precondition]
		if !exists {
			t.Fatalf("Unknown precondition: %s", precondition)
		}
		checkFunc(t)
	}
}

// prepareAtmosCommand prepares an atmos command with coverage support if enabled.
func prepareAtmosCommand(t *testing.T, ctx context.Context, args ...string) *exec.Cmd {
	// AtmosRunner should be initialized early in runCLICommandTest before directory changes
	if atmosRunner == nil {
		t.Fatalf("AtmosRunner should have been initialized before directory changes")
	}
	return atmosRunner.CommandContext(ctx, args...)
}

func runCLICommandTest(t *testing.T, tc TestCase) {
	defer func() {
		// Change back to the original working directory after the test
		if err := os.Chdir(startingDir); err != nil {
			t.Fatalf("Failed to change back to the starting directory: %v", err)
		}
	}()

	// Skip long tests in short mode
	if testing.Short() && tc.Short != nil && !*tc.Short {
		t.Skipf("Skipping long-running test in short mode (use 'go test' without -short to run)")
	}

	// Check preconditions before running the test
	checkPreconditions(t, tc.Preconditions)

	// Initialize AtmosRunner early, before any directory changes, so it can build from the git repo
	if tc.Command == "atmos" && atmosRunner == nil {
		atmosRunner = testhelpers.NewAtmosRunner(coverDir)
		if err := atmosRunner.Build(); err != nil {
			t.Skipf("Failed to initialize Atmos: %v", err)
		}
		logger.Info("Atmos runner initialized for test", "coverageEnabled", coverDir != "")
	}

	// Create a context with timeout if specified
	var ctx context.Context
	var cancel context.CancelFunc

	if tc.Expect.Timeout != "" {
		// Parse the timeout from the Expectation
		timeout, err := parseTimeout(tc.Expect.Timeout)
		if err != nil {
			t.Fatalf("Failed to parse timeout for test %s: %v", tc.Name, err)
		}
		if timeout > 0 {
			ctx, cancel = context.WithTimeout(context.Background(), timeout)
		} else {
			ctx, cancel = context.WithCancel(context.Background()) // No timeout, but cancelable
		}
	} else {
		ctx, cancel = context.WithCancel(context.Background()) // No timeout, but cancelable
	}
	defer cancel()

	// Create a temporary HOME directory for the test case that's clean
	// Otherwise a test may pass/fail due to existing files in the user's HOME directory
	tempDir, err := os.MkdirTemp("", "test_home")
	if err != nil {
		t.Fatalf("Failed to create temporary directory: %v", err)
	}
	defer func(path string) {
		_ = os.RemoveAll(path)
	}(tempDir) // Clean up the temporary directory after the test

	// ALWAYS set XDG_CACHE_HOME to a clean temp directory for test isolation
	// This ensures every test has its own cache and prevents interference
	xdgCacheHome := filepath.Join(tempDir, ".cache")
	tc.Env["XDG_CACHE_HOME"] = xdgCacheHome
	// Also set the process environment so removeCacheFile() uses the test path
	t.Setenv("XDG_CACHE_HOME", xdgCacheHome)
	// Reload XDG to pick up the new environment
	xdg.Reload()

	if runtime.GOOS == "darwin" && isCIEnvironment() {
		// For some reason the empty HOME directory causes issues on macOS in GitHub Actions
		// Copying over the `.gitconfig` was not enough to fix the issue
		logger.Info("skipping empty home dir on macOS in CI", "GOOS", runtime.GOOS)
	} else {
		// Set environment variables for the test case
		tc.Env["HOME"] = tempDir
		tc.Env["XDG_CONFIG_HOME"] = filepath.Join(tempDir, ".config")
		tc.Env["XDG_DATA_HOME"] = filepath.Join(tempDir, ".local", "share")
		// Copy some files to the temporary HOME directory
		originalHome := os.Getenv("HOME")
		filesToCopy := []string{".gitconfig", ".ssh", ".netrc"} // Expand list if needed
		for _, file := range filesToCopy {
			src := filepath.Join(originalHome, file)
			dest := filepath.Join(tempDir, file)

			if _, err := os.Stat(src); err == nil { // Check if the file/directory exists
				// t.Logf("Copying %s to %s\n", src, dest)
				if err := copy.Copy(src, dest); err != nil {
					t.Fatalf("Failed to copy %s to test folder: %v", src, err)
				}
			}
		}
	}

	// Change to the specified working directory
	if tc.Workdir != "" {
		absoluteWorkdir, err := filepath.Abs(tc.Workdir)
		if err != nil {
			t.Fatalf("failed to resolve absolute path of workdir %q: %v", tc.Workdir, err)
		}
		err = os.Chdir(absoluteWorkdir)
		if err != nil {
			t.Fatalf("Failed to change directory to %q: %v", tc.Workdir, err)
		}

		// Setup sandbox environment if enabled
		var sandboxEnv *testhelpers.SandboxEnvironment
		switch v := tc.Sandbox.(type) {
		case bool:
			if v {
				// Boolean true = isolated sandbox for this test only
				logger.Info("Setting up isolated sandbox", "test", tc.Name, "workdir", absoluteWorkdir)
				sandboxEnv = createIsolatedSandbox(t, absoluteWorkdir)
				// Clean up immediately after test
				defer func() {
					logger.Debug("Cleaning up isolated sandbox", "tempdir", sandboxEnv.TempDir)
					sandboxEnv.Cleanup()
				}()
			}
		case string:
			if v != "" {
				// Named sandbox = shared across related tests
				logger.Info("Using named sandbox", "test", tc.Name, "name", v, "workdir", absoluteWorkdir)
				sandboxEnv = getOrCreateNamedSandbox(t, v, absoluteWorkdir)
				// Cleanup handled by TestMain
			}
		}

		// Add sandbox environment variables to override component paths
		if sandboxEnv != nil {
			if tc.Env == nil {
				tc.Env = make(map[string]string)
			}
			for k, v := range sandboxEnv.GetEnvironmentVariables() {
				logger.Debug("Setting sandbox env var", "key", k, "value", v)
				tc.Env[k] = v
			}
		}

		// Clean the directory if enabled
		if tc.Clean {
			logger.Info("Cleaning directory", "workdir", tc.Workdir)
			if err := cleanDirectory(t, absoluteWorkdir); err != nil {
				t.Fatalf("Failed to clean directory %q: %v", tc.Workdir, err)
			}
		}
	}

	// Include the system PATH in the test environment
	tc.Env["PATH"] = os.Getenv("PATH")

	// Set the test Git root to a clean temporary directory
	// This makes each test scenario act as if it's its own Git repository
	// preventing the actual repository's .atmos.d from being loaded
	// This is especially important for tests that use workdir: "../"
	testGitRoot := filepath.Join(tempDir, "mock-git-root")
	if err := os.MkdirAll(testGitRoot, 0o755); err == nil {
		tc.Env["TEST_GIT_ROOT"] = testGitRoot
	}

	// Also set an environment variable to exclude the repository's .atmos.d
	// This is needed for tests that change to parent directories
	tc.Env["TEST_EXCLUDE_ATMOS_D"] = repoRoot

	// Remove the cache file before running the test.
	// This is to ensure that the test is not affected by the cache file.
	err = removeCacheFile()
	assert.NoError(t, err, "failed to remove cache file")

	// Preserve the CI environment variables.
	// This is to ensure that the test is not affected by the CI environment variables.
	currentEnvVars := telemetry.PreserveCIEnvVars()
	defer telemetry.RestoreCIEnvVars(currentEnvVars)

	// Force consistent color/terminal environment for reproducible ANSI codes across platforms.
	// Test cases can still override these by explicitly setting them.
	if _, exists := tc.Env["TERM"]; !exists {
		tc.Env["TERM"] = "xterm-256color"
	}
	if _, exists := tc.Env["COLORTERM"]; !exists {
		tc.Env["COLORTERM"] = "" // Explicitly empty to prevent truecolor (force 256-color)
	}
	// Set any environment variables defined in the test case using t.Setenv for proper isolation.
	for key, value := range tc.Env {
		t.Setenv(key, value)
	}

	// Prepare the command based on what's being tested
	var cmd *exec.Cmd
	if tc.Command == "atmos" {
		cmd = prepareAtmosCommand(t, ctx, tc.Args...)
	} else {
		// For non-atmos commands, use regular exec
		binaryPath, err := exec.LookPath(tc.Command)
		if err != nil {
			t.Fatalf("Binary not found: %s", tc.Command)
		}
		cmd = exec.CommandContext(ctx, binaryPath, tc.Args...)
	}

	// Preserve GOCOVERDIR if it's already set by atmosRunner
	existingEnv := cmd.Env
	if existingEnv == nil {
		existingEnv = []string{}
	}

	// Preserve all environment variables from AtmosRunner (including PATH and GOCOVERDIR)
	// and add/override with test-specific environment variables
	var envVars []string

	// Start with the environment from AtmosRunner if available
	if len(existingEnv) > 0 {
		envVars = append(envVars, existingEnv...)
	}

	// Add/override test-specific environment variables
	for key, value := range tc.Env {
		// NEVER allow test cases to override PATH - AtmosRunner's PATH must be preserved
		if key == "PATH" {
			continue
		}

		// Remove any existing env var with the same key before adding the new one
		for i, env := range envVars {
			if strings.HasPrefix(env, key+"=") {
				envVars = append(envVars[:i], envVars[i+1:]...)
				break
			}
		}
		envVars = append(envVars, fmt.Sprintf("%s=%s", key, value))
	}

	// Ensure NO_COLOR is not inherited unless test explicitly sets it (presence disables color).
	if _, exists := tc.Env["NO_COLOR"]; !exists {
		for i, env := range envVars {
			if strings.HasPrefix(env, "NO_COLOR=") {
				envVars = append(envVars[:i], envVars[i+1:]...)
				break
			}
		}
	}

	cmd.Env = envVars

	var stdout, stderr bytes.Buffer
	var exitCode int

	if tc.Tty {
		// Run the command in TTY mode
		ptyOutput, err := simulateTtyCommand(t, cmd, "")

		// Check if the context timeout was exceeded
		if ctx.Err() == context.DeadlineExceeded {
			t.Errorf("Reason: Test timed out after %s", tc.Expect.Timeout)
			t.Errorf("Captured stdout:\n%s", stdout.String())
			t.Errorf("Captured stderr:\n%s", stderr.String())
			return
		}

		if err != nil {
			// Check if the error is an ExitError
			if exitErr, ok := err.(*exec.ExitError); ok {
				// Capture the actual exit code
				exitCode = exitErr.ExitCode()

				if exitCode < 0 {
					// Negative exit code indicates interruption by a signal
					t.Errorf("TTY Command interrupted by signal: %s, Signal: %d, Error: %v", tc.Command, -exitCode, err)
				}
			} else {
				// Handle other types of errors
				t.Fatalf("Failed to simulate TTY command: %v", err)
			}
		}
		stdout.WriteString(ptyOutput)
	} else {
		// Run the command in non-TTY mode

		// Attach stdout and stderr buffers for non-TTY execution
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		if ctx.Err() == context.DeadlineExceeded {
			// Handle the timeout case first
			t.Errorf("Reason: Test timed out after %s", tc.Expect.Timeout)
			t.Errorf("Captured stdout:\n%s", stdout.String())
			t.Errorf("Captured stderr:\n%s", stderr.String())
			return
		}

		if err != nil {
			// Handle other command execution errors
			if exitErr, ok := err.(*exec.ExitError); ok {
				// Capture the actual exit code
				exitCode = exitErr.ExitCode()

				if exitCode < 0 {
					// Negative exit code indicates termination by a signal
					t.Errorf("Non-TTY Command terminated by signal: %s, Signal: %d, Error: %v", tc.Command, -exitCode, err)
				}
			} else {
				// Handle other non-exec-related errors
				t.Fatalf("Failed to run command; Error: %v", err)
			}
		} else {
			// Successful command execution
			exitCode = 0
		}
	}

	// Validate outputs
	if !verifyExitCode(t, tc.Expect.ExitCode, exitCode) {
		t.Errorf("Description: %s", tc.Description)
	}

	// Validate output based on TTY mode
	verifyTestOutputs(t, &tc, stdout.String(), stderr.String())

	// Validate format (YAML/JSON)
	if len(tc.Expect.Valid) > 0 {
		if !verifyFormatValidation(t, stdout.String(), tc.Expect.Valid) {
			t.Errorf("Format validation failed for test: %s", tc.Name)
			t.Errorf("Description: %s", tc.Description)
		}
	}

	// Validate file existence
	if !verifyFileExists(t, tc.Expect.FileExists) {
		t.Errorf("Description: %s", tc.Description)
	}

	// Validate file not existence
	if !verifyFileNotExists(t, tc.Expect.FileNotExists) {
		t.Errorf("Description: %s", tc.Description)
	}

	// Validate file contents
	if !verifyFileContains(t, tc.Expect.FileContains) {
		t.Errorf("Description: %s", tc.Description)
	}

	// Validate snapshots
	if !verifySnapshot(t, tc, stdout.String(), stderr.String(), *regenerateSnapshots) {
		t.Errorf("Description: %s", tc.Description)
	}
}

func removeCacheFile() error {
	cacheFilePath, err := config.GetCacheFilePath()
	if err != nil {
		return nil
	}

	if _, err := os.Stat(cacheFilePath); os.IsNotExist(err) {
		return nil
	}
	err = os.Remove(cacheFilePath)
	if err != nil {
		return err
	}
	return nil
}

func TestCLICommands(t *testing.T) {
	if skipReason != "" {
		t.Skipf("%s", skipReason)
	}

	// Load test suite
	testSuite, err := loadTestSuites("test-cases")
	if err != nil {
		t.Fatalf("Failed to load test suites: %v", err)
	}

	for _, tc := range testSuite.Tests {
		if !tc.Enabled {
			logger.Warn("Skipping disabled test", "test", tc.Name)
			continue
		}

		// Check OS condition for skipping
		if !verifyOS(t, []MatchPattern{tc.Skip.OS}) {
			logger.Info("Skipping test due to OS condition", "test", tc.Name)
			continue
		}

		// Run tests
		t.Run(tc.Name, func(t *testing.T) {
			runCLICommandTest(t, tc)
		})
	}
}

func verifyOS(t *testing.T, osPatterns []MatchPattern) bool {
	currentOS := runtime.GOOS // Get the current operating system
	success := true

	for _, pattern := range osPatterns {
		// Compile the regex pattern
		re, err := regexp.Compile(pattern.Pattern)
		if err != nil {
			t.Errorf("Invalid OS regex pattern: %q, error: %v", pattern.Pattern, err)
			success = false
			continue
		}

		// Check if the current OS matches the pattern
		match := re.MatchString(currentOS)
		if pattern.Negate && match {
			logger.Info("Reason: OS matched negated pattern", "os", currentOS, "pattern", pattern.Pattern)
			success = false
		} else if !pattern.Negate && !match {
			logger.Info("Reason: OS did not match pattern", "os", currentOS, "pattern", pattern.Pattern)
			success = false
		}
	}

	return success
}

func verifyExitCode(t *testing.T, expected, actual int) bool {
	success := true
	if expected != actual {
		t.Errorf("Reason: Expected exit code %d, got %d", expected, actual)
		success = false
	}
	return success
}

func verifyOutput(t *testing.T, outputType, output string, patterns []MatchPattern) bool {
	success := true
	for _, pattern := range patterns {
		re, err := regexp.Compile(pattern.Pattern)
		if err != nil {
			t.Errorf("Invalid %s regex: %q, error: %v", outputType, pattern.Pattern, err)
			success = false
			continue
		}

		match := re.MatchString(output)
		if pattern.Negate && match {
			t.Errorf("Reason: %s unexpectedly matched negated pattern %q.", outputType, pattern.Pattern)
			t.Errorf("Output: %q", output)
			success = false
		} else if !pattern.Negate && !match {
			t.Errorf("Reason: %s did not match pattern %q.", outputType, pattern.Pattern)
			t.Errorf("Output: %q", output)
			success = false
		}
	}
	return success
}

func verifyFileExists(t *testing.T, files []string) bool {
	success := true
	for _, file := range files {
		if _, err := os.Stat(file); errors.Is(err, os.ErrNotExist) {
			t.Errorf("Reason: Expected file does not exist: %q", file)
			success = false
		}
	}
	return success
}

func verifyFileNotExists(t *testing.T, files []string) bool {
	success := true
	for _, file := range files {
		if _, err := os.Stat(file); err == nil {
			t.Errorf("Reason: File %q exists but it should not.", file)
			success = false
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Errorf("Reason: Unexpected error checking file %q: %v", file, err)
			success = false
		}
	}
	return success
}

func verifyFileContains(t *testing.T, filePatterns map[string][]MatchPattern) bool {
	success := true
	for file, patterns := range filePatterns {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Errorf("Reason: Failed to read file %q: %v", file, err)
			success = false
			continue
		}
		for _, matchPattern := range patterns {
			re, err := regexp.Compile(matchPattern.Pattern)
			if err != nil {
				t.Errorf("Invalid regex for file %q: %q, error: %v", file, matchPattern.Pattern, err)
				success = false
				continue
			}
			if matchPattern.Negate {
				// Negated pattern: Ensure the pattern does NOT match
				if re.Match(content) {
					t.Errorf("Reason: File %q unexpectedly matched negated pattern %q.", file, matchPattern.Pattern)
					t.Errorf("Content: %q", string(content))
					success = false
				}
			} else {
				// Regular pattern: Ensure the pattern matches
				if !re.Match(content) {
					t.Errorf("Reason: File %q did not match pattern %q.", file, matchPattern.Pattern)
					t.Errorf("Content: %q", string(content))
					success = false
				}
			}
		}
	}
	return success
}

func verifyFormatValidation(t *testing.T, output string, formats []string) bool {
	for _, format := range formats {
		switch format {
		case "json":
			if !verifyJSONFormat(t, output) {
				return false
			}
		case "yaml":
			if !verifyYAMLFormat(t, output) {
				return false
			}
		default:
			t.Logf("Unknown format: %s", format)
			return false
		}
	}
	return true
}

func verifyYAMLFormat(t *testing.T, output string) bool {
	var data interface{}
	err := yaml.Unmarshal([]byte(output), &data)
	if err != nil {
		t.Logf("YAML validation failed: %v", err)
		// Show context around the error if possible.
		lines := strings.Split(output, "\n")
		preview := strings.Join(lines[:min(10, len(lines))], "\n")
		if len(preview) > 500 {
			preview = preview[:500] + "..."
		}
		t.Logf("Output preview:\n%s", preview)
		return false
	}
	return true
}

func verifyJSONFormat(t *testing.T, output string) bool {
	var data interface{}
	err := json.Unmarshal([]byte(output), &data)
	if err != nil {
		t.Logf("JSON validation failed: %v", err)
		// Try to provide context about where the error occurred.
		if syntaxErr, ok := err.(*json.SyntaxError); ok {
			offset := syntaxErr.Offset
			// Show a snippet around the error location.
			start := max(0, int(offset)-50)
			end := min(len(output), int(offset)+50)
			snippet := output[start:end]
			t.Logf("Error at offset %d, context: ...%s...", offset, snippet)
		}
		return false
	}
	return true
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func updateSnapshot(fullPath, output string) {
	err := os.MkdirAll(filepath.Dir(fullPath), 0o755) // Ensure parent directories exist
	if err != nil {
		panic(fmt.Sprintf("Failed to create snapshot directory: %v", err))
	}
	// Normalize line endings to LF for cross-platform consistency.
	// This ensures snapshots work reliably across Windows, macOS, and Linux.
	normalized := normalizeLineEndings(output)
	err = os.WriteFile(fullPath, []byte(normalized), 0o644) // Write snapshot
	if err != nil {
		panic(fmt.Sprintf("Failed to write snapshot file: %v", err))
	}
}

func readSnapshot(t *testing.T, fullPath string) string {
	data, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("Error reading snapshot file %q: %v", fullPath, err)
	}
	// Normalize line endings when reading to gracefully handle any existing
	// snapshots that were committed with CRLF line endings.
	return normalizeLineEndings(string(data))
}

// normalizeLineEndings converts CRLF line endings to LF for cross-platform consistency.
// This ensures snapshots work reliably across Windows, macOS, and Linux development.
//
// Important: Only CRLF sequences (\r\n) are converted to LF (\n).
// Standalone CR (\r) characters are preserved, as they're used by spinners and
// progress indicators to overwrite terminal lines.
//
// Examples:
//   - "line1\r\nline2\r\n" → "line1\nline2\n" (CRLF normalized)
//   - "line1\nline2\n" → "line1\nline2\n" (LF unchanged)
//   - "Progress\r" → "Progress\r" (spinner CR preserved)
func normalizeLineEndings(s string) string {
	// Only replace CRLF with LF, preserve standalone CR for spinners.
	return strings.ReplaceAll(s, "\r\n", "\n")
}

// Generate a unified diff using gotextdiff.
func generateUnifiedDiff(actual, expected string) string {
	edits := myers.ComputeEdits(span.URIFromPath("actual"), expected, actual)
	unified := gotextdiff.ToUnified("expected", "actual", expected, edits)

	// Use a buffer to construct the colorized diff
	var buf bytes.Buffer
	for _, line := range strings.Split(fmt.Sprintf("%v", unified), "\n") {
		switch {
		case strings.HasPrefix(line, "+"):
			// Apply green style for additions
			fmt.Fprintln(&buf, addedStyle.Render(line))
		case strings.HasPrefix(line, "-"):
			// Apply red style for deletions
			fmt.Fprintln(&buf, removedStyle.Render(line))
		default:
			// Keep other lines as-is
			fmt.Fprintln(&buf, line)
		}
	}
	return buf.String()
}

// Generate a diff using diffmatchpatch.
func DiffStrings(x, y string) string {
	dmp := diffmatchpatch.New()
	diffs := dmp.DiffMain(x, y, false)
	dmp.DiffCleanupSemantic(diffs) // Clean up the diff for readability
	return dmp.DiffPrettyText(diffs)
}

// Colorize diff output based on the threshold.
func colorizeDiffWithThreshold(actual, expected string, threshold int) string {
	dmp := diffmatchpatch.New()
	diffs := dmp.DiffMain(expected, actual, false)
	dmp.DiffCleanupSemantic(diffs)

	var sb strings.Builder
	for _, diff := range diffs {
		text := diff.Text
		switch diff.Type {
		case diffmatchpatch.DiffInsert, diffmatchpatch.DiffDelete:
			if len(text) < threshold {
				// For short diffs, highlight entire line
				sb.WriteString(fmt.Sprintf("\033[1m\033[33m%s\033[0m", text))
			} else {
				// For long diffs, highlight at word/character level
				color := "\033[32m" // Insert: green
				if diff.Type == diffmatchpatch.DiffDelete {
					color = "\033[31m" // Delete: red
				}
				sb.WriteString(fmt.Sprintf("%s%s\033[0m", color, text))
			}
		case diffmatchpatch.DiffEqual:
			sb.WriteString(text)
		}
	}

	return sb.String()
}

// getSnapshotFilenames returns the appropriate snapshot filenames based on whether TTY mode is enabled.
// When isTty is true, returns only the .tty.golden filename.
// When isTty is false, returns .stdout.golden and .stderr.golden filenames.
func getSnapshotFilenames(testName string, isTty bool) (stdout, stderr, tty string) {
	sanitized := sanitizeTestName(testName)
	if isTty {
		return "", "", filepath.Join(snapshotBaseDir, sanitized+".tty.golden")
	}
	return filepath.Join(snapshotBaseDir, sanitized+".stdout.golden"),
		filepath.Join(snapshotBaseDir, sanitized+".stderr.golden"),
		""
}

// verifyTTYSnapshot handles snapshot verification for TTY mode tests.
func verifyTTYSnapshot(t *testing.T, tc *TestCase, ttyPath, combinedOutput string, regenerate bool) bool {
	if regenerate {
		t.Logf("Updating TTY snapshot at %q", ttyPath)
		updateSnapshot(ttyPath, combinedOutput)
		return true
	}

	if _, err := os.Stat(ttyPath); errors.Is(err, os.ErrNotExist) {
		t.Fatalf(`TTY snapshot file not found: %q
Run the following command to create it:
$ go test ./tests -run %q -regenerate-snapshots`, ttyPath, t.Name())
	}

	filteredActual := applyIgnorePatterns(combinedOutput, tc.Expect.Diff)
	filteredExpected := applyIgnorePatterns(readSnapshot(t, ttyPath), tc.Expect.Diff)

	if filteredExpected != filteredActual {
		var diff string
		if isCIEnvironment() || !term.IsTerminal(int(os.Stdout.Fd())) {
			diff = generateUnifiedDiff(filteredActual, filteredExpected)
		} else {
			diff = colorizeDiffWithThreshold(filteredActual, filteredExpected, 10)
		}
		t.Errorf("TTY output mismatch for %q:\n%s", ttyPath, diff)
	}

	return true
}

// verifyTestOutputs validates test outputs based on TTY mode.
func verifyTestOutputs(t *testing.T, tc *TestCase, stdout, stderr string) {
	if tc.Tty {
		// TTY mode: validate combined output against tty expectations
		if !verifyOutput(t, "tty", stdout, tc.Expect.Tty) {
			t.Errorf("TTY output mismatch for test: %s", tc.Name)
		}
		return
	}

	// Non-TTY mode: validate stdout and stderr separately
	if !verifyOutput(t, "stdout", stdout, tc.Expect.Stdout) {
		t.Errorf("Stdout mismatch for test: %s", tc.Name)
	}

	if !verifyOutput(t, "stderr", stderr, tc.Expect.Stderr) {
		t.Errorf("Stderr mismatch for test: %s", tc.Name)
	}
}

func verifySnapshot(t *testing.T, tc TestCase, stdoutOutput, stderrOutput string, regenerate bool) bool {
	if !tc.Snapshot {
		return true
	}

	// Sanitize outputs and fail the test if sanitization fails.
	var err error
	stdoutOutput, err = sanitizeOutput(stdoutOutput)
	if err != nil {
		t.Fatalf("failed to sanitize stdout output: %v", err)
	}
	stderrOutput, err = sanitizeOutput(stderrOutput)
	if err != nil {
		t.Fatalf("failed to sanitize stderr output: %v", err)
	}

	// Normalize line endings in actual output for cross-platform consistency.
	// This handles cases where CLI might output CRLF on Windows but snapshots use LF.
	stdoutOutput = normalizeLineEndings(stdoutOutput)
	stderrOutput = normalizeLineEndings(stderrOutput)

	stdoutPath, stderrPath, ttyPath := getSnapshotFilenames(t.Name(), tc.Tty)

	// TTY mode: combined output in single .tty.golden file
	if tc.Tty {
		// In TTY mode, stdout contains the combined output (from PTY)
		return verifyTTYSnapshot(t, &tc, ttyPath, stdoutOutput, regenerate)
	}

	// Non-TTY mode: separate stdout and stderr snapshots
	if regenerate {
		t.Logf("Updating stdout snapshot at %q", stdoutPath)
		updateSnapshot(stdoutPath, stdoutOutput)
		t.Logf("Updating stderr snapshot at %q", stderrPath)
		updateSnapshot(stderrPath, stderrOutput)
		return true
	}

	// Verify stdout
	if _, err := os.Stat(stdoutPath); errors.Is(err, os.ErrNotExist) {
		t.Fatalf(`Stdout snapshot file not found: %q
Run the following command to create it:
$ go test ./tests -run %q -regenerate-snapshots`, stdoutPath, t.Name())
	}

	filteredStdoutActual := applyIgnorePatterns(stdoutOutput, tc.Expect.Diff)
	filteredStdoutExpected := applyIgnorePatterns(readSnapshot(t, stdoutPath), tc.Expect.Diff)

	if filteredStdoutExpected != filteredStdoutActual {
		var diff string
		if isCIEnvironment() || !term.IsTerminal(int(os.Stdout.Fd())) {
			// Generate a colorized diff for better readability
			diff = generateUnifiedDiff(filteredStdoutActual, filteredStdoutExpected)
		} else {
			diff = colorizeDiffWithThreshold(filteredStdoutActual, filteredStdoutExpected, 10)
		}

		t.Errorf("Stdout mismatch for %q:\n%s", stdoutPath, diff)
	}

	// Verify stderr
	if _, err := os.Stat(stderrPath); errors.Is(err, os.ErrNotExist) {
		t.Fatalf(`Stderr snapshot file not found: %q
Run the following command to create it:
$ go test -run=%q -regenerate-snapshots`, stderrPath, t.Name())
	}
	filteredStderrActual := applyIgnorePatterns(stderrOutput, tc.Expect.Diff)
	filteredStderrExpected := applyIgnorePatterns(readSnapshot(t, stderrPath), tc.Expect.Diff)

	if filteredStderrExpected != filteredStderrActual {
		var diff string
		if isCIEnvironment() || !term.IsTerminal(int(os.Stdout.Fd())) {
			diff = generateUnifiedDiff(filteredStderrActual, filteredStderrExpected)
		} else {
			// Generate a colorized diff for better readability
			diff = colorizeDiffWithThreshold(filteredStderrActual, filteredStderrExpected, 10)
		}
		t.Errorf("Stderr diff mismatch for %q:\n%s", stdoutPath, diff)
	}

	return true
}

// Clean up untracked files in the working directory.
func cleanDirectory(t *testing.T, workdir string) error {
	// Find the root of the Git repository
	repoRoot, err := findGitRepoRoot(workdir)
	if err != nil {
		return fmt.Errorf("failed to locate git repository from %q: %w", workdir, err)
	}

	// Open the repository
	repo, err := git.PlainOpen(repoRoot)
	if err != nil {
		return fmt.Errorf("failed to open git repository: %w", err)
	}

	// Get the worktree
	worktree, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	// Get the repository status
	status, err := worktree.Status()
	if err != nil {
		return fmt.Errorf("failed to get git status: %w", err)
	}

	// Clean only files in the provided working directory
	for file, statusEntry := range status {
		if statusEntry.Worktree == git.Untracked {
			fullPath := filepath.Join(repoRoot, file)
			if strings.HasPrefix(fullPath, workdir) {
				t.Logf("Removing untracked file: %q\n", fullPath)
				if err := os.RemoveAll(fullPath); err != nil {
					return fmt.Errorf("failed to remove %q: %w", fullPath, err)
				}
			}
		}
	}

	return nil
}

// findGitRepo finds the Git repository root.
func findGitRepoRoot(path string) (string, error) {
	// Open the Git repository starting from the given path
	repo, err := git.PlainOpenWithOptions(path, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return "", fmt.Errorf("failed to find git repository: %w", err)
	}

	// Get the repository's working tree
	worktree, err := repo.Worktree()
	if err != nil {
		return "", fmt.Errorf("failed to get worktree: %w", err)
	}

	// Return the absolute path to the root of the working tree
	root, err := filepath.Abs(worktree.Filesystem.Root())
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path of repository root: %w", err)
	}

	return root, nil
}

func TestUnmarshalMatchPattern(t *testing.T) {
	yamlData := `
expect:
  stdout:
    - "Normal output"
    - !not "Negated pattern"
`

	type TestCase struct {
		Expect struct {
			Stdout []MatchPattern `yaml:"stdout"`
		} `yaml:"expect"`
	}

	var testCase TestCase
	err := yaml.Unmarshal([]byte(yamlData), &testCase)
	if err != nil {
		t.Fatalf("Failed to unmarshal YAML: %v", err)
	}

	for i, pattern := range testCase.Expect.Stdout {
		t.Logf("Pattern %d: %+v", i, pattern)
	}
}
