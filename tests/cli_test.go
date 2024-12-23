package tests

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"regexp"
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
			if !verifyOutput(stdout.String(), tc.Expect.Stdout) {
				t.Errorf("Description: %s", tc.Description)
				t.Errorf("Reason: Stdout did not match expected patterns.")
				t.Errorf("Output: %q", stdout.String())
			}

			// Validate stderr
			if !verifyOutput(stderr.String(), tc.Expect.Stderr) {
				t.Errorf("Description: %s", tc.Description)
				t.Errorf("Reason: Stderr did not match expected patterns.")
				t.Errorf("Output: %q", stderr.String())
			}

			// Validate file existence
			if !verifyFileExists(tc.Expect.FileExists) {
				t.Errorf("Description: %s", tc.Description)
			}

			// Validate file contents
			if !verifyFileContains(tc.Expect.FileContains) {
				t.Errorf("Description: %s", tc.Description)
			}
		})
	}
}

func verifyOutput(output string, patterns []string) bool {
	for _, pattern := range patterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return false
		}
		if !re.MatchString(output) {
			return false
		}
	}
	return true
}

func verifyFileExists(files []string) bool {
	for _, file := range files {
		if _, err := os.Stat(file); errors.Is(err, os.ErrNotExist) {
			return false
		}
	}
	return true
}

func verifyFileContains(filePatterns map[string][]string) bool {
	for file, patterns := range filePatterns {
		content, err := ioutil.ReadFile(file)
		if err != nil {
			return false
		}
		for _, pattern := range patterns {
			re, err := regexp.Compile(pattern)
			if err != nil {
				return false
			}
			if !re.Match(content) {
				return false
			}
		}
	}
	return true
}
