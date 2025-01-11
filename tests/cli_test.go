package tests

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	u "github.com/cloudposse/atmos/pkg/utils"
)

type Expectation struct {
	Stdout       []string            `yaml:"stdout"`
	Stderr       []string            `yaml:"stderr"`
	ExitCode     int                 `yaml:"exit_code"`
	FileExists   []string            `yaml:"file_exists"`
	FileContains map[string][]string `yaml:"file_contains"`
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
}

type TestSuite struct {
	Tests []TestCase `yaml:"tests"`
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
			u.TestLogf(nil, u.TestVerbosityVerbose, "Failed to resolve absolute path for %q: %v\n", dir, err)
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

func verifyOutput(t *testing.T, outputType, output string, patterns []string) bool {
	success := true
	for _, pattern := range patterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			u.LogTestFailure(t, "", pattern, err, fmt.Sprintf("Invalid %s regex pattern", outputType))
			t.Error("Test failed due to invalid regex pattern")
			success = false
			continue
		}
		if !re.MatchString(output) {
			u.LogTestFailure(t, "", pattern, output,
				fmt.Sprintf("%s output did not match expected pattern", outputType),
				fmt.Sprintf("Full %s output:", outputType),
				output)
			t.Error("Test failed due to pattern mismatch")
			success = false
		}
	}
	return success
}

func verifyFileExists(t *testing.T, files []string) bool {
	success := true
	for _, file := range files {
		if _, err := os.Stat(file); errors.Is(err, os.ErrNotExist) {
			u.LogTestFailure(t, "", file, "file does not exist",
				"Expected file to exist but it does not")
			t.Error("Test failed due to missing file")
			success = false
		}
	}
	return success
}

func verifyFileContains(t *testing.T, filePatterns map[string][]string) bool {
	success := true
	for file, patterns := range filePatterns {
		content, err := os.ReadFile(file)
		if err != nil {
			u.LogTestFailure(t, "", file, err,
				fmt.Sprintf("Failed to read file contents"))
			t.Error("Test failed due to file read error")
			success = false
			continue
		}
		for _, pattern := range patterns {
			re, err := regexp.Compile(pattern)
			if err != nil {
				u.LogTestFailure(t, "", pattern, err,
					fmt.Sprintf("Invalid regex pattern for file %s", file))
				t.Error("Test failed due to invalid file content pattern")
				success = false
				continue
			}
			if !re.Match(content) {
				u.LogTestFailure(t, "", pattern, string(content),
					fmt.Sprintf("File %s content did not match pattern", file),
					"File contents:",
					string(content))
				t.Error("Test failed due to file content mismatch")
				success = false
			}
		}
	}
	return success
}

func TestCLICommands(t *testing.T) {
	// Capture the starting working directory
	startingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get the current working directory: %v", err)
	}

	// Initialize PATH manager and update PATH
	pathManager := NewPathManager()
	pathManager.Prepend("../build", "..")
	err = pathManager.Apply()
	if err != nil {
		t.Fatalf("Failed to apply updated PATH: %v", err)
	}

	// Log PATH only in verbose mode
	if u.GetTestVerbosity() >= u.TestVerbosityVerbose {
		u.TestLogf(t, u.TestVerbosityVerbose, "Updated PATH: %s", pathManager.GetPath())
	}

	// Update the test suite loading
	testSuite, err := loadTestSuites("test-cases")
	if err != nil {
		t.Fatalf("Failed to load test suites: %v", err)
	}

	for _, tc := range testSuite.Tests {
		if !tc.Enabled {
			// Log skipped tests based on verbosity
			if u.GetTestVerbosity() >= u.TestVerbosityNormal {
				u.TestLogf(t, u.TestVerbosityNormal, "Skipping disabled test: %s", tc.Name)
			}
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

			// Run the command
			err = cmd.Run()

			// Validate exit code
			exitCode := 0
			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					exitCode = exitErr.ExitCode()
				}
			}

			if exitCode != tc.Expect.ExitCode {
				u.LogTestFailure(t, tc.Description, tc.Expect.ExitCode, exitCode,
					"Command failed with unexpected exit code",
					fmt.Sprintf("Command: %s %v", tc.Command, tc.Args),
					fmt.Sprintf("Stdout:\n%s", stdout.String()),
					fmt.Sprintf("Stderr:\n%s", stderr.String()))
				return
			}

			if !verifyOutput(t, "stdout", stdout.String(), tc.Expect.Stdout) {
				return
			}

			if !verifyOutput(t, "stderr", stderr.String(), tc.Expect.Stderr) {
				return
			}

			if !verifyFileExists(t, tc.Expect.FileExists) {
				return
			}

			if !verifyFileContains(t, tc.Expect.FileContains) {
				return
			}

			// Log success message only in verbose mode
			if u.GetTestVerbosity() >= u.TestVerbosityVerbose {
				u.TestLogf(t, u.TestVerbosityVerbose, "Test passed: %s", tc.Description)
			}
		})
	}
}
