package tests

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath" // For resolving absolute paths
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/creack/pty"
	"github.com/go-git/go-git/v5"
	"github.com/hexops/gotextdiff"
	"github.com/hexops/gotextdiff/myers"
	"github.com/hexops/gotextdiff/span"
	"github.com/muesli/termenv"
	"github.com/sergi/go-diff/diffmatchpatch"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"
)

// Command-line flag for regenerating snapshots
var (
	regenerateSnapshots = flag.Bool("regenerate-snapshots", false, "Regenerate all golden snapshots")
	startingDir         string
	snapshotBaseDir     string
)

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
}
type TestCase struct {
	Name        string            `yaml:"name"`
	Description string            `yaml:"description"`
	Enabled     bool              `yaml:"enabled"`
	Workdir     string            `yaml:"workdir"`
	Command     string            `yaml:"command"`
	Args        []string          `yaml:"args"`
	Env         map[string]string `yaml:"env"`
	Expect      Expectation       `yaml:"expect"`
	Tty         bool              `yaml:"tty"`
	Snapshot    bool              `yaml:"snapshot"`
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
	// Check for common CI environment variables
	// Note, that the CI variable has many possible truthy values, so we check for any non-empty value that is not "false".
	return (os.Getenv("CI") != "" && os.Getenv("CI") != "false") || os.Getenv("GITHUB_ACTIONS") == "true"
}

// collapseExtraSlashes replaces multiple consecutive slashes with a single slash.
func collapseExtraSlashes(s string) string {
	// Regex to detect URLs (e.g., https://, s3://, file://, github.com/org/repo.git//)
	urlPattern := regexp.MustCompile(`\b[a-zA-Z][a-zA-Z0-9+\-.]*://[^\s]+`)

	// Find all URLs in the string
	urls := urlPattern.FindAllString(s, -1)

	// Placeholder to replace URLs temporarily
	placeholder := "\x01URL_PLACEHOLDER\x02"
	modified := urlPattern.ReplaceAllString(s, placeholder)

	// Collapse extra slashes in the non-URL portion
	modified = regexp.MustCompile("/+").ReplaceAllString(modified, "/")

	// Restore URLs back in place
	for _, url := range urls {
		modified = strings.Replace(modified, placeholder, url, 1)
	}

	return modified
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
	normalizedOutput := filepath.ToSlash(output)

	// 3. Build a regex that matches the repository root even if extra slashes appear.
	//    First, escape any regex metacharacters in the normalized repository root.
	quoted := regexp.QuoteMeta(normalizedRepoRoot)
	// Replace each literal "/" with the regex token "/+" so that e.g. "a/b/c" becomes "a/+b/+c".
	patternBody := strings.ReplaceAll(quoted, "/", "/+")
	// Allow for extra trailing slashes.
	pattern := patternBody + "/*"
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
	// 6. Replace lines atmos-import log
	filePathRegex := regexp.MustCompile(`file_path=[^ ]+/atmos-import-\d+/atmos-import-\d+\.yaml`)
	result = filePathRegex.ReplaceAllString(result, "file_path=/atmos-import/atmos-import.yaml")

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

// Execute the command and return the exit code
func executeCommand(t *testing.T, cmd *exec.Cmd) int {
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		t.Fatalf("Command execution failed: %v", err)
	}
	return 0
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

	// Change to the specified working directory
	if tc.Workdir != "" {
		err := os.Chdir(tc.Workdir)
		if err != nil {
			t.Fatalf("Failed to change directory to %q: %v", tc.Workdir, err)
		}
	}

	// Check if the binary exists
	binaryPath, err := exec.LookPath(tc.Command)
	if err != nil {
		t.Fatalf("Binary not found: %s. Current PATH: %s", tc.Command, os.Getenv("PATH"))
	}

	// Prepare the command
	cmd := exec.Command(binaryPath, tc.Args...)

	// Set environment variables
	envVars := os.Environ()
	for key, value := range tc.Env {
		envVars = append(envVars, fmt.Sprintf("%s=%s", key, value))
	}
	cmd.Env = envVars

	var stdout, stderr bytes.Buffer
	var exitCode int

	if tc.Tty {
		// Run the command in TTY mode
		ptyOutput, err := simulateTtyCommand(t, cmd, "")
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				// Capture the actual exit code
				exitCode = exitErr.ExitCode()
			} else {
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
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				// Capture the actual exit code
				exitCode = exitErr.ExitCode()
			} else {
				t.Fatalf("Failed to run command; Error %v", err)
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

		// Run with `t.Run` for non-TTY tests
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
	// output, err := sanitizeOutput(stdOutput)
	// if err != nil {
	// 	t.Fatalf("failed to sanitize stdout output: %v", err)
	// }
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
	err := os.MkdirAll(filepath.Dir(fullPath), 0o755) // Ensure parent directories exist
	if err != nil {
		panic(fmt.Sprintf("Failed to create snapshot directory: %v", err))
	}
	err = os.WriteFile(fullPath, []byte(output), 0o644) // Write snapshot
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
$ go test -run=%q -regenerate-snapshots`, stdoutPath, t.Name())
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
		t.Errorf("Stderr mismatch for %q:\n%s", stdoutPath, diff)
	}

	return true
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
