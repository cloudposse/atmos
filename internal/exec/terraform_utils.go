package exec

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/charmbracelet/log"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
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

// prettyDiff function to recursively compare maps
func prettyDiff(a, b map[string]interface{}, path string) bool {
	hasDifferences := false

	for k, v1 := range a {
		currentPath := path
		if path != "" {
			currentPath += "."
		}
		currentPath += k

		v2, exists := b[k]

		if !exists {
			// Format complex objects nicely
			switch v1.(type) {
			case map[string]interface{}, []interface{}:
				jsonBytes, err := json.MarshalIndent(v1, "", "  ")
				if err != nil {
					// If marshaling fails, fall back to simple format
					fmt.Printf("- %s: %v\n", currentPath, v1)
				} else {
					fmt.Printf("- %s:\n%s\n", currentPath, string(jsonBytes))
				}
			default:
				fmt.Printf("- %s: %v\n", currentPath, v1)
			}
			hasDifferences = true
			continue
		}

		if reflect.TypeOf(v1) != reflect.TypeOf(v2) {
			// Format complex objects nicely
			fmt.Printf("~ %s:\n", currentPath)
			fmt.Printf("  - %v\n", v1)
			fmt.Printf("  + %v\n", v2)
			hasDifferences = true
			continue
		}

		switch val := v1.(type) {
		case map[string]interface{}:
			if prettyDiff(val, v2.(map[string]interface{}), currentPath) {
				hasDifferences = true
			}
		case []interface{}:
			// Handle arrays specially
			if !reflect.DeepEqual(val, v2) {
				// For terraform plans, resources arrays are especially important to show clearly
				if k == "resources" || strings.HasSuffix(currentPath, ".resources") {
					fmt.Printf("~ %s: (resource changes)\n", currentPath)

					// Create a simple visual diff
					fmt.Println("  Resources:")
					// Find common prefix for resources to show targeted diff
					if len(val) > 0 && len(v2.([]interface{})) > 0 {
						// Show a focused diff of just the resource changes
						for _, origRes := range val {
							origMap, ok1 := origRes.(map[string]interface{})
							if !ok1 {
								continue
							}

							found := false
							// Try to find matching resource in new plan
							for _, newRes := range v2.([]interface{}) {
								newMap, ok2 := newRes.(map[string]interface{})
								if !ok2 {
									continue
								}

								// Match resources by address if possible
								if address, hasAddr := origMap["address"]; hasAddr {
									if newAddr, hasNewAddr := newMap["address"]; hasNewAddr && address == newAddr {
										found = true
										// Compare the two resources
										fmt.Printf("  Resource: %s\n", address)
										resourceDiff(origMap, newMap, "  ")
										break
									}
								}
							}

							if !found {
								fmt.Printf("  - Resource removed: %v\n", getResourceName(origMap))
								resourceBytes, err := json.MarshalIndent(origMap, "    ", "  ")
								if err != nil {
									// If marshaling fails, just print the map
									fmt.Printf("    %v\n", origMap)
								} else {
									fmt.Printf("    %s\n", strings.ReplaceAll(string(resourceBytes), "\n", "\n    "))
								}
							}
						}

						// Look for added resources
						for _, newRes := range v2.([]interface{}) {
							newMap, ok := newRes.(map[string]interface{})
							if !ok {
								continue
							}

							found := false
							for _, origRes := range val {
								origMap, ok := origRes.(map[string]interface{})
								if !ok {
									continue
								}

								// Match resources by address if possible
								if address, hasAddr := newMap["address"]; hasAddr {
									if origAddr, hasOrigAddr := origMap["address"]; hasOrigAddr && address == origAddr {
										found = true
										break
									}
								}
							}

							if !found {
								fmt.Printf("  + Resource added: %v\n", getResourceName(newMap))
								resourceBytes, err := json.MarshalIndent(newMap, "    ", "  ")
								if err != nil {
									// If marshaling fails, just print the map
									fmt.Printf("    %v\n", newMap)
								} else {
									fmt.Printf("    %s\n", strings.ReplaceAll(string(resourceBytes), "\n", "\n    "))
								}
							}
						}
					} else {
						// Simple fallback for empty arrays or when resources can't be matched
						if len(val) == 0 {
							fmt.Println("  - No resources in original plan")
						}
						if len(v2.([]interface{})) == 0 {
							fmt.Println("  + No resources in new plan")
						}
					}
				} else {
					// For other arrays, show a simpler diff
					fmt.Printf("~ %s:\n", currentPath)
					if len(val) > 0 {
						jsonBytes, err := json.MarshalIndent(val, "  - ", "  ")
						if err != nil {
							fmt.Printf("  - [Array marshaling error: %v]\n", err)
						} else {
							fmt.Printf("  - %s\n", string(jsonBytes))
						}
					} else {
						fmt.Println("  - []")
					}

					newArray := v2.([]interface{})
					if len(newArray) > 0 {
						jsonBytes, err := json.MarshalIndent(newArray, "  + ", "  ")
						if err != nil {
							fmt.Printf("  + [Array marshaling error: %v]\n", err)
						} else {
							fmt.Printf("  + %s\n", string(jsonBytes))
						}
					} else {
						fmt.Println("  + []")
					}
				}
				hasDifferences = true
			}
		default:
			if !reflect.DeepEqual(v1, v2) {
				fmt.Printf("~ %s: %v => %v\n", currentPath, v1, v2)
				hasDifferences = true
			}
		}
	}

	for k, v2 := range b {
		currentPath := path
		if path != "" {
			currentPath += "."
		}
		currentPath += k

		_, exists := a[k]
		if !exists {
			// Format complex objects nicely
			switch v2.(type) {
			case map[string]interface{}, []interface{}:
				jsonBytes, err := json.MarshalIndent(v2, "", "  ")
				if err != nil {
					// If marshaling fails, fall back to simple format
					fmt.Printf("+ %s: %v\n", currentPath, v2)
				} else {
					fmt.Printf("+ %s:\n%s\n", currentPath, string(jsonBytes))
				}
			default:
				fmt.Printf("+ %s: %v\n", currentPath, v2)
			}
			hasDifferences = true
		}
	}

	return hasDifferences
}

