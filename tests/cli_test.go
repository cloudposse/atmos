package tests

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath" // For resolving absolute paths
	"regexp"
	"strings"
	"testing"

	"github.com/creack/pty"
	"github.com/sergi/go-diff/diffmatchpatch"
	"gopkg.in/yaml.v3"
)

// Command-line flag for regenerating snapshots
var regenerateSnapshots = flag.Bool("regenerate-snapshots", false, "Regenerate all golden snapshots")
var startingDir string
var snapshotBaseDir string

type Expectation struct {
	Stdout       []string            `yaml:"stdout"`
	Stderr       []string            `yaml:"stderr"`
	ExitCode     int                 `yaml:"exit_code"`
	FileExists   []string            `yaml:"file_exists"`
	FileContains map[string][]string `yaml:"file_contains"`
	Diff         []string            `yaml:"diff"` // Acceptable differences in snapshot
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
}

type TestSuite struct {
	Tests []TestCase `yaml:"tests"`
}

func loadTestSuite(filePath string) (*TestSuite, error) {
	data, err := ioutil.ReadFile(filePath)
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
		if suite.Tests[i].Expect.Diff == nil {
			suite.Tests[i].Expect.Diff = []string{}
		}
		if !suite.Tests[i].Snapshot {
			suite.Tests[i].Snapshot = false
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

// Apply regex ignore patterns to text (by replacing them with empty strings)
func applyIgnorePatterns(input string, patterns []string) string {
	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		input = re.ReplaceAllString(input, "")
	}
	return input
}

// Simulate TTY command execution
func simulateTtyCommand(cmd *exec.Cmd) (string, error) {
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return "", fmt.Errorf("failed to start TTY: %v", err)
	}
	defer func() { _ = ptmx.Close() }() // Best effort cleanup

	var buffer bytes.Buffer
	_, err = buffer.ReadFrom(ptmx)
	if err != nil {
		return "", fmt.Errorf("failed to read TTY output: %v", err)
	}
	return buffer.String(), nil
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

	// Capture the starting working directory
	startingDir, err = os.Getwd()
	if err != nil {
		fmt.Printf("Failed to get the current working directory: %v\n", err)
		os.Exit(1) // Exit with a non-zero code to indicate failure
	}

	// Define the base directory for snapshots relative to startingDir
	snapshotBaseDir = filepath.Join(startingDir, "snapshots")

	flag.Parse() // Parse command-line flags
	os.Exit(m.Run())
}

func TestCLICommands(t *testing.T) {
	// Declare err in the function's scope
	var err error

	// Initialize PathManager and update PATH
	pathManager := NewPathManager()
	pathManager.Prepend("../build", "..")
	err = pathManager.Apply()
	if err != nil {
		t.Fatalf("Failed to apply updated PATH: %v", err)
	}
	fmt.Printf("Updated PATH: %s\n", pathManager.GetPath())

	// Update the test suite loading
	testSuite, err := loadTestSuites("test-cases")
	if err != nil {
		t.Fatalf("Failed to load test suites: %v", err)
	}

	for _, tc := range testSuite.Tests {

		if !tc.Enabled {
			t.Logf("Skipping disabled test: %s", tc.Name)
			continue
		}

		t.Run(tc.Name, func(t *testing.T) {
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
				t.Fatalf("Binary not found: %s. Current PATH: %s", tc.Command, pathManager.GetPath())
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
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			var exitCode int
			if tc.Tty {
				// Run the command in a pseudo-terminal
				ptyOutput, err := simulateTtyCommand(cmd)
				if err != nil {
					if exitErr, ok := err.(*exec.ExitError); ok {
						exitCode = exitErr.ExitCode()
					} else {
						t.Fatalf("Failed to run TTY command: %v", err)
					}
				}
				stdout.WriteString(ptyOutput)
			} else {
				// Run the command directly and capture the exit code
				err := cmd.Run()
				if err != nil {
					if exitErr, ok := err.(*exec.ExitError); ok {
						exitCode = exitErr.ExitCode()
					} else {
						t.Fatalf("Failed to run command: %v", err)
					}
				}
			}

			// Validate exit code
			if !verifyExitCode(t, tc.Expect.ExitCode, exitCode) {
				t.Errorf("Description: %s", tc.Description)
			}

			// Validate stdout
			if !verifyOutput(t, "stdout", stdout.String(), tc.Expect.Stdout) {
				t.Errorf("Description: %s", tc.Description)
			}

			// Validate stderr
			if !verifyOutput(t, "stderr", stderr.String(), tc.Expect.Stderr) {
				t.Errorf("Description: %s", tc.Description)
			}

			// Validate file existence
			if !verifyFileExists(t, tc.Expect.FileExists) {
				t.Errorf("Description: %s", tc.Description)
			}

			// Validate file contents
			if !verifyFileContains(t, tc.Expect.FileContains) {
				t.Errorf("Description: %s", tc.Description)
			}

			// Validate snapshots for stdout and stderr
			if !verifySnapshot(t, tc, stdout.String(), stderr.String(), *regenerateSnapshots) {
				t.Errorf("Description: %s", tc.Description)
			}
		})
	}
}

func verifyExitCode(t *testing.T, expected, actual int) bool {
	success := true
	if expected != actual {
		t.Errorf("Reason: Expected exit code %d, got %d", expected, actual)
		success = false
	}
	return success
}

func verifyOutput(t *testing.T, outputType, output string, patterns []string) bool {
	success := true
	for _, pattern := range patterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			t.Errorf("Invalid %s regex: %q, error: %v", outputType, pattern, err)
			success = false
			continue
		}
		if !re.MatchString(output) {
			t.Errorf("Reason: %s did not match pattern %q.", outputType, pattern)
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

func verifyFileContains(t *testing.T, filePatterns map[string][]string) bool {
	success := true
	for file, patterns := range filePatterns {
		content, err := ioutil.ReadFile(file)
		if err != nil {
			t.Errorf("Reason: Failed to read file %q: %v", file, err)
			success = false
			continue
		}
		for _, pattern := range patterns {
			re, err := regexp.Compile(pattern)
			if err != nil {
				t.Errorf("Invalid regex for file %q: %q, error: %v", file, pattern, err)
				success = false
				continue
			}
			if !re.Match(content) {
				t.Errorf("Reason: File %q did not match pattern %q.", file, pattern)
				t.Errorf("Content: %q", string(content))
				success = false
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
func DiffStrings(x, y string) string {
	dmp := diffmatchpatch.New()
	diffs := dmp.DiffMain(x, y, false)
	dmp.DiffCleanupSemantic(diffs) // Clean up the diff for readability
	return dmp.DiffPrettyText(diffs)
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
$ go test -run=%q -regenerate-snapshots`, stdoutPath, t.Name())
	}
	filteredStdoutActual := applyIgnorePatterns(stdoutOutput, tc.Expect.Diff)
	filteredStdoutExpected := applyIgnorePatterns(readSnapshot(t, stdoutPath), tc.Expect.Diff)

	if filteredStdoutExpected != filteredStdoutActual {
		diff := DiffStrings(filteredStdoutExpected, filteredStdoutActual)
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
		diff := DiffStrings(filteredStderrExpected, filteredStderrActual)
		t.Errorf("Stderr mismatch for %q:\n%s", stderrPath, diff)
	}

	return true
}
