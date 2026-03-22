package tests

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath" // For resolving absolute paths
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"go.yaml.in/yaml/v3"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
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

// gitHubUsernameVars lists environment variables that expose the GitHub username.
// They are cleared from the subprocess environment (unless the test explicitly sets them)
// to produce consistent golden snapshots across local and CI environments.
var gitHubUsernameVars = []string{"ATMOS_GITHUB_USERNAME", "GITHUB_ACTOR", "GITHUB_USERNAME"}

// ghaOutputEnvVars lists GitHub Actions environment variables whose values are
// relative file paths written by the subprocess (e.g. GITHUB_OUTPUT, GITHUB_STEP_SUMMARY).
// These files are registered for cleanup after each test to prevent stale data
// from a failed run from corrupting the next run.
var ghaOutputEnvVars = []string{"GITHUB_OUTPUT", "GITHUB_STEP_SUMMARY"}

type Expectation struct {
	Stdout                   []MatchPattern            `yaml:"stdout"`                     // Expected stdout output (non-TTY mode)
	Stderr                   []MatchPattern            `yaml:"stderr"`                     // Expected stderr output (non-TTY mode)
	Tty                      []MatchPattern            `yaml:"tty"`                        // Expected TTY output (TTY mode - combined stdout+stderr)
	ExitCode                 int                       `yaml:"exit_code"`                  // Expected exit code
	FileExists               []string                  `yaml:"file_exists"`                // Files to validate
	FileNotExists            []string                  `yaml:"file_not_exists"`            // Files that should not exist
	FileContains             map[string][]MatchPattern `yaml:"file_contains"`              // File contents to validate (file to patterns map)
	Diff                     []string                  `yaml:"diff"`                       // Acceptable differences in snapshot
	Timeout                  string                    `yaml:"timeout"`                    // Maximum execution time as a string, e.g., "1s", "1m", "1h", or a number (seconds)
	Valid                    []string                  `yaml:"valid"`                      // Format validations: "yaml", "json"
	IgnoreTrailingWhitespace bool                      `yaml:"ignore_trailing_whitespace"` // Strip trailing whitespace before snapshot comparison
	Sanitize                 map[string]string         `yaml:"sanitize"`                   // Custom sanitization rules (regex pattern -> replacement)
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
	Parallel      *bool             `yaml:"parallel"`      // If false, test runs sequentially (defaults to true)
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

		// Default parallel to true if not specified
		if testCase.Parallel == nil {
			defaultParallel := true
			testCase.Parallel = &defaultParallel
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
	// Parse flags first to get -v status
	flag.Parse()

	// CRITICAL: Unset ATMOS_CHDIR to prevent tests from accessing non-test directories.
	// This prevents tests from inadvertently reading real infrastructure configs (e.g., infra-live).
	// Tests should only use their fixture directories, not the user's working environment.
	os.Unsetenv("ATMOS_CHDIR")

	// Disable CI auto-detection so deploy/apply hooks don't try to
	// download planfiles from GitHub Artifacts during tests.
	os.Unsetenv("GITHUB_ACTIONS")

	// Configure logger verbosity based on test flags
	switch {
	case os.Getenv("ATMOS_TEST_DEBUG") != "":
		logger.SetLevel(log.DebugLevel) // Show everything including debug
	case testing.Verbose():
		logger.SetLevel(log.InfoLevel) // Show info, warnings, and errors with -v flag
	default:
		logger.SetLevel(log.WarnLevel) // Only show warnings and errors by default
	}

	logger.Info("Smoke tests for atmos CLI starting")

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

	// Pre-build the Atmos binary once before running any tests.
	// This avoids a race condition that arises when multiple parallel tests
	// all try to lazily initialise atmosRunner at the same time.
	atmosRunner = testhelpers.NewAtmosRunner(coverDir)
	if err := atmosRunner.Build(); err != nil {
		logger.Warn("Failed to pre-build Atmos binary; tests requiring 'atmos' will be skipped", "error", err)
		atmosRunner = nil // Signals individual tests to skip themselves.
	} else {
		logger.Info("Atmos binary pre-built", "coverageEnabled", coverDir != "")
	}

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
		"github_token":    RequireOCIAuthentication,
		"aws_credentials": RequireAWSCredentials,
		"terraform":       RequireTerraform,
		"packer":          RequirePacker,
		"helmfile":        RequireHelmfile,
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
}
