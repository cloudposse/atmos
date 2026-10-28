package terraform

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/perf"
	provWorkdir "github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terraform/plandiff"
)

// Static errors.
var (
	// ErrNoJSONOutput is returned when no JSON output is found in terraform show output.
	ErrNoJSONOutput = errors.New("no JSON output found in terraform show output")
)

type TerraformPlanDiffExecutor interface {
	ExecuteTerraform(schema.ConfigAndStacksInfo, ...ShellCommandOption) error
}

type TerraformPlanDiffExecutorFunc func(schema.ConfigAndStacksInfo, ...ShellCommandOption) error

//nolint:gocritic // Preserve the value-based execution contract.
func (f TerraformPlanDiffExecutorFunc) ExecuteTerraform(info schema.ConfigAndStacksInfo, opts ...ShellCommandOption) error {
	defer perf.Track(nil, "terraform.TerraformPlanDiffExecutorFunc.ExecuteTerraform")()

	return f(info, opts...)
}

// PlanFileOptions contains parameters for plan file operations.
type PlanFileOptions struct {
	ComponentPath string
	OrigPlanFile  string
	NewPlanFile   string
	TmpDir        string
}

// TerraformPlanDiff represents the plan-diff command implementation.
//
//nolint:revive // Preserve the existing generation and evaluation order during package extraction.
func (s *Operations) TerraformPlanDiff(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(atmosConfig, "exec.TerraformPlanDiff")()
	if _, err := plandiff.Policy(atmosConfig); err != nil {
		return err
	}

	// Process stacks to resolve component metadata (metadata.component, component_folder_prefix).
	// This is required to get the correct FinalComponent and ComponentFolderPrefix.
	// Without this, plan-diff would look in the wrong directory when the component instance
	// name differs from the actual component (e.g., "foobar-atmos-pro" with metadata.component: "foobar").
	processedInfo, err := s.host.ProcessStacks(atmosConfig, *info, true, true, true, nil, nil)
	if err != nil {
		return fmt.Errorf("error processing stacks for plan-diff: %w", err)
	}

	// Extract flags and setup paths
	origPlanFile, newPlanFile, err := ParsePlanDiffFlags(processedInfo.AdditionalArgsAndFlags)
	if err != nil {
		return err
	}

	// Create a temporary directory for all temporary files
	tmpDir, err := os.MkdirTemp("", "atmos-terraform-plan-diff")
	if err != nil {
		return errors.Wrap(err, "error creating temporary directory")
	}
	defer os.RemoveAll(tmpDir)

	// Get the component path using the resolved FinalComponent from ProcessStacks.
	componentPath := filepath.Join(atmosConfig.TerraformDirAbsolutePath, processedInfo.ComponentFolderPrefix, processedInfo.FinalComponent)

	// For workdir-enabled components, compute the workdir path from first principles
	// because ProcessStacks builds a fresh ComponentSection that does not include
	// WorkdirPathKey (only set by AutoProvisionSource at execution time).
	if provWorkdir.IsWorkdirEnabled(processedInfo.ComponentSection) {
		candidate, exists, resolveErr := component.BuildAndResolveWorkdirPath(atmosConfig, &processedInfo, cfg.TerraformComponentType)
		if resolveErr != nil {
			return resolveErr
		}
		if exists {
			componentPath = candidate
		}
	}

	// Ensure original plan file exists and is absolute
	origPlanFile, err = ValidateOriginalPlanFile(origPlanFile, componentPath)
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
	newPlanFile, err = s.prepareNewPlanFile(atmosConfig, &processedInfo, opts)
	if err != nil {
		return err
	}
	// Compare the plans and generate diff
	return s.ComparePlansAndGenerateDiff(atmosConfig, &processedInfo, componentPath, origPlanFile, newPlanFile)
}

// ParsePlanDiffFlags extracts the orig and new plan file paths from command arguments.
func ParsePlanDiffFlags(args []string) (string, string, error) {
	defer perf.Track(nil, "terraform.ParsePlanDiffFlags")()

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
		return "", "", errors.New("original plan file (--orig) is required")
	}

	return origPlanFile, newPlanFile, nil
}

// ValidateOriginalPlanFile ensures original plan file exists and returns absolute path.
func ValidateOriginalPlanFile(origPlanFile, componentPath string) (string, error) {
	defer perf.Track(nil, "terraform.ValidateOriginalPlanFile")()

	// Make sure the original plan file exists
	if !filepath.IsAbs(origPlanFile) {
		// If the path is relative, make it absolute based on the component directory
		origPlanFile = filepath.Join(componentPath, origPlanFile)
	}

	if _, err := os.Stat(origPlanFile); os.IsNotExist(err) {
		return "", errors.Errorf("original plan file '%s' does not exist", origPlanFile)
	}

	return origPlanFile, nil
}

