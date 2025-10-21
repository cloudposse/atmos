package exec

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// PlanFileOptions contains parameters for plan file operations.
type PlanFileOptions struct {
	ComponentPath string
	OrigPlanFile  string
	NewPlanFile   string
	TmpDir        string
}

// TerraformPlanDiff represents the plan-diff command implementation.
func TerraformPlanDiff(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(atmosConfig, "exec.TerraformPlanDiff")()

	// Extract flags and setup paths
	origPlanFile, newPlanFile, err := parsePlanDiffFlags(info.AdditionalArgsAndFlags)
	if err != nil {
		return err
	}

	// Create a temporary directory for all temporary files
	tmpDir, err := os.MkdirTemp("", "atmos-terraform-plan-diff")
	if err != nil {
		return fmt.Errorf("error creating temporary directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Get the component path
	componentPath := filepath.Join(atmosConfig.TerraformDirAbsolutePath, info.ComponentFolderPrefix, info.FinalComponent)

	// Ensure original plan file exists and is absolute
	origPlanFile, err = validateOriginalPlanFile(origPlanFile, componentPath)
	if err != nil {
		return err
	}

	// Handle new plan file (generate one if needed)
	opts := PlanFileOptions{
		ComponentPath: componentPath,
		OrigPlanFile:  origPlanFile,
		NewPlanFile:   newPlanFile,
		TmpDir:        tmpDir,
	}
	newPlanFile, err = prepareNewPlanFile(atmosConfig, info, opts)
	if err != nil {
		return err
	}
	// Compare the plans and generate diff
	return comparePlansAndGenerateDiff(atmosConfig, info, componentPath, origPlanFile, newPlanFile)
}

// parsePlanDiffFlags extracts the orig and new plan file paths from command arguments.
func parsePlanDiffFlags(args []string) (string, string, error) {
	origPlanFile := ""
	newPlanFile := ""

	// Extract command-specific flags
	for i := 0; i < len(args); i++ {
		arg := args[i]

		if strings.HasPrefix(arg, "--orig=") {
			origPlanFile = strings.TrimPrefix(arg, "--orig=")
		} else if arg == "--orig" && i+1 < len(args) {
			origPlanFile = args[i+1]
		}

		if strings.HasPrefix(arg, "--new=") {
			newPlanFile = strings.TrimPrefix(arg, "--new=")
		} else if arg == "--new" && i+1 < len(args) {
			newPlanFile = args[i+1]
		}
	}

	if origPlanFile == "" {
		return "", "", errUtils.ErrOriginalPlanFileRequired
	}

	return origPlanFile, newPlanFile, nil
}

// validateOriginalPlanFile ensures original plan file exists and returns absolute path.
func validateOriginalPlanFile(origPlanFile, componentPath string) (string, error) {
	// Make sure the original plan file exists
	if !filepath.IsAbs(origPlanFile) {
		// If the path is relative, make it absolute based on the component directory
		origPlanFile = filepath.Join(componentPath, origPlanFile)
	}

	if _, err := os.Stat(origPlanFile); os.IsNotExist(err) {
		return "", fmt.Errorf("%w: '%s'", errUtils.ErrOriginalPlanFileNotExist, origPlanFile)
	}

	return origPlanFile, nil
}

// comparePlansAndGenerateDiff compares two plan files and generates a diff.
func comparePlansAndGenerateDiff(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, componentPath, origPlanFile, newPlanFile string) error {
	// Get the JSON representation of the original plan
	origPlanJSON, err := getTerraformPlanJSON(atmosConfig, info, componentPath, origPlanFile)
	if err != nil {
		return fmt.Errorf("error getting JSON for original plan: %w", err)
	}

	// Get the JSON representation of the new plan
	newPlanJSON, err := getTerraformPlanJSON(atmosConfig, info, componentPath, newPlanFile)
	if err != nil {
		return fmt.Errorf("error getting JSON for new plan: %w", err)
	}

	// Parse the JSON
	var origPlan, newPlan map[string]interface{}
	err = json.Unmarshal([]byte(origPlanJSON), &origPlan)
	if err != nil {
		return fmt.Errorf("error parsing original plan JSON: %w", err)
	}

	err = json.Unmarshal([]byte(newPlanJSON), &newPlan)
	if err != nil {
		return fmt.Errorf("error parsing new plan JSON: %w", err)
	}

	// Sort maps to ensure consistent ordering
	origPlan = sortMapKeys(origPlan)
	newPlan = sortMapKeys(newPlan)

	// Generate the diff
	diff, hasDiff := generatePlanDiff(origPlan, newPlan)

	// Print the diff
	if hasDiff {
		fmt.Fprintln(os.Stdout, "\nDiff Output")
		fmt.Fprintln(os.Stdout, "===========")
		fmt.Fprintln(os.Stdout, "")
		fmt.Fprintln(os.Stdout, diff)

		// Print the error message
		errUtils.CheckErrorAndPrint(errUtils.ErrPlanHasDiff, "", "")

		// Exit with code 2 to indicate that the plans are different
		errUtils.OsExit(2)
		return nil // This line will never be reached
	}

	fmt.Fprintln(os.Stdout, "The planfiles are identical")
	return nil
}

// getTerraformPlanJSON gets the JSON representation of a terraform plan.
func getTerraformPlanJSON(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, componentPath, planFile string) (string, error) {
	// Run terraform init before show
	if err := runTerraformInit(atmosConfig, componentPath, info); err != nil {
		return "", err
	}

	// Copy the plan file to the component directory if needed
	planFileInComponentDir, cleanup, err := copyPlanFileIfNeeded(planFile, componentPath)
	if err != nil {
		return "", err
	}
	if cleanup != nil {
		defer cleanup()
	}
	// Run terraform show and capture output
	output, err := runTerraformShow(info, planFileInComponentDir)
	if err != nil {
		return "", err
	}
	// Extract JSON from output
	return extractJSONFromOutput(output)
}

// extractJSONFromOutput extracts the JSON part from terraform show output.
func extractJSONFromOutput(output string) (string, error) {
	// Find the beginning of the JSON output (first '{' character)
	jsonStartIdx := strings.Index(output, "{")
	if jsonStartIdx == -1 {
		return "", errUtils.ErrNoJSONOutput
	}

	// Extract just the JSON part
	jsonOutput := output[jsonStartIdx:]

	return jsonOutput, nil
}

// copyPlanFileIfNeeded copies the plan file to the component directory if it's not already there.
func copyPlanFileIfNeeded(planFile, componentPath string) (string, func(), error) {
	planFileInComponentDir := planFile
	planFileBaseName := filepath.Base(planFile)

	// If the plan file is not in the component directory, create a temporary copy
	if !strings.HasPrefix(planFile, componentPath) {
		planFileInComponentDir = filepath.Join(componentPath, planFileBaseName)

		// Open source file
		src, err := os.Open(planFile)
		if err != nil {
			return "", nil, fmt.Errorf("error opening source plan file: %w", err)
		}
		defer src.Close()

		// Create destination file
		dst, err := os.OpenFile(planFileInComponentDir, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, planFileMode)
		if err != nil {
			return "", nil, fmt.Errorf("error creating destination plan file: %w", err)
		}
		defer dst.Close()

		// Copy the file contents without loading it all into memory
		_, err = io.Copy(dst, src)
		if err != nil {
			return "", nil, fmt.Errorf("error copying plan file to component directory: %w", err)
		}

		// Return a cleanup function
		cleanup := func() {
			os.Remove(planFileInComponentDir)
		}
		return planFileInComponentDir, cleanup, nil
	}

	return planFileInComponentDir, nil, nil
}

// runTerraformShow runs the terraform show command and captures its output.
func runTerraformShow(info *schema.ConfigAndStacksInfo, planFile string) (string, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return "", fmt.Errorf("error creating pipe: %w", err)
	}
	defer r.Close()
	defer w.Close()

	// Save original stdout and replace it with the pipe
	origStdout := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = origStdout }()

	// Use a goroutine to read from the pipe
	var outputBuf bytes.Buffer
	readDone := make(chan error)
	go func() {
		_, err := io.Copy(&outputBuf, r)
		readDone <- err
	}()

	// Set up the show command
	showInfo := *info
	showInfo.SubCommand = "show"
	showInfo.AdditionalArgsAndFlags = []string{"-json", planFile}

	// Run the command
	execErr := ExecuteTerraform(showInfo)

	// Close writer to signal EOF to reader
	w.Close()

	// Wait for reader to finish
	readErr := <-readDone

	if execErr != nil {
		return "", fmt.Errorf("error running terraform show: %w", execErr)
	}
	if readErr != nil {
		return "", fmt.Errorf("error reading output: %w", readErr)
	}

	return outputBuf.String(), nil
}

// runTerraformInit runs a basic terraform init in the specified directory using
// terraformRun method (ExecuteTerraform).
func runTerraformInit(atmosConfig *schema.AtmosConfiguration, dir string, info *schema.ConfigAndStacksInfo) error {
	if info.SkipInit {
		return nil
	}

	// Clean terraform workspace to prevent workspace selection prompt
	cleanTerraformWorkspace(*atmosConfig, dir)

	// Create a copy of the info struct with init subcommand
	initInfo := *info
	initInfo.SubCommand = "init"

	// Add -reconfigure flag conditionally based on config
	if atmosConfig.Components.Terraform.InitRunReconfigure {
		initInfo.AdditionalArgsAndFlags = []string{"-reconfigure"}
	} else {
		initInfo.AdditionalArgsAndFlags = []string{}
	}

	// Run terraform init using ExecuteTerraform.
	err := ExecuteTerraform(initInfo)
	if err != nil {
		return fmt.Errorf("error running terraform init: %w", err)
	}

	return nil
}
