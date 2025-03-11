package exec

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"

	"github.com/charmbracelet/log"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/pmezard/go-difflib/difflib"
)

func checkTerraformConfig(atmosConfig schema.AtmosConfiguration) error {
	if len(atmosConfig.Components.Terraform.BasePath) < 1 {
		return errors.New("Base path to terraform components must be provided in 'components.terraform.base_path' config or " +
			"'ATMOS_COMPONENTS_TERRAFORM_BASE_PATH' ENV variable")
	}

	return nil
}

// cleanTerraformWorkspace deletes the `.terraform/environment` file from the component directory.
// The `.terraform/environment` file contains the name of the currently selected workspace,
// helping Terraform identify the active workspace context for managing your infrastructure.
// We delete the file to prevent the Terraform prompt asking to select the default or the
// previously used workspace. This happens when different backends are used for the same component.
func cleanTerraformWorkspace(atmosConfig schema.AtmosConfiguration, componentPath string) {
	// Get `TF_DATA_DIR` ENV variable, default to `.terraform` if not set
	tfDataDir := os.Getenv("TF_DATA_DIR")
	if tfDataDir == "" {
		tfDataDir = ".terraform"
	}

	// Convert relative path to absolute
	if !filepath.IsAbs(tfDataDir) {
		tfDataDir = filepath.Join(componentPath, tfDataDir)
	}

	// Ensure the path is cleaned properly
	tfDataDir = filepath.Clean(tfDataDir)

	// Construct the full file path
	filePath := filepath.Join(tfDataDir, "environment")

	// Check if the file exists before attempting deletion
	if _, err := os.Stat(filePath); err == nil {
		log.Debug("Terraform environment file found. Proceeding with deletion.", "file", filePath)

		if err := os.Remove(filePath); err != nil {
			log.Debug("Failed to delete Terraform environment file.", "file", filePath, "error", err)
		} else {
			log.Debug("Successfully deleted Terraform environment file.", "file", filePath)
		}
	} else if os.IsNotExist(err) {
		log.Debug("Terraform environment file not found. No action needed.", "file", filePath)
	} else {
		log.Debug("Error checking Terraform environment file.", "file", filePath, "error", err)
	}
}

func shouldProcessStacks(info *schema.ConfigAndStacksInfo) (bool, bool) {
	shouldProcessStacks := true
	shouldCheckStack := true

	if info.SubCommand == "clean" {
		if info.ComponentFromArg == "" {
			shouldProcessStacks = false
		}
		shouldCheckStack = info.Stack != ""

	}

	return shouldProcessStacks, shouldCheckStack
}

