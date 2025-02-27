package tests

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
func TestCLITerraformENV(t *testing.T) {
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
	workDir := "../tests/fixtures/scenarios/env"
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("Failed to change directory to %q: %v", workDir, err)
	}

	// Find the binary path for "atmos"
	binaryPath, err := exec.LookPath("atmos")
	if err != nil {
		t.Fatalf("Binary not found: %s. Current PATH: %s", "atmos", pathManager.GetPath())
	}
	cmd := exec.Command(binaryPath, "terraform", "apply", "env-example", "-s", "dev")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	t.Log(stdout.String())
	if err != nil {
		t.Fatalf("Failed to run terraform apply env-example -s dev: %v", stderr.String())
	}
	// Check ATMOS_BASE_PATH and ATMOS_CLI_CONFIG_PATH exported on stdout
	ATMOS_BASE_PATH := "ATMOS_BASE_PATH"
	ATMOS_CLI_CONFIG_PATH := "ATMOS_CLI_CONFIG_PATH"
	if !strings.Contains(stdout.String(), ATMOS_BASE_PATH) {
		t.Errorf("Expected output not found in stdout: %s", ATMOS_BASE_PATH)
	}
	if !strings.Contains(stdout.String(), ATMOS_CLI_CONFIG_PATH) {
		t.Errorf("Expected output not found in stdout: %s", ATMOS_CLI_CONFIG_PATH)
	}

}