// ComparePlansAndGenerateDiff compares two plan files and generates a diff.
func (s *Operations) ComparePlansAndGenerateDiff(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, componentPath, origPlanFile, newPlanFile string) error {
	defer perf.Track(atmosConfig, "terraform.Operations.ComparePlansAndGenerateDiff")()

	policy, err := plandiff.Policy(atmosConfig)
	if err != nil {
		return err
	}
	// Get the JSON representation of the original plan
	origPlanJSON, err := s.getTerraformPlanJSON(atmosConfig, info, componentPath, origPlanFile)
	if err != nil {
		return errors.Wrap(err, "error getting JSON for original plan")
	}

	// Get the JSON representation of the new plan
	newPlanJSON, err := s.getTerraformPlanJSON(atmosConfig, info, componentPath, newPlanFile)
	if err != nil {
		return errors.Wrap(err, "error getting JSON for new plan")
	}

	// Parse the JSON
	var origPlan, newPlan map[string]interface{}
	err = json.Unmarshal([]byte(origPlanJSON), &origPlan)
	if err != nil {
		return errors.Wrap(err, "error parsing original plan JSON")
	}

	err = json.Unmarshal([]byte(newPlanJSON), &newPlan)
	if err != nil {
		return errors.Wrap(err, "error parsing new plan JSON")
	}

	// Warn about ignore_changes entries that cannot match anything in these plans.
	plandiff.WarnUnmatchedIgnoreChanges(atmosConfig.Components.Terraform.PlanDiff.IgnoreChanges, origPlan, newPlan)

	// Generate the diff
	diff, hasDiff := plandiff.Compare(origPlan, newPlan, policy)

	// Print the diff
	if hasDiff {
		_ = data.Writeln("\nDiff Output")
		_ = data.Writeln("===========")
		_ = data.Writeln("")
		_ = data.Writeln(diff)

		// Print the error message
		errUtils.CheckErrorAndPrint(errUtils.ErrPlanHasDiff, "", "")

		// Exit with code 2 to indicate that the plans are different
		errUtils.OsExit(2)
		return nil // This line will never be reached
	}

	_ = data.Writeln("The planfiles are identical")
	return nil
}

// getTerraformPlanJSON gets the JSON representation of a terraform plan.
func (s *Operations) getTerraformPlanJSON(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, componentPath, planFile string) (string, error) {
	// Run terraform init before show
	if err := s.runTerraformInit(atmosConfig, componentPath, info); err != nil {
		return "", err
	}

	// Copy the plan file to the component directory if needed
	planFileInComponentDir, cleanup, err := CopyPlanFileIfNeeded(planFile, componentPath)
	if err != nil {
		return "", err
	}
	if cleanup != nil {
		defer cleanup()
	}
	// Run terraform show and capture output
	output, err := s.runTerraformShow(info, planFileInComponentDir)
	if err != nil {
		return "", err
	}
	// Extract JSON from output
	return ExtractJSONFromOutput(output)
}

// ExtractJSONFromOutput extracts the JSON part from terraform show output.
func ExtractJSONFromOutput(output string) (string, error) {
	defer perf.Track(nil, "terraform.ExtractJSONFromOutput")()

	// Find the beginning of the JSON output (first '{' character)
	jsonStartIdx := strings.Index(output, "{")
	if jsonStartIdx == -1 {
		return "", ErrNoJSONOutput
	}

	// Extract just the JSON part
	jsonOutput := output[jsonStartIdx:]

	return jsonOutput, nil
}

// CopyPlanFileIfNeeded copies the plan file to the component directory if it's not already there.
func CopyPlanFileIfNeeded(planFile, componentPath string) (string, func(), error) {
	defer perf.Track(nil, "terraform.CopyPlanFileIfNeeded")()

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
func (s *Operations) runTerraformShow(info *schema.ConfigAndStacksInfo, planFile string) (string, error) {
	return s.RunTerraformShowWithExecutor(info, planFile, TerraformPlanDiffExecutorFunc(s.host.ExecuteTerraform))
}

// RunTerraformShowWithExecutor captures plan JSON through the supplied executor without changing the caller's command metadata.
func (s *Operations) RunTerraformShowWithExecutor(info *schema.ConfigAndStacksInfo, planFile string, executor TerraformPlanDiffExecutor) (string, error) {
	defer perf.Track(nil, "terraform.Operations.RunTerraformShowWithExecutor")()

	var outputBuf bytes.Buffer

	// Set up the show command
	showInfo := *info
	showInfo.SubCommand = "show"
	showInfo.AdditionalArgsAndFlags = []string{"-json", planFile}

	// Run the command
	if execErr := executor.ExecuteTerraform(showInfo, component.WithStdoutOverride(&outputBuf)); execErr != nil {
		return "", fmt.Errorf("error running terraform show: %w", execErr)
	}

	return outputBuf.String(), nil
}

// runTerraformInit runs a basic terraform init in the specified directory using
// terraformRun method (ExecuteTerraform).
func (s *Operations) runTerraformInit(atmosConfig *schema.AtmosConfiguration, dir string, info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(atmosConfig, "terraform.Operations.runTerraformInit")()

	if info.SkipInit {
		return nil
	}

	// Clean terraform workspace to prevent workspace selection prompt
	CleanTerraformWorkspace(dir)

	// Create a copy of the info struct with init subcommand
	initInfo := *info
	initInfo.SubCommand = "init"

	// No manual -reconfigure flag here: initInfo.SubCommand == "init" routes this call through
	// ExecuteTerraform's normal explicit-init path (buildInitSubcommandArgs), which now decides
	// -reconfigure/-upgrade itself via the smart-init policy (autoinit.Decide, called with
	// Force: true for an explicit init) instead of this helper duplicating that decision.
	initInfo.AdditionalArgsAndFlags = []string{}

	// Run terraform init using ExecuteTerraform.
	err := s.host.ExecuteTerraform(initInfo)
	if err != nil {
		return fmt.Errorf("error running terraform init: %w", err)
	}

	return nil
}
