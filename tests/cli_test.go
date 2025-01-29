package tests

import (
	"bytes"
	"context"
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
	"golang.org/x/term"
	"gopkg.in/yaml.v3"
)

// Command-line flag for regenerating snapshots
var regenerateSnapshots = flag.Bool("regenerate-snapshots", false, "Regenerate all golden snapshots")
var startingDir string
var snapshotBaseDir string

// Define styles using lipgloss
var (
	addedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))  // Green
	removedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("160")) // Red
)

type Expectation struct {
	Stdout       []MatchPattern            `yaml:"stdout"`        // Expected stdout output
	Stderr       []MatchPattern            `yaml:"stderr"`        // Expected stderr output
	ExitCode     int                       `yaml:"exit_code"`     // Expected exit code
	FileExists   []string                  `yaml:"file_exists"`   // Files to validate
	FileContains map[string][]MatchPattern `yaml:"file_contains"` // File contents to validate (file to patterns map)
	Diff         []string                  `yaml:"diff"`          // Acceptable differences in snapshot
	Timeout      string                    `yaml:"timeout"`       // Maximum execution time as a string, e.g., "1s", "1m", "1h", or a number (seconds)
}
type TestCase struct {
	Name        string            `yaml:"name"`        // Name of the test
	Description string            `yaml:"description"` // Description of the test
	Enabled     bool              `yaml:"enabled"`     // Enable or disable the test
	Workdir     string            `yaml:"workdir"`     // Working directory for the command
	Command     string            `yaml:"command"`     // Command to run
	Args        []string          `yaml:"args"`        // Command arguments
	Env         map[string]string `yaml:"env"`         // Environment variables
	Expect      Expectation       `yaml:"expect"`      // Expected output
	Tty         bool              `yaml:"tty"`         // Enable TTY simulation
	Snapshot    bool              `yaml:"snapshot"`    // Enable snapshot comparison
	Clean       bool              `yaml:"clean"`       // Removes untracked files in work directory
	Skip        struct {
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

type PathManager struct {
	OriginalPath string
	Prepended    []string
}

// NewPathManager initializes a PathManager with the current PATH.
func NewPathManager() *PathManager {
	return &PathManager{
		OriginalPath: os.Getenv("PATH"),
		Prepended:    []string{},
	}
}

// Prepend adds directories to the PATH with precedence.
func (pm *PathManager) Prepend(dirs ...string) {
	for _, dir := range dirs {
		absPath, err := filepath.Abs(dir)
		if err != nil {
			fmt.Printf("Failed to resolve absolute path for %q: %v\n", dir, err)
			continue
		}
		pm.Prepended = append(pm.Prepended, absPath)
	}
}

// GetPath returns the updated PATH.
func (pm *PathManager) GetPath() string {
	return fmt.Sprintf("%s%c%s",
		strings.Join(pm.Prepended, string(os.PathListSeparator)),
		os.PathListSeparator,
		pm.OriginalPath,
	)
}

// Apply updates the PATH environment variable globally.
func (pm *PathManager) Apply() error {
	return os.Setenv("PATH", pm.GetPath())
}

// Determine if running in a CI environment
func isCIEnvironment() bool {
	return os.Getenv("CI") != ""
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

// Drop any lines matched by the ignore patterns so they do not affect the comparison
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

// Simulate TTY command execution with optional stdin and proper stdout redirection
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
		_, err := buffer.ReadFrom(ptmx)
		done <- ptyError(err) // Wrap the error handling
	}()

	err = cmd.Wait()
	if err != nil {
		t.Logf("Command execution error: %v", err)
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

// loadTestSuites loads and merges all .yaml files from the test-cases directory
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

// Entry point for tests to parse flags and handle setup/teardown
func TestMain(m *testing.M) {
	// Declare err in the function's scope
	var err error

	// Ensure that Lipgloss uses terminal colors for tests
	lipgloss.SetColorProfile(termenv.TrueColor)

	// Capture the starting working directory
	startingDir, err = os.Getwd()
	if err != nil {
		fmt.Printf("Failed to get the current working directory: %v\n", err)
		os.Exit(1) // Exit with a non-zero code to indicate failure
	}

	fmt.Printf("Starting directory: %s\n", startingDir)
	// Define the base directory for snapshots relative to startingDir
	snapshotBaseDir = filepath.Join(startingDir, "snapshots")

	flag.Parse() // Parse command-line flags
	os.Exit(m.Run())
}

func runCLICommandTest(t *testing.T, tc TestCase) {
	defer func() {
		// Change back to the original working directory after the test
		if err := os.Chdir(startingDir); err != nil {
			t.Fatalf("Failed to change back to the starting directory: %v", err)
		}
	}()

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
	defer os.RemoveAll(tempDir) // Clean up the temporary directory after the test

	// Set environment variables for the test case
	tc.Env["HOME"] = tempDir
	tc.Env["XDG_CONFIG_HOME"] = filepath.Join(tempDir, ".config")
	tc.Env["XDG_CACHE_HOME"] = filepath.Join(tempDir, ".cache")
	tc.Env["XDG_DATA_HOME"] = filepath.Join(tempDir, ".local", "share")

	// Copy necessary files to the temporary HOME directory
	// This includes .gitconfig, .ssh, and .netrc
	// On GitHub Runners for macOS, the .gitconfig is critical for git to work
	originalHome := os.Getenv("HOME")
	filesToCopy := []string{".gitconfig", ".ssh", ".netrc"} // Expand list if needed
	for _, file := range filesToCopy {
		src := filepath.Join(originalHome, file)
		dest := filepath.Join(tempDir, file)

		if _, err := os.Stat(src); err == nil { // Check if the file/directory exists
			//t.Logf("Copying %s to %s\n", src, dest)
			if err := copy.Copy(src, dest); err != nil {
				t.Fatalf("Failed to copy %s to test folder: %v", src, err)
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

		// Clean the directory if enabled
		if tc.Clean {
			t.Logf("Cleaning directory: %q", tc.Workdir)
			if err := cleanDirectory(t, absoluteWorkdir); err != nil {
				t.Fatalf("Failed to clean directory %q: %v", tc.Workdir, err)
			}
		}
	}

	// Check if the binary exists
	binaryPath, err := exec.LookPath(tc.Command)
	if err != nil {
		t.Fatalf("Binary not found: %s. Current PATH: %s", tc.Command, os.Getenv("PATH"))
	}

	// Prepare the command using the context
	cmd := exec.CommandContext(ctx, binaryPath, tc.Args...)

	// Set environment variables
	envVars := os.Environ()
	for key, value := range tc.Env {
		//t.Logf("Setting env: %s=%s", key, value)
		envVars = append(envVars, fmt.Sprintf("%s=%s", key, value))
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
				exitCode := exitErr.ExitCode()

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

	// Validate stdout
	if !verifyOutput(t, "stdout", stdout.String(), tc.Expect.Stdout) {
		t.Errorf("Stdout mismatch for test: %s", tc.Name)
	}

	// Validate stderr
	if !verifyOutput(t, "stderr", stderr.String(), tc.Expect.Stderr) {
		t.Errorf("Stderr mismatch for test: %s", tc.Name)
	}

	// Validate file existence
	if !verifyFileExists(t, tc.Expect.FileExists) {
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

func TestCLICommands(t *testing.T) {
	// Initialize PathManager and update PATH
	pathManager := NewPathManager()
	pathManager.Prepend("../build", "..")
	err := pathManager.Apply()
	if err != nil {
		t.Fatalf("Failed to apply updated PATH: %v", err)
	}
	fmt.Printf("Updated PATH: %s\n", pathManager.GetPath())

	// Load test suite
	testSuite, err := loadTestSuites("test-cases")
	if err != nil {
		t.Fatalf("Failed to load test suites: %v", err)
	}

	for _, tc := range testSuite.Tests {
		if !tc.Enabled {
			t.Logf("Skipping disabled test: %s", tc.Name)
			continue
		}

		// Check OS condition for skipping
		if !verifyOS(t, []MatchPattern{tc.Skip.OS}) {
			t.Logf("Skipping test due to OS condition: %s", tc.Name)
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
			t.Logf("Invalid OS regex pattern: %q, error: %v", pattern.Pattern, err)
			success = false
			continue
		}

		// Check if the current OS matches the pattern
		match := re.MatchString(currentOS)
		if pattern.Negate && match {
			t.Logf("Reason: OS %q matched negated pattern %q.", currentOS, pattern.Pattern)
			success = false
		} else if !pattern.Negate && !match {
			t.Logf("Reason: OS %q did not match pattern %q.", currentOS, pattern.Pattern)
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

func updateSnapshot(fullPath, output string) {
	err := os.MkdirAll(filepath.Dir(fullPath), 0755) // Ensure parent directories exist
	if err != nil {
		panic(fmt.Sprintf("Failed to create snapshot directory: %v", err))
	}
	err = os.WriteFile(fullPath, []byte(output), 0644) // Write snapshot
	if err != nil {
		panic(fmt.Sprintf("Failed to write snapshot file: %v", err))
	}
}

func readSnapshot(t *testing.T, fullPath string) string {
	data, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("Error reading snapshot file %q: %v", fullPath, err)
	}
	return string(data)
}

// Generate a unified diff using gotextdiff
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

// Generate a diff using diffmatchpatch
func DiffStrings(x, y string) string {
	dmp := diffmatchpatch.New()
	diffs := dmp.DiffMain(x, y, false)
	dmp.DiffCleanupSemantic(diffs) // Clean up the diff for readability
	return dmp.DiffPrettyText(diffs)
}

// Colorize diff output based on the threshold
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

func verifySnapshot(t *testing.T, tc TestCase, stdoutOutput, stderrOutput string, regenerate bool) bool {
	if !tc.Snapshot {
		return true
	}

	testName := sanitizeTestName(t.Name())
	stdoutFileName := fmt.Sprintf("%s.stdout.golden", testName)
	stderrFileName := fmt.Sprintf("%s.stderr.golden", testName)
	stdoutPath := filepath.Join(snapshotBaseDir, stdoutFileName)
	stderrPath := filepath.Join(snapshotBaseDir, stderrFileName)

	// Regenerate snapshots if the flag is set
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

// Clean up untracked files in the working directory
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

// findGitRepo finds the Git repository root
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
