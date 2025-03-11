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

			err = u.WriteToFileAsJSON(backendFileName, componentBackendConfig, 0o600)
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
			err := u.WriteToFileAsJSON(providerOverrideFileName, providerOverrides, 0o600)
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

	// Compare keys in map a to map b
	hasDifferences = compareMapAtoB(a, b, path) || hasDifferences

	// Compare keys in map b to map a (for keys only in b)
	hasDifferences = compareMapBtoA(a, b, path) || hasDifferences

	return hasDifferences
}

// Helper function to compare keys from map a to map b
func compareMapAtoB(a, b map[string]interface{}, path string) bool {
	hasDifferences := false

	for k, v1 := range a {
		currentPath := buildPath(path, k)
		v2, exists := b[k]

		if !exists {
			// Key exists in a but not in b
			printRemovedValue(currentPath, v1)
			hasDifferences = true
			continue
		}

		// Types are different
		if reflect.TypeOf(v1) != reflect.TypeOf(v2) {
			printTypeDifference(currentPath, v1, v2)
			hasDifferences = true
			continue
		}

		// Handle based on value type
		switch val := v1.(type) {
		case map[string]interface{}:
			if prettyDiff(val, v2.(map[string]interface{}), currentPath) {
				hasDifferences = true
			}
		case []interface{}:
			if !reflect.DeepEqual(val, v2) {
				if diffArrays(currentPath, val, v2.([]interface{})) {
					hasDifferences = true
				}
			}
		default:
			if !reflect.DeepEqual(v1, v2) {
				fmt.Printf("~ %s: %v => %v\n", currentPath, v1, v2)
				hasDifferences = true
			}
		}
	}

	return hasDifferences
}

// Helper function to compare keys from map b to map a
func compareMapBtoA(a, b map[string]interface{}, path string) bool {
	hasDifferences := false

	for k, v2 := range b {
		currentPath := buildPath(path, k)
		_, exists := a[k]

		if !exists {
			// Key exists in b but not in a
			printAddedValue(currentPath, v2)
			hasDifferences = true
		}
	}

	return hasDifferences
}

// Helper function to build the path string
func buildPath(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

// Helper function to print a value that was removed
func printRemovedValue(path string, value interface{}) {
	switch v := value.(type) {
	case map[string]interface{}, []interface{}:
		jsonBytes, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			// If marshaling fails, fall back to simple format
			fmt.Printf("- %s: %v\n", path, v)
		} else {
			fmt.Printf("- %s:\n%s\n", path, string(jsonBytes))
		}
	default:
		fmt.Printf("- %s: %v\n", path, v)
	}
}

// Helper function to print a value that was added
func printAddedValue(path string, value interface{}) {
	switch v := value.(type) {
	case map[string]interface{}, []interface{}:
		jsonBytes, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			// If marshaling fails, fall back to simple format
			fmt.Printf("+ %s: %v\n", path, v)
		} else {
			fmt.Printf("+ %s:\n%s\n", path, string(jsonBytes))
		}
	default:
		fmt.Printf("+ %s: %v\n", path, v)
	}
}

// Helper function to print a type difference
func printTypeDifference(path string, v1, v2 interface{}) {
	fmt.Printf("~ %s:\n", path)
	fmt.Printf("  - %v\n", v1)
	fmt.Printf("  + %v\n", v2)
}

// Helper function to diff arrays
func diffArrays(path string, arr1, arr2 []interface{}) bool {
	// For terraform plans, resources arrays are especially important to show clearly
	if path == "resources" || strings.HasSuffix(path, ".resources") {
		return diffResourceArrays(path, arr1, arr2)
	} else {
		return diffGenericArrays(path, arr1, arr2)
	}
}

