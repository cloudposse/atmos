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

	log "github.com/charmbracelet/log"

	"github.com/cloudposse/atmos/pkg/schema"
)

// Constants for field names
const (
	timestampField = "timestamp"
)

// PlanJSONData contains the JSON representations of original and new Terraform plans
type PlanJSONData struct {
	OriginalPlan []byte
	NewPlan      []byte
}

// prettyDiff function to recursively compare maps.
func prettyDiff(a, b map[string]interface{}, path string) bool {
	hasDifferences := false

	// Compare keys in map a to map b
	hasDifferences = compareMapAtoB(a, b, path) || hasDifferences

	// Compare keys in map b to map a (for keys only in b)
	hasDifferences = compareMapBtoA(a, b, path) || hasDifferences

	return hasDifferences
}

// Helper function to compare keys from map a to map b.
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
				fmt.Fprintf(os.Stdout, "~ %s: %v => %v\n", currentPath, v1, v2)
				hasDifferences = true
			}
		}
	}

	return hasDifferences
}

// Helper function to compare keys from map b to map a.
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

// Helper function to build the path string.
func buildPath(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

// Helper function to print a value that was removed.
func printRemovedValue(path string, value interface{}) {
	switch v := value.(type) {
	case map[string]interface{}, []interface{}:
		jsonBytes, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			// If marshaling fails, fall back to simple format
			fmt.Fprintf(os.Stdout, "- %s: %v\n", path, v)
		} else {
			fmt.Fprintf(os.Stdout, "- %s:\n%s\n", path, string(jsonBytes))
		}
	default:
		fmt.Fprintf(os.Stdout, "- %s: %v\n", path, v)
	}
}

// Helper function to print a value that was added.
func printAddedValue(path string, value interface{}) {
	switch v := value.(type) {
	case map[string]interface{}, []interface{}:
		jsonBytes, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			// If marshaling fails, fall back to simple format
			fmt.Fprintf(os.Stdout, "+ %s: %v\n", path, v)
		} else {
			fmt.Fprintf(os.Stdout, "+ %s:\n%s\n", path, string(jsonBytes))
		}
	default:
		fmt.Fprintf(os.Stdout, "+ %s: %v\n", path, v)
	}
}

// Helper function to print a type difference.
func printTypeDifference(path string, v1, v2 interface{}) {
	fmt.Fprintf(os.Stdout, "~ %s:\n", path)
	fmt.Fprintf(os.Stdout, "  - %v\n", v1)
	fmt.Fprintf(os.Stdout, "  + %v\n", v2)
}

// Helper function to diff arrays.
func diffArrays(path string, arr1, arr2 []interface{}) bool {
	// For terraform plans, resources arrays are especially important to show clearly
	if path == "resources" || strings.HasSuffix(path, ".resources") {
		return diffResourceArrays(path, arr1, arr2)
	} else {
		return diffGenericArrays(path, arr1, arr2)
	}
}

// Helper function to diff resource arrays.
func diffResourceArrays(path string, arr1, arr2 []interface{}) bool {
	fmt.Fprintf(os.Stdout, "~ %s: (resource changes)\n", path)
	fmt.Fprintln(os.Stdout, "  Resources:")

	// Process only if there's content in both arrays
	if len(arr1) > 0 && len(arr2) > 0 {
		// Find resources that changed or were removed.
		processRemovedOrChangedResources(arr1, arr2)

		// Find added resources
		processAddedResources(arr1, arr2)
	} else {
		// Simple fallback for empty arrays
		if len(arr1) == 0 {
			fmt.Fprintln(os.Stdout, "  - No resources in original plan")
		}
		if len(arr2) == 0 {
			fmt.Fprintln(os.Stdout, "  + No resources in new plan")
		}
	}

	return true // Always return true since we printed something
}