// Helper function to get a readable resource name
func getResourceName(resource map[string]interface{}) string {
	if address, hasAddr := resource["address"]; hasAddr {
		return fmt.Sprintf("%v", address)
	}

	var parts []string

	if t, hasType := resource["type"]; hasType {
		parts = append(parts, fmt.Sprintf("%v", t))
	}

	if name, hasName := resource["name"]; hasName {
		parts = append(parts, fmt.Sprintf("%v", name))
	}

	if len(parts) > 0 {
		return strings.Join(parts, ".")
	}

	return "<unknown resource>"
}

// Helper function to diff individual resources
func resourceDiff(a, b map[string]interface{}, indent string) {
	// Focus on the values part of the resource if present
	if values1, hasValues1 := a["values"].(map[string]interface{}); hasValues1 {
		if values2, hasValues2 := b["values"].(map[string]interface{}); hasValues2 {
			// Compare values
			for k, v1 := range values1 {
				v2, exists := values2[k]

				if !exists {
					fmt.Printf("%s- %s: %v\n", indent, k, v1)
					continue
				}

				if !reflect.DeepEqual(v1, v2) {
					fmt.Printf("%s~ %s: %v => %v\n", indent, k, v1, v2)
				}
			}

			for k, v2 := range values2 {
				_, exists := values1[k]
				if !exists {
					fmt.Printf("%s+ %s: %v\n", indent, k, v2)
				}
			}
			return
		}
	}

	// Fallback if no values field
	for k, v1 := range a {
		if k == "address" || k == "type" || k == "name" || k == "mode" || k == "provider_name" {
			continue // Skip common metadata fields
		}

		v2, exists := b[k]

		if !exists {
			fmt.Printf("%s- %s: %v\n", indent, k, v1)
			continue
		}

		if !reflect.DeepEqual(v1, v2) {
			fmt.Printf("%s~ %s: %v => %v\n", indent, k, v1, v2)
		}
	}

	for k, v2 := range b {
		if k == "address" || k == "type" || k == "name" || k == "mode" || k == "provider_name" {
			continue // Skip common metadata fields
		}

		_, exists := a[k]
		if !exists {
			fmt.Printf("%s+ %s: %v\n", indent, k, v2)
		}
	}
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

	// Generate a hierarchical diff between the two plans
	fmt.Println("Plan differences:")
	fmt.Println("----------------")

	hasDifferences := prettyDiff(origPlan, newPlan, "")

	if !hasDifferences {
		fmt.Println("No differences found between the plans.")
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
