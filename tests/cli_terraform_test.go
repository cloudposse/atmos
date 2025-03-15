package tests

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCLITerraformClean(t *testing.T) {
	// Capture the starting working directory
	startingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get the current working directory: %v", err)
	}

	// Initialize PathManager and update PATH
	pathManager := NewPathManager()
	pathManager.Prepend("../build", "..")
	err = pathManager.Apply()
	if err != nil {
		t.Fatalf("Failed to apply updated PATH: %v", err)
	}
	fmt.Printf("Updated PATH: %s\n", pathManager.GetPath())
	defer func() {
		// Change back to the original working directory after the test
		if err := os.Chdir(startingDir); err != nil {
			t.Fatalf("Failed to change back to the starting directory: %v", err)
		}
	}()

	// Define the work directory and change to it
	workDir := "../examples/quick-start-simple"
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("Failed to change directory to %q: %v", workDir, err)
	}

	// Find the binary path for "atmos"
	binaryPath, err := exec.LookPath("atmos")
	if err != nil {
		t.Fatalf("Binary not found: %s. Current PATH: %s", "atmos", pathManager.GetPath())
	}

	// Force clean everything
	runTerraformCleanCommand(t, binaryPath, "--force")
	// Clean everything
	runTerraformCleanCommand(t, binaryPath)
	// Clean specific component
	runTerraformCleanCommand(t, binaryPath, "station")
	// Clean component with stack
	runTerraformCleanCommand(t, binaryPath, "station", "-s", "dev")

	// Run terraform apply for prod environment
	runTerraformApply(t, binaryPath, "prod")
	verifyStateFilesExist(t, []string{"./components/terraform/weather/terraform.tfstate.d/prod-station"})
	runCLITerraformCleanComponent(t, binaryPath, "prod")
	verifyStateFilesDeleted(t, []string{"./components/terraform/weather/terraform.tfstate.d/prod-station"})

	// Run terraform apply for dev environment
	runTerraformApply(t, binaryPath, "dev")

	// Verify if state files exist before cleaning
	stateFiles := []string{
		"./components/terraform/weather/.terraform",
		"./components/terraform/weather/terraform.tfstate.d",
		"./components/terraform/weather/.terraform.lock.hcl",
	}
	verifyStateFilesExist(t, stateFiles)

	// Run terraform clean
	runTerraformClean(t, binaryPath)

	// Verify if state files have been deleted after clean
	verifyStateFilesDeleted(t, stateFiles)
}

// runTerraformApply runs the terraform apply command for a given environment.
func runTerraformApply(t *testing.T, binaryPath, environment string) {
	cmd := exec.Command(binaryPath, "terraform", "apply", "station", "-s", environment)
	envVars := os.Environ()
	envVars = append(envVars, "ATMOS_COMPONENTS_TERRAFORM_APPLY_AUTO_APPROVE=true")
	cmd.Env = envVars

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	t.Log(stdout.String())
	if err != nil {
		t.Fatalf("Failed to run terraform apply station -s %s: %v", environment, stderr.String())
	}
}

// verifyStateFilesExist checks if the state files exist before cleaning.
func verifyStateFilesExist(t *testing.T, stateFiles []string) {
	for _, file := range stateFiles {
		fileAbs, err := filepath.Abs(file)
		if err != nil {
			t.Fatalf("Failed to resolve absolute path for %q: %v", file, err)
		}
		if _, err := os.Stat(fileAbs); errors.Is(err, os.ErrNotExist) {
			t.Errorf("Expected file to exist before cleaning: %q", fileAbs)
		}
	}
}

// runTerraformClean runs the terraform clean command.
func runTerraformClean(t *testing.T, binaryPath string) {
	cmd := exec.Command(binaryPath, "terraform", "clean", "--force")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	t.Logf("Clean command output:\n%s", stdout.String())
	if err != nil {
		t.Fatalf("Failed to run terraform clean: %v", stderr.String())
	}
}

// verifyStateFilesDeleted checks if the state files have been deleted after cleaning.
func verifyStateFilesDeleted(t *testing.T, stateFiles []string) {
	for _, file := range stateFiles {
		fileAbs, err := filepath.Abs(file)
		if err != nil {
			t.Fatalf("Failed to resolve absolute path for %q: %v", file, err)
		}
		_, err = os.Stat(fileAbs)
		if err == nil {
			t.Errorf("Expected Terraform state file to be deleted: %q", fileAbs)
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Errorf("Unexpected error checking file %q: %v", fileAbs, err)
		}
	}
}

func runCLITerraformCleanComponent(t *testing.T, binaryPath, environment string) {
	cmd := exec.Command(binaryPath, "terraform", "clean", "station", "-s", environment, "--force")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	t.Logf("Clean command output:\n%s", stdout.String())
	if err != nil {
		t.Fatalf("Failed to run terraform clean: %v", stderr.String())
	}
}

func runCLITerraformClean(t *testing.T, binaryPath string) {
	cmd := exec.Command(binaryPath, "terraform", "clean")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	t.Logf("Clean command output:\n%s", stdout.String())
	if err != nil {
		t.Fatalf("Failed to run terraform clean: %v", stderr.String())
	}
}

func runTerraformCleanCommand(t *testing.T, binaryPath string, args ...string) {
	cmdArgs := append([]string{"terraform", "clean"}, args...)
	cmd := exec.Command(binaryPath, cmdArgs...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	t.Logf("Clean command output:\n%s", stdout.String())
	if err != nil {
		t.Fatalf("Failed to run terraform clean: %v", stderr.String())
	}
}
func TestCollapseExtraSlashesHandlesOnlySlashes(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		// Basic cases with only slashes
		{"///", "/"},
		{"/", "/"},
		{"//", "/"},
		{"", ""},

		// Relative paths
		{"..//path", "../path"},
		{"/path//to//file", "/path/to/file"},
		{"./../path", "./../path"}, // No change expected

		// Protocol handling
		{"https://", "https://"},
		{"http://", "http://"},
		{"http://example.com//path//", "http://example.com/path/"},
		{"https:////example.com", "https://example.com"}, // Normalize after protocol
		{"http:/example.com", "http://example.com"},      // Fix missing slashes after protocol

		// Complex URLs
		{"http://example.com:8080//api//v1", "http://example.com:8080/api/v1"},
		{"http://user:pass@example.com//path", "http://user:pass@example.com/path"},

		// Edge cases for trimming
		{"http:////example.com", "http://example.com"}, // Extra slashes after protocol
		{"http:///path", "http://path"},                // Implicit empty authority
	}

	for _, tc := range testCases {
		result := collapseExtraSlashes(tc.input)
		if result != tc.expected {
			t.Errorf("collapseExtraSlashes(%q) = %q, want %q", tc.input, result, tc.expected)
		}
	}
}