// Helper function to diff resource arrays
func diffResourceArrays(path string, arr1, arr2 []interface{}) bool {
	fmt.Printf("~ %s: (resource changes)\n", path)
	fmt.Println("  Resources:")

	// Process only if there's content in both arrays
	if len(arr1) > 0 && len(arr2) > 0 {
		// Find resources that changed or were removed
		processRemovedOrChangedResources(arr1, arr2)

		// Find added resources
		processAddedResources(arr1, arr2)
	} else {
		// Simple fallback for empty arrays
		if len(arr1) == 0 {
			fmt.Println("  - No resources in original plan")
		}
		if len(arr2) == 0 {
			fmt.Println("  + No resources in new plan")
		}
	}

	return true // Always return true since we printed something
}

// Helper function to process resources that were removed or changed
func processRemovedOrChangedResources(arr1, arr2 []interface{}) {
	for _, origRes := range arr1 {
		origMap, ok1 := origRes.(map[string]interface{})
		if !ok1 {
			continue
		}

		matchingResource := findMatchingResource(origMap, arr2)

		if matchingResource != nil {
			// Resource exists in both - compare them
			fmt.Printf("  Resource: %s\n", getResourceName(origMap))
			resourceDiff(origMap, matchingResource, "  ")
		} else {
			// Resource was removed
			printRemovedResource(origMap)
		}
	}
}

// Helper function to find a matching resource in the array
func findMatchingResource(resource map[string]interface{}, resources []interface{}) map[string]interface{} {
	if address, hasAddr := resource["address"]; hasAddr {
		for _, res := range resources {
			resMap, ok := res.(map[string]interface{})
			if !ok {
				continue
			}

			if newAddr, hasNewAddr := resMap["address"]; hasNewAddr && address == newAddr {
				return resMap
			}
		}
	}

	return nil
}

// Helper function to process resources that were added
func processAddedResources(arr1, arr2 []interface{}) {
	for _, newRes := range arr2 {
		newMap, ok := newRes.(map[string]interface{})
		if !ok {
			continue
		}

		// Check if this resource exists in the original array
		if findMatchingResource(newMap, arr1) == nil {
			// This is a new resource
			printAddedResource(newMap)
		}
	}
}

// Helper function to print a removed resource
func printRemovedResource(resource map[string]interface{}) {
	fmt.Printf("  - Resource removed: %v\n", getResourceName(resource))
	resourceBytes, err := json.MarshalIndent(resource, "    ", "  ")
	if err != nil {
		// If marshaling fails, just print the map
		fmt.Printf("    %v\n", resource)
	} else {
		fmt.Printf("    %s\n", strings.ReplaceAll(string(resourceBytes), "\n", "\n    "))
	}
}

// Helper function to print an added resource
func printAddedResource(resource map[string]interface{}) {
	fmt.Printf("  + Resource added: %v\n", getResourceName(resource))
	resourceBytes, err := json.MarshalIndent(resource, "    ", "  ")
	if err != nil {
		// If marshaling fails, just print the map
		fmt.Printf("    %v\n", resource)
	} else {
		fmt.Printf("    %s\n", strings.ReplaceAll(string(resourceBytes), "\n", "\n    "))
	}
}

// Helper function to diff generic (non-resource) arrays
func diffGenericArrays(path string, arr1, arr2 []interface{}) bool {
	fmt.Printf("~ %s:\n", path)

	// Print the first array
	if len(arr1) > 0 {
		jsonBytes, err := json.MarshalIndent(arr1, "  - ", "  ")
		if err != nil {
			fmt.Printf("  - [Array marshaling error: %v]\n", err)
		} else {
			fmt.Printf("  - %s\n", string(jsonBytes))
		}
	} else {
		fmt.Println("  - []")
	}

	// Print the second array
	if len(arr2) > 0 {
		jsonBytes, err := json.MarshalIndent(arr2, "  + ", "  ")
		if err != nil {
			fmt.Printf("  + [Array marshaling error: %v]\n", err)
		} else {
			fmt.Printf("  + %s\n", string(jsonBytes))
		}
	} else {
		fmt.Println("  + []")
	}

	return true
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
			diffResourceValues(values1, values2, indent)
			return
		}
	}

	// Fallback if no values field
	diffResourceFallback(a, b, indent)
}