// Helper function to process resources that were removed or changed.
func processRemovedOrChangedResources(arr1, arr2 []interface{}) {
	for _, origRes := range arr1 {
		origMap, ok1 := origRes.(map[string]interface{})
		if !ok1 {
			continue
		}

		matchingResource := findMatchingResource(origMap, arr2)

		if matchingResource == nil {
			// Resource was removed
			printRemovedResource(origMap)
		} else {
			// Resource exists in both - compare them
			a := []any{getResourceName(origMap)}
			fmt.Fprintf(os.Stdout, "  Resource: %s\n", a...)
			resourceDiff(origMap, matchingResource, "  ")
		}
	}
}

// Helper function to find a matching resource in the array.
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

// Helper function to process resources that were added.
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

// Helper function to print a removed resource.
func printRemovedResource(resource map[string]interface{}) {
	a := []any{getResourceName(resource)}
	fmt.Fprintf(os.Stdout, "  - Resource removed: %v\n", a...)
	resourceBytes, err := json.MarshalIndent(resource, "    ", "  ")
	if err != nil {
		// If marshaling fails, just print the map
		fmt.Fprintf(os.Stdout, "    %v\n", resource)
	} else {
		a := []any{strings.ReplaceAll(string(resourceBytes), "\n", "\n    ")}
		fmt.Fprintf(os.Stdout, "    %s\n", a...)
	}
}

// Helper function to print an added resource.
func printAddedResource(resource map[string]interface{}) {
	a := []any{getResourceName(resource)}
	fmt.Fprintf(os.Stdout, "  + Resource added: %v\n", a...)
	resourceBytes, err := json.MarshalIndent(resource, "    ", "  ")
	if err != nil {
		// If marshaling fails, just print the map
		fmt.Fprintf(os.Stdout, "    %v\n", resource)
	} else {
		a := []any{strings.ReplaceAll(string(resourceBytes), "\n", "\n    ")}
		fmt.Fprintf(os.Stdout, "    %s\n", a...)
	}
}

// Helper function to diff generic (non-resource) arrays.
func diffGenericArrays(path string, arr1, arr2 []interface{}) bool {
	fmt.Fprintf(os.Stdout, "~ %s:\n", path)

	// Print the first array
	if len(arr1) > 0 {
		jsonBytes, err := json.MarshalIndent(arr1, "", "  ")
		if err != nil {
			a := []any{err}
			fmt.Fprintf(os.Stdout, "  - [Array marshaling error: %v]\n", a...)
		} else {
			fmt.Fprintf(os.Stdout, "  - %s\n", string(jsonBytes))
		}
	} else {
		fmt.Fprintln(os.Stdout, "  - []")
	}

	// Print the second array
	if len(arr2) > 0 {
		jsonBytes, err := json.MarshalIndent(arr2, "", "  ")
		if err != nil {
			a := []any{err}
			fmt.Fprintf(os.Stdout, "  + [Array marshaling error: %v]\n", a...)
		} else {
			fmt.Fprintf(os.Stdout, "  + %s\n", string(jsonBytes))
		}
	} else {
		fmt.Fprintln(os.Stdout, "  + []")
	}

	return true
}

// Helper function to get a readable resource name.
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

// Helper function to diff individual resources.
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

// Helper function to diff resource values.
func diffResourceValues(values1, values2 map[string]interface{}, indent string) {
	// Compare values in first map
	for k, v1 := range values1 {
		v2, exists := values2[k]

		if !exists {
			a := []any{indent, k, v1}
			fmt.Fprintf(os.Stdout, "%s- %s: %v\n", a...)
			continue
		}

		if !reflect.DeepEqual(v1, v2) {
			a := []any{indent, k, v1, v2}
			fmt.Fprintf(os.Stdout, "%s~ %s: %v => %v\n", a...)
		}
	}

	// Check for added values
	for k, v2 := range values2 {
		_, exists := values1[k]
		if !exists {
			a := []any{indent, k, v2}
			fmt.Fprintf(os.Stdout, "%s+ %s: %v\n", a...)
		}
	}
}

