package tests

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath" // For resolving absolute paths
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
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
	data, err := ioutil.ReadFile(filePath)
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

func prependPath(envVars []string, dirs ...string) []string {
	// Resolve absolute paths for the directories
	var resolvedPaths []string
	for _, dir := range dirs {
		absPath, err := filepath.Abs(dir)
		if err != nil {
			fmt.Printf("Failed to resolve absolute path for %q: %v\n", dir, err)
			continue
		}
		resolvedPaths = append(resolvedPaths, absPath)
	}

	// Create the new PATH variable with precedence order
	currentPath := os.Getenv("PATH")
	newPath := fmt.Sprintf("%s%c%s",
		strings.Join(resolvedPaths, string(os.PathListSeparator)),
		os.PathListSeparator,
		currentPath,
	)

	// Replace the PATH in envVars
	for i, env := range envVars {
		if len(env) > 5 && env[:5] == "PATH=" {
			envVars[i] = fmt.Sprintf("PATH=%s", newPath)
			return envVars
		}
	}

	// If PATH is not found, add it
	return append(envVars, fmt.Sprintf("PATH=%s", newPath))
}

func TestCLICommands(t *testing.T) {
	// Capture the starting working directory
	startingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get the current working directory: %v", err)
	}

	testSuite, err := loadTestSuite("test_cases.yaml")
	if err != nil {
		t.Fatalf("Failed to load test suite: %v", err)
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

			// Prepare the command
			cmd := exec.Command(tc.Command, tc.Args...)

			// Set environment variables
			envVars := os.Environ()
			envVars = prependPath(envVars, "./build", ".") // Add ./build and . to PATH in precedence order
			for key, value := range tc.Env {
				envVars = append(envVars, fmt.Sprintf("%s=%s", key, value))
			}
			cmd.Env = envVars

			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			// Run the command
			err := cmd.Run()

			// Validate exit code
			exitCode := 0
			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					exitCode = exitErr.ExitCode()
				}
			}
			if exitCode != tc.Expect.ExitCode {
				t.Errorf("Description: %s", tc.Description)
				t.Errorf("Reason: Expected exit code %d, got %d", tc.Expect.ExitCode, exitCode)
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
		})
	}
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