// Helper function to diff resource values
func diffResourceValues(values1, values2 map[string]interface{}, indent string) {
	// Compare values in first map
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

	// Check for added values
	for k, v2 := range values2 {
		_, exists := values1[k]
		if !exists {
			fmt.Printf("%s+ %s: %v\n", indent, k, v2)
		}
	}
}

// Helper function for resource diff fallback method
func diffResourceFallback(a, b map[string]interface{}, indent string) {
	// Skip these common metadata fields
	skipFields := map[string]bool{
		"address":       true,
		"type":          true,
		"name":          true,
		"mode":          true,
		"provider_name": true,
	}

	// Compare fields in first resource
	for k, v1 := range a {
		if skipFields[k] {
			continue
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

	// Check for added fields
	for k, v2 := range b {
		if skipFields[k] {
			continue
		}

		_, exists := a[k]
		if !exists {
			fmt.Printf("%s+ %s: %v\n", indent, k, v2)
		}
	}
}

// executeTerraformPlanDiff generates a diff between two Terraform plan files
func executeTerraformPlanDiff(atmosConfig schema.AtmosConfiguration, info schema.ConfigAndStacksInfo, componentPath, varFile, planFile string) error {
	// Parse command line arguments to get plan file paths
	origPlanPath, newPlanPath, additionalPlanArgs, err := parsePlanDiffArgs(info, componentPath, planFile)
	if err != nil {
		return err
	}

	// Generate a new plan if the new plan file wasn't provided
	if newPlanPath == "" {
		newPlanPath, err = generateNewPlan(atmosConfig, info, componentPath, varFile, planFile, additionalPlanArgs)
		if err != nil {
			return err
		}
	}

	// Convert both plans to comparable JSON format
	origPlanData, newPlanData, err := convertPlansToJSON(atmosConfig, info, componentPath, origPlanPath, newPlanPath)
	if err != nil {
		return err
	}

	// Compare the two plans
	return comparePlans(origPlanData, newPlanData)
}

// parsePlanDiffArgs extracts the original and new plan file paths from command line arguments
func parsePlanDiffArgs(info schema.ConfigAndStacksInfo, componentPath, planFile string) (string, string, []string, error) {
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
		return "", "", nil, errors.New("--orig flag must be provided with the path to the original plan file")
	}

	// Get the absolute path of the original plan file
	origPlanPath := origPlanFlag
	if !filepath.IsAbs(origPlanPath) {
		origPlanPath = filepath.Join(componentPath, origPlanPath)
	}

	// Check if orig plan file exists
	if _, err := os.Stat(origPlanPath); os.IsNotExist(err) {
		return "", "", nil, fmt.Errorf("original plan file does not exist at path: %s", origPlanPath)
	}

	// Process the new plan path if provided
	newPlanPath := ""
	if newPlanFlag != "" {
		newPlanPath = newPlanFlag
		if !filepath.IsAbs(newPlanPath) {
			newPlanPath = filepath.Join(componentPath, newPlanPath)
		}

		// Check if new plan file exists
		if _, err := os.Stat(newPlanPath); os.IsNotExist(err) {
			return "", "", nil, fmt.Errorf("new plan file does not exist at path: %s", newPlanPath)
		}
	}

	return origPlanPath, newPlanPath, additionalPlanArgs, nil
}

// generateNewPlan creates a new Terraform plan file
func generateNewPlan(atmosConfig schema.AtmosConfiguration, info schema.ConfigAndStacksInfo, componentPath, varFile, planFile string, additionalPlanArgs []string) (string, error) {
	// Generate a new plan
	log.Info("Generating new plan...")

	// Create a new plan file path
	newPlanPath := filepath.Join(componentPath, "new-"+filepath.Base(planFile))

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
		return "", err
	}

	return newPlanPath, nil
}