// Helper function for resource diff fallback method.
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
			a := []any{indent, k, v1}
			fmt.Fprintf(os.Stdout, "%s- %s: %v\n", a...)
			continue
		}

		if !reflect.DeepEqual(v1, v2) {
			a := []any{indent, k, v1, v2}
			fmt.Fprintf(os.Stdout, "%s~ %s: %v => %v\n", a...)
		}
	}

	// Check for added fields
	for k, v2 := range b {
		if skipFields[k] {
			continue
		}

		_, exists := a[k]
		if !exists {
			a := []any{indent, k, v2}
			fmt.Fprintf(os.Stdout, "%s+ %s: %v\n", a...)
		}
	}
}

// executeTerraformPlanDiff generates a diff between two Terraform plan files.
func executeTerraformPlanDiff(atmosConfig schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, componentPath, varFile, planFile string) error {
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
	planData, err := convertPlansToJSON(info, componentPath, origPlanPath, newPlanPath)
	if err != nil {
		return err
	}

	// Compare the two plans
	return comparePlans(planData.OriginalPlan, planData.NewPlan)
}

// parsePlanDiffArgs extracts the original and new plan file paths from command line arguments.
func parsePlanDiffArgs(info *schema.ConfigAndStacksInfo, componentPath, planFile string) (string, string, []string, error) {
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

// generateNewPlan creates a new Terraform plan file.
func generateNewPlan(atmosConfig schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, componentPath, varFile, planFile string, additionalPlanArgs []string) (string, error) {
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

// convertPlansToJSON converts both plan files to JSON format for comparison.
func convertPlansToJSON(info *schema.ConfigAndStacksInfo, componentPath, origPlanPath, newPlanPath string) (PlanJSONData, error) {
	// Create temporary files for the human-readable versions of the plans
	origPlanHumanReadable, err := os.CreateTemp("", "orig-plan-*.json")
	if err != nil {
		return PlanJSONData{}, fmt.Errorf("error creating temporary file for original plan: %w", err)
	}
	defer os.Remove(origPlanHumanReadable.Name())
	origPlanHumanReadable.Close()

	newPlanHumanReadable, err := os.CreateTemp("", "new-plan-*.json")
	if err != nil {
		return PlanJSONData{}, fmt.Errorf("error creating temporary file for new plan: %w", err)
	}
	defer os.Remove(newPlanHumanReadable.Name())
	newPlanHumanReadable.Close()

	// Run terraform show to get human-readable JSON versions of the plans
	log.Info("Converting plan files to JSON...")

	// Get JSON for the original plan
	origPlanData, err := getPlanAsJSON(componentPath, origPlanPath, origPlanHumanReadable.Name(), info.ComponentEnvList)
	if err != nil {
		return PlanJSONData{}, fmt.Errorf("error processing original plan: %w", err)
	}

	// Get JSON for the new plan
	newPlanData, err := getPlanAsJSON(componentPath, newPlanPath, newPlanHumanReadable.Name(), info.ComponentEnvList)
	if err != nil {
		return PlanJSONData{}, fmt.Errorf("error processing new plan: %w", err)
	}

	return PlanJSONData{
		OriginalPlan: origPlanData,
		NewPlan:      newPlanData,
	}, nil
}

// getPlanAsJSON converts a Terraform plan file to JSON format.
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

// comparePlans parses and compares two JSON plans, then displays the differences.
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
	fmt.Fprintln(os.Stdout, "Plan differences:")
	fmt.Fprintln(os.Stdout, "----------------")

	hasDifferences := prettyDiff(origPlan, newPlan, "")

	if !hasDifferences {
		fmt.Fprintln(os.Stdout, "No differences found between the plans.")
	}

	return nil
}

// normalizeTimestamps removes or normalizes timestamp fields to avoid showing them in the diff.
func normalizeTimestamps(origPlan, newPlan map[string]interface{}) {
	if _, ok := origPlan[timestampField]; ok {
		origPlan[timestampField] = "TIMESTAMP_IGNORED"
	}
	if _, ok := newPlan[timestampField]; ok {
		newPlan[timestampField] = "TIMESTAMP_IGNORED"
	}
}

// sortJSON recursively sorts all maps in a JSON object to ensure consistent ordering.
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