func generateBackendConfig(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, workingDir string) error {
	// Auto-generate backend file
	if atmosConfig.Components.Terraform.AutoGenerateBackendFile {
		backendFileName := filepath.Join(workingDir, "backend.tf.json")

		log.Debug("Writing the backend config to file.", "file", backendFileName)

		if !info.DryRun {
			componentBackendConfig, err := generateComponentBackendConfig(info.ComponentBackendType, info.ComponentBackendSection, info.TerraformWorkspace)
			if err != nil {
				return err
			}

			err = u.WriteToFileAsJSON(backendFileName, componentBackendConfig, 0o644)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func generateProviderOverrides(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, workingDir string) error {
	// Generate `providers_override.tf.json` file if the `providers` section is configured
	if len(info.ComponentProvidersSection) > 0 {
		providerOverrideFileName := filepath.Join(workingDir, "providers_override.tf.json")

		log.Debug("Writing the provider overrides to file.", "file", providerOverrideFileName)

		if !info.DryRun {
			providerOverrides := generateComponentProviderOverrides(info.ComponentProvidersSection)
			err := u.WriteToFileAsJSON(providerOverrideFileName, providerOverrides, 0o644)
			return err
		}
	}
	return nil
}

// needProcessTemplatesAndYamlFunctions checks if a Terraform command
// requires the `Go` templates and Atmos YAML functions to be processed
func needProcessTemplatesAndYamlFunctions(command string) bool {
	commandsThatNeedFuncProcessing := []string{
		"init",
		"plan",
		"apply",
		"deploy",
		"destroy",
		"generate",
		"output",
		"clean",
		"shell",
		"write",
		"force-unlock",
		"import",
		"refresh",
		"show",
		"taint",
		"untaint",
		"validate",
		"state list",
		"state mv",
		"state pull",
		"state push",
		"state replace-provider",
		"state rm",
		"state show",
	}
	return u.SliceContainsString(commandsThatNeedFuncProcessing, command)
}

// isWorkspacesEnabled checks if Terraform workspaces are enabled for a component.
// Workspaces are enabled by default except for:
// 1. When explicitly disabled via workspaces_enabled: false in `atmos.yaml`.
// 2. When using HTTP backend (which doesn't support workspaces).
func isWorkspacesEnabled(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) bool {
	// Check if using HTTP backend first, as it doesn't support workspaces
	if info.ComponentBackendType == "http" {
		// If workspaces are explicitly enabled for HTTP backend, log a warning.
		if atmosConfig.Components.Terraform.WorkspacesEnabled != nil && *atmosConfig.Components.Terraform.WorkspacesEnabled {
			log.Warn("ignoring the enabled setting for workspaces since HTTP backend doesn't support workspaces",
				"backend", "http",
				"component", info.Component)
		}
		return false
	}

	// Check if workspaces are explicitly disabled.
	if atmosConfig.Components.Terraform.WorkspacesEnabled != nil && !*atmosConfig.Components.Terraform.WorkspacesEnabled {
		return false
	}

	return true
}

// executeTerraformPlanDiff generates a diff between two Terraform plan files
func executeTerraformPlanDiff(atmosConfig schema.AtmosConfiguration, info schema.ConfigAndStacksInfo, componentPath, varFile, planFile string) error {
	origPlanFlag := ""
	newPlanFlag := ""
	var skipNext bool
	var additionalPlanArgs []string

	// Extract the orig and new plan file paths from the flags and collect other arguments
	for i, arg := range info.AdditionalArgsAndFlags {
		if skipNext {
			skipNext = false
			continue
		}

		if arg == "--orig" && i+1 < len(info.AdditionalArgsAndFlags) {
			origPlanFlag = info.AdditionalArgsAndFlags[i+1]
			skipNext = true
		} else if arg == "--new" && i+1 < len(info.AdditionalArgsAndFlags) {
			newPlanFlag = info.AdditionalArgsAndFlags[i+1]
			skipNext = true
		} else {
			// Add any other arguments to be passed to the terraform plan command
			additionalPlanArgs = append(additionalPlanArgs, arg)
		}
	}

	// Check if orig flag is provided
	if origPlanFlag == "" {
		return errors.New("--orig flag must be provided with the path to the original plan file")
	}

	origPlanPath := origPlanFlag
	if !filepath.IsAbs(origPlanPath) {
		origPlanPath = filepath.Join(componentPath, origPlanPath)
	}

	// Check if orig plan file exists
	if _, err := os.Stat(origPlanPath); os.IsNotExist(err) {
		return fmt.Errorf("original plan file does not exist at path: %s", origPlanPath)
	}

	// Generate a new plan if --new flag is not provided
	newPlanPath := ""
	if newPlanFlag == "" {
		// Generate a new plan
		log.Info("Generating new plan...")

		// Create a temporary plan file
		newPlanPath = filepath.Join(componentPath, "new-"+filepath.Base(planFile))

		// Run terraform plan to generate the new plan with all additional arguments
		planCmd := []string{"plan", varFileFlag, varFile, outFlag, newPlanPath}
		planCmd = append(planCmd, additionalPlanArgs...)

		err := ExecuteShellCommand(
			atmosConfig,
			"terraform",
			planCmd,
			componentPath,
			info.ComponentEnvList,
			info.DryRun,
			info.RedirectStdErr,
		)
		if err != nil {
			return err
		}
	} else {
		newPlanPath = newPlanFlag
		if !filepath.IsAbs(newPlanPath) {
			newPlanPath = filepath.Join(componentPath, newPlanPath)
		}

		// Check if new plan file exists
		if _, err := os.Stat(newPlanPath); os.IsNotExist(err) {
			return fmt.Errorf("new plan file does not exist at path: %s", newPlanPath)
		}
	}

	// Create temporary files for the human-readable versions of the plans
	origPlanHumanReadable, err := os.CreateTemp("", "orig-plan-*.json")
	if err != nil {
		return fmt.Errorf("error creating temporary file for original plan: %w", err)
	}
	defer os.Remove(origPlanHumanReadable.Name())
	origPlanHumanReadable.Close()

	newPlanHumanReadable, err := os.CreateTemp("", "new-plan-*.json")
	if err != nil {
		return fmt.Errorf("error creating temporary file for new plan: %w", err)
	}
	defer os.Remove(newPlanHumanReadable.Name())
	newPlanHumanReadable.Close()

	// Run terraform show to get human-readable JSON versions of the plans
	log.Info("Converting plan files to JSON...")

	// Create commands for showing plans in JSON format
	origShowCmd := exec.Command("terraform", "show", "-json", origPlanPath)
	origShowCmd.Dir = componentPath
	origShowCmd.Env = append(os.Environ(), info.ComponentEnvList...)

	newShowCmd := exec.Command("terraform", "show", "-json", newPlanPath)
	newShowCmd.Dir = componentPath
	newShowCmd.Env = append(os.Environ(), info.ComponentEnvList...)

	// Capture stdout and stderr
	origOut, err := origShowCmd.Output()
	if err != nil {
		return fmt.Errorf("error showing original plan: %w", err)
	}

	newOut, err := newShowCmd.Output()
	if err != nil {
		return fmt.Errorf("error showing new plan: %w", err)
	}

	// Write the output to temporary files
	err = os.WriteFile(origPlanHumanReadable.Name(), origOut, 0o644)
	if err != nil {
		return fmt.Errorf("error writing original plan JSON to file: %w", err)
	}

	err = os.WriteFile(newPlanHumanReadable.Name(), newOut, 0o644)
	if err != nil {
		return fmt.Errorf("error writing new plan JSON to file: %w", err)
	}

	// Parse and normalize the JSON files to ensure consistent ordering
	log.Info("Comparing plans...")
	origPlanData, err := os.ReadFile(origPlanHumanReadable.Name())
	if err != nil {
		return fmt.Errorf("error reading original plan JSON: %w", err)
	}

	newPlanData, err := os.ReadFile(newPlanHumanReadable.Name())
	if err != nil {
		return fmt.Errorf("error reading new plan JSON: %w", err)
	}

	// Parse JSON
	var origPlan, newPlan map[string]interface{}
	if err := json.Unmarshal(origPlanData, &origPlan); err != nil {
		return fmt.Errorf("error parsing original plan JSON: %w", err)
	}
	if err := json.Unmarshal(newPlanData, &newPlan); err != nil {
		return fmt.Errorf("error parsing new plan JSON: %w", err)
	}

	// Remove or normalize timestamp to avoid showing it in the diff
	if _, ok := origPlan["timestamp"]; ok {
		origPlan["timestamp"] = "TIMESTAMP_IGNORED"
	}
	if _, ok := newPlan["timestamp"]; ok {
		newPlan["timestamp"] = "TIMESTAMP_IGNORED"
	}

	// Convert maps to sorted JSON strings
	origPlanSorted, err := json.MarshalIndent(sortJSON(origPlan), "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling original plan JSON: %w", err)
	}

	newPlanSorted, err := json.MarshalIndent(sortJSON(newPlan), "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling new plan JSON: %w", err)
	}

	// Generate a diff between the two plans
	diff := difflib.UnifiedDiff{
		A:        difflib.SplitLines(string(origPlanSorted)),
		B:        difflib.SplitLines(string(newPlanSorted)),
		FromFile: "Original Plan",
		ToFile:   "New Plan",
		Context:  3,
		Eol:      "\n",
	}

	diffText, err := difflib.GetUnifiedDiffString(diff)
	if err != nil {
		return fmt.Errorf("error generating diff: %w", err)
	}

	if diffText == "" {
		fmt.Println("No differences found between the plans.")
	} else {
		fmt.Println(diffText)
	}

	return nil
}

// sortJSON recursively sorts all maps in a JSON object to ensure consistent ordering
func sortJSON(data interface{}) interface{} {
	switch v := data.(type) {
	case map[string]interface{}:
		// Get all keys
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		// Sort keys
		sort.Strings(keys)

		// Create new map with sorted keys
		sortedMap := make(map[string]interface{})
		for _, k := range keys {
			sortedMap[k] = sortJSON(v[k])
		}
		return sortedMap
	case []interface{}:
		// Recursively sort each item in the array
		for i := range v {
			v[i] = sortJSON(v[i])
		}
		return v
	default:
		// Return all other types as is
		return v
	}
}
