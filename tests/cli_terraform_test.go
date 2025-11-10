package tests

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCLITerraformClean(t *testing.T) {
	// Skip if atmosRunner is not initialized
	if atmosRunner == nil || skipReason != "" {
		t.Skipf("Skipping test: %s", skipReason)
	}

	err := os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
	assert.NoError(t, err, "Unset 'ATMOS_CLI_CONFIG_PATH' environment variable should execute without error")
	err = os.Unsetenv("ATMOS_BASE_PATH")
	assert.NoError(t, err, "Unset 'ATMOS_BASE_PATH' environment variable should execute without error")

	// Define the work directory and change to it
	workDir := "fixtures/scenarios/basic"
	t.Chdir(workDir)

	// Force clean everything (no TTY available in CI)
	runTerraformCleanCommand(t, "--force")
	// Clean specific component with force (no TTY available in CI)
	runTerraformCleanCommand(t, "mycomponent", "--force")
	// Clean component with stack with force (no TTY available in CI)
	runTerraformCleanCommand(t, "mycomponent", "-s", "nonprod", "--force")

	// Run terraform apply for prod environment
	runTerraformApply(t, "prod")
	verifyStateFilesExist(t, []string{"./components/terraform/mock/terraform.tfstate.d/prod-mycomponent"})
	runCLITerraformCleanComponent(t, "prod")
	verifyStateFilesDeleted(t, []string{"./components/terraform/mock/terraform.tfstate.d/prod-mycomponent"})

	// Run terraform apply for nonprod environment
	runTerraformApply(t, "nonprod")

	// Verify if state files exist before cleaning
	stateFiles := []string{
		"./components/terraform/mock/.terraform",
		"./components/terraform/mock/terraform.tfstate.d",
	}
	verifyStateFilesExist(t, stateFiles)

	// Run terraform clean
	runTerraformClean(t)

	// Verify if state files have been deleted after clean
	verifyStateFilesDeleted(t, stateFiles)
}

// runTerraformApply runs the terraform apply command for a given environment.
func runTerraformApply(t *testing.T, environment string) {
	cmd := atmosRunner.Command("terraform", "apply", "mycomponent", "-s", environment)
	envVars := os.Environ()
	envVars = append(envVars, "ATMOS_COMPONENTS_TERRAFORM_APPLY_AUTO_APPROVE=true")
	cmd.Env = envVars

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		t.Logf("Terraform stdout:\n%s", stdout.String())
		t.Logf("Terraform stderr:\n%s", stderr.String())
		t.Fatalf("Failed to run terraform apply mycomponent -s %s: %v", environment, stderr.String())
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
func runTerraformClean(t *testing.T) {
	cmd := atmosRunner.Command("terraform", "clean", "--force")
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

func runCLITerraformCleanComponent(t *testing.T, environment string) {
	cmd := atmosRunner.Command("terraform", "clean", "mycomponent", "-s", environment, "--force")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	t.Logf("Clean command output:\n%s", stdout.String())
	if err != nil {
		t.Fatalf("Failed to run terraform clean: %v", stderr.String())
	}
}

func runTerraformCleanCommand(t *testing.T, args ...string) {
	cmdArgs := append([]string{"terraform", "clean"}, args...)
	cmd := atmosRunner.Command(cmdArgs...)
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