// convertPlansToJSON converts both plan files to JSON format for comparison
func convertPlansToJSON(atmosConfig schema.AtmosConfiguration, info schema.ConfigAndStacksInfo, componentPath, origPlanPath, newPlanPath string) ([]byte, []byte, error) {
	// Create temporary files for the human-readable versions of the plans
	origPlanHumanReadable, err := os.CreateTemp("", "orig-plan-*.json")
	if err != nil {
		return nil, nil, fmt.Errorf("error creating temporary file for original plan: %w", err)
	}
	defer os.Remove(origPlanHumanReadable.Name())
	origPlanHumanReadable.Close()

	newPlanHumanReadable, err := os.CreateTemp("", "new-plan-*.json")
	if err != nil {
		return nil, nil, fmt.Errorf("error creating temporary file for new plan: %w", err)
	}
	defer os.Remove(newPlanHumanReadable.Name())
	newPlanHumanReadable.Close()

	// Run terraform show to get human-readable JSON versions of the plans
	log.Info("Converting plan files to JSON...")

	// Get JSON for the original plan
	origPlanData, err := getPlanAsJSON(componentPath, origPlanPath, origPlanHumanReadable.Name(), info.ComponentEnvList)
	if err != nil {
		return nil, nil, fmt.Errorf("error processing original plan: %w", err)
	}

	// Get JSON for the new plan
	newPlanData, err := getPlanAsJSON(componentPath, newPlanPath, newPlanHumanReadable.Name(), info.ComponentEnvList)
	if err != nil {
		return nil, nil, fmt.Errorf("error processing new plan: %w", err)
	}

	return origPlanData, newPlanData, nil
}

// getPlanAsJSON converts a Terraform plan file to JSON format
func getPlanAsJSON(componentPath, planPath, outputPath string, envVars []string) ([]byte, error) {
	// Create command for showing plan in JSON format
	showCmd := exec.Command("terraform", "show", "-json", planPath)
	showCmd.Dir = componentPath
	showCmd.Env = append(os.Environ(), envVars...)

	// Capture output
	planOut, err := showCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("error showing plan: %w", err)
	}

	// Write the output to the temporary file
	err = os.WriteFile(outputPath, planOut, 0o600)
	if err != nil {
		return nil, fmt.Errorf("error writing plan JSON to file: %w", err)
	}

	// Read the JSON file
	planData, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("error reading plan JSON: %w", err)
	}

	return planData, nil
}

// comparePlans parses and compares two JSON plans, then displays the differences
func comparePlans(origPlanData, newPlanData []byte) error {
	// Parse JSON
	var origPlan, newPlan map[string]interface{}
	if err := json.Unmarshal(origPlanData, &origPlan); err != nil {
		return fmt.Errorf("error parsing original plan JSON: %w", err)
	}
	if err := json.Unmarshal(newPlanData, &newPlan); err != nil {
		return fmt.Errorf("error parsing new plan JSON: %w", err)
	}

	// Remove or normalize timestamp to avoid showing it in the diff
	normalizeTimestamps(origPlan, newPlan)

	// Generate a hierarchical diff between the two plans
	fmt.Println("Plan differences:")
	fmt.Println("----------------")

	hasDifferences := prettyDiff(origPlan, newPlan, "")

	if !hasDifferences {
		fmt.Println("No differences found between the plans.")
	}

	return nil
}

// normalizeTimestamps removes or normalizes timestamp fields to avoid showing them in the diff
func normalizeTimestamps(origPlan, newPlan map[string]interface{}) {
	if _, ok := origPlan["timestamp"]; ok {
		origPlan["timestamp"] = "TIMESTAMP_IGNORED"
	}
	if _, ok := newPlan["timestamp"]; ok {
		newPlan["timestamp"] = "TIMESTAMP_IGNORED"
	}
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
