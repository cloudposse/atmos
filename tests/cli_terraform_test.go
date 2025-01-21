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
	workDir := "../examples/quick-start-simple"
	err = os.Chdir(workDir)
	if err != nil {
		t.Fatalf("Failed to change directory to %q: %v", workDir, err)
	}
	binaryPath, err := exec.LookPath("atmos")
	if err != nil {
		t.Fatalf("Binary not found: %s. Current PATH: %s", "atmos", pathManager.GetPath())
	}
	cmdDev := exec.Command(binaryPath, "terraform", "apply", "station", "-s", "dev")
	var stdout, stderr bytes.Buffer
	cmdDev.Stdout = &stdout
	cmdDev.Stderr = &stderr
	// ATMOS_COMPONENTS_TERRAFORM_APPLY_AUTO_APPROVE
	envVarsDev := os.Environ()
	envVarsDev = append(envVarsDev, "ATMOS_COMPONENTS_TERRAFORM_APPLY_AUTO_APPROVE=true")
	cmdDev.Env = envVarsDev

	// run terraform apply station -s dev and terraform apply station -s prod
	err = cmdDev.Run()
	if err != nil {
		t.Log(stdout.String())
		t.Fatalf("Failed to run terraform apply station -s dev: %v", stderr.String())
		return
	}
	cmdProd := exec.Command(binaryPath, "terraform", "apply", "station", "-s", "prod")
	envVarsProd := os.Environ()
	envVarsProd = append(envVarsProd, "ATMOS_COMPONENTS_TERRAFORM_APPLY_AUTO_APPROVE=true")
	cmdProd.Env = envVarsProd
	var stdoutProd, stderrProd bytes.Buffer
	cmdDev.Stdout = &stdoutProd
	cmdDev.Stderr = &stderrProd
	err = cmdProd.Run()
	if err != nil {
		t.Log(stdoutProd.String())
		t.Fatalf("Failed to run terraform apply station -s prod: %v", stderrProd.String())
		return
	}
	// get command error sta
	// check if the state files and directories for the component and stack are exist
	stateFiles := []string{
		"./components/terraform/weather/.terraform",
		"./components/terraform/weather/terraform.tfstate.d",
		"./components/terraform/weather/.terraform.lock.hcl",
	}
	for _, file := range stateFiles {
		fileAbs, err := filepath.Abs(file)
		if err != nil {
			t.Fatalf("Failed to resolve absolute path for %q: %v", file, err)
		}
		if _, err := os.Stat(fileAbs); errors.Is(err, os.ErrNotExist) {
			t.Errorf("Reason: Expected file exist: %q", fileAbs)
			return
		}
	}

	// run atmos terraform clean
	cmdClean := exec.Command(binaryPath, "terraform", "clean", "--force")
	var stdoutClean, stderrClean bytes.Buffer
	cmdClean.Stdout = &stdoutClean
	cmdClean.Stderr = &stderrClean
	err = cmdClean.Run()
	t.Logf("Clean command output:\n%s", stdoutClean.String())
	if err != nil {
		t.Fatalf("Failed to run atmos terraform clean: %v", stderrClean.String())
	}
	// check if the state files and directories for the component and stack are deleted
	for _, file := range stateFiles {
		fileAbs, err := filepath.Abs(file)
		if err != nil {
			t.Fatalf("Failed to resolve absolute path for %q: %v", file, err)
		}
		_, err = os.Stat(fileAbs)
		if err == nil {
			t.Errorf("Expected Terraform state file to be deleted: %q", fileAbs)
			continue
		}
		if !errors.Is(err, os.ErrNotExist) {
			t.Errorf("Unexpected error checking file %q: %v", fileAbs, err)
		}
	}

}
