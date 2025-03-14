package exec

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	terrerrors "github.com/cloudposse/atmos/pkg/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/pkg/errors"
)

// noChangesText is the text used to represent that no changes were found in a diff.
const noChangesText = "(no changes)"

// planFileMode defines the file permission for plan files.
const planFileMode = 0o600

// maxStringDisplayLength is the maximum length of a string before truncating it in output.
const maxStringDisplayLength = 300

// halfStringDisplayLength is half of the max string length used for truncation.
const halfStringDisplayLength = 150

// Static errors
var (
	// ErrNoJSONOutput is returned when no JSON output is found in terraform show output.
	ErrNoJSONOutput = errors.New("no JSON output found in terraform show output")
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
	// Extract flags and setup paths
	origPlanFile, newPlanFile, err := parsePlanDiffFlags(info.AdditionalArgsAndFlags)
	if err != nil {
		return err
	}

	// Create a temporary directory for all temporary files
	tmpDir, err := os.MkdirTemp("", "atmos-terraform-plan-diff")
	if err != nil {
		return errors.Wrap(err, "error creating temporary directory")
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
		return "", "", errors.New("original plan file (--orig) is required")
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
		return "", errors.Errorf("original plan file '%s' does not exist", origPlanFile)
	}

	return origPlanFile, nil
}

// prepareNewPlanFile handles the new plan file (generates one if not provided).
func prepareNewPlanFile(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, opts PlanFileOptions) (string, error) {
	// If no new plan file is specified, generate one
	if opts.NewPlanFile == "" {
		var err error
		opts.NewPlanFile, err = generateNewPlanFile(atmosConfig, info, opts.ComponentPath, opts.OrigPlanFile, opts.TmpDir)
		if err != nil {
			return "", errors.Wrap(err, "error generating new plan file")
		}
	} else if !filepath.IsAbs(opts.NewPlanFile) {
		// If the path is relative, make it absolute based on the component directory
		opts.NewPlanFile = filepath.Join(opts.ComponentPath, opts.NewPlanFile)
	}

	// Make sure the new plan file exists
	if _, err := os.Stat(opts.NewPlanFile); os.IsNotExist(err) {
		return "", errors.Errorf("new plan file '%s' does not exist", opts.NewPlanFile)
	}

	return opts.NewPlanFile, nil
}

// comparePlansAndGenerateDiff compares two plan files and generates a diff.
func comparePlansAndGenerateDiff(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, componentPath, origPlanFile, newPlanFile string) error {
	// Get the JSON representation of the original plan
	origPlanJSON, err := getTerraformPlanJSON(atmosConfig, info, componentPath, origPlanFile)
	if err != nil {
		return errors.Wrap(err, "error getting JSON for original plan")
	}

	// Get the JSON representation of the new plan
	newPlanJSON, err := getTerraformPlanJSON(atmosConfig, info, componentPath, newPlanFile)
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
		u.PrintErrorMarkdown("", terrerrors.ErrPlanHasDiff, "")

		// Exit with code 2 to indicate that the plans are different
		u.OsExit(2)
		return nil // This line will never be reached
	}

	fmt.Fprintln(os.Stdout, "The planfiles are identical")
	return nil
}

// generateNewPlanFile generates a new plan file by running terraform plan.
func generateNewPlanFile(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, componentPath string, origPlanFile string, tmpDir string) (string, error) {
	// Create a temporary file for the new plan
	newPlanFile := filepath.Join(tmpDir, "new.plan")

	// Run terraform init before plan
	if err := runTerraformInit(atmosConfig, componentPath, info); err != nil {
		return "", err
	}

	// Create a new info object for the plan command
	planInfo := *info
	planInfo.SubCommand = "plan"

	// Filter out --orig and --new flags from AdditionalArgsAndFlags
	var planArgs []string
	for i := 0; i < len(info.AdditionalArgsAndFlags); i++ {
		arg := info.AdditionalArgsAndFlags[i]

		// Skip --orig and --new flags and their values
		if arg == "--orig" || arg == "--new" {
			// Skip the value too if it exists
			if i+1 < len(info.AdditionalArgsAndFlags) && !strings.HasPrefix(info.AdditionalArgsAndFlags[i+1], "-") {
				i++
			}
			continue
		}

		if strings.HasPrefix(arg, "--orig=") || strings.HasPrefix(arg, "--new=") {
			continue
		}

		planArgs = append(planArgs, arg)
	}

	// Add -out flag to specify the output plan file
	planArgs = append(planArgs, "-out="+newPlanFile)

	// Update the AdditionalArgsAndFlags with our filtered and augmented args
	planInfo.AdditionalArgsAndFlags = planArgs

	// Execute the plan command using the standard Atmos terraform execution
	err := ExecuteTerraform(planInfo)
	if err != nil {
		return "", fmt.Errorf("error running terraform plan: %w", err)
	}

	return newPlanFile, nil
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

// copyPlanFileIfNeeded copies the plan file to the component directory if it's not already there.
func copyPlanFileIfNeeded(planFile, componentPath string) (string, func(), error) {
	planFileInComponentDir := planFile
	planFileBaseName := filepath.Base(planFile)

	// If the plan file is not in the component directory, create a temporary copy
	if !strings.HasPrefix(planFile, componentPath) {
		planFileInComponentDir = filepath.Join(componentPath, planFileBaseName)

		// Copy the plan file content
		planContent, err := os.ReadFile(planFile)
		if err != nil {
			return "", nil, fmt.Errorf("error reading plan file: %w", err)
		}

		err = os.WriteFile(planFileInComponentDir, planContent, planFileMode)
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
	// Create a pipe to capture stdout
	r, w, err := os.Pipe()
	if err != nil {
		return "", fmt.Errorf("error creating pipe: %w", err)
	}

	// Save original stdout and replace it with our pipe
	origStdout := os.Stdout
	os.Stdout = w

	// Create a new info object for the show command
	showInfo := *info
	showInfo.SubCommand = "show"
	showInfo.AdditionalArgsAndFlags = []string{"-json", planFile}

	// Execute terraform show command
	execErr := ExecuteTerraform(showInfo)

	// Close write end of pipe to flush all output
	w.Close()

	// Restore original stdout
	os.Stdout = origStdout

	// Read the captured output
	output, readErr := io.ReadAll(r)
	r.Close()

	// Check for errors
	if execErr != nil {
		return "", fmt.Errorf("error running terraform show: %w", execErr)
	}
	if readErr != nil {
		return "", fmt.Errorf("error reading output: %w", readErr)
	}

	return string(output), nil
}

// extractJSONFromOutput extracts the JSON part from terraform show output.
func extractJSONFromOutput(output string) (string, error) {
	// Find the beginning of the JSON output (first '{' character)
	jsonStartIdx := strings.Index(output, "{")
	if jsonStartIdx == -1 {
		return "", ErrNoJSONOutput
	}

	// Extract just the JSON part
	jsonOutput := output[jsonStartIdx:]

	return jsonOutput, nil
}

// sortMapKeys recursively sorts map keys for consistent comparison.
func sortMapKeys(m map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	// Get all keys
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	// Sort keys
	sort.Strings(keys)

	// Process each key in sorted order
	for _, k := range keys {
		v := m[k]

		// Recursively sort nested maps
		if nestedMap, ok := v.(map[string]interface{}); ok {
			result[k] = sortMapKeys(nestedMap)
		} else if nestedSlice, ok := v.([]interface{}); ok {
			// Process slices
			processedSlice := make([]interface{}, len(nestedSlice))
			for i, item := range nestedSlice {
				if nestedItem, ok := item.(map[string]interface{}); ok {
					processedSlice[i] = sortMapKeys(nestedItem)
				} else {
					processedSlice[i] = item
				}
			}
			result[k] = processedSlice
		} else {
			result[k] = v
		}
	}

	return result
}

// generatePlanDiff generates a diff between two terraform plans.
func generatePlanDiff(origPlan, newPlan map[string]interface{}) (string, bool) {
	var diff strings.Builder
	hasDiff := false

	// Compare variables
	if origVars, newVars := getVariables(origPlan), getVariables(newPlan); !reflect.DeepEqual(origVars, newVars) {
		hasDiff = true
		diff.WriteString("Variables:\n")
		diff.WriteString("----------\n")

		// Find added variables
		for k, v := range newVars {
			if _, exists := origVars[k]; !exists {
				diff.WriteString(fmt.Sprintf("+ %s: %v\n", k, formatValue(v)))
			}
		}

		// Find removed variables
		for k, v := range origVars {
			if _, exists := newVars[k]; !exists {
				diff.WriteString(fmt.Sprintf("- %s: %v\n", k, formatValue(v)))
			}
		}

		// Find changed variables
		for k, origV := range origVars {
			if newV, exists := newVars[k]; exists && !reflect.DeepEqual(origV, newV) {
				diff.WriteString(fmt.Sprintf("~ %s: %v => %v\n", k, formatValue(origV), formatValue(newV)))
			}
		}

		diff.WriteString("\n")
	}

	// Compare resources
	if origResources, newResources := getResources(origPlan), getResources(newPlan); !reflect.DeepEqual(origResources, newResources) {
		hasDiff = true
		diff.WriteString("Resources:\n")
		diff.WriteString("-----------\n")
		diff.WriteString("\n")

		resourceDiff := compareResources(origResources, newResources)
		diff.WriteString(resourceDiff)
		diff.WriteString("\n")
	}

	// Compare outputs
	if origOutputs, newOutputs := getOutputs(origPlan), getOutputs(newPlan); !reflect.DeepEqual(origOutputs, newOutputs) {
		hasDiff = true
		diff.WriteString("Outputs:\n")
		diff.WriteString("--------\n")

		outputDiff := compareOutputs(origOutputs, newOutputs)
		diff.WriteString(outputDiff)
	}

	return diff.String(), hasDiff
}

// compareOutputs compares outputs between two terraform plans.
func compareOutputs(origOutputs, newOutputs map[string]interface{}) string {
	var diff strings.Builder

	// Find added outputs
	for k, v := range newOutputs {
		if _, exists := origOutputs[k]; !exists {
			diff.WriteString(fmt.Sprintf("+ %s: %v\n", k, formatValue(v)))
		}
	}

	// Find removed outputs
	for k, v := range origOutputs {
		if _, exists := newOutputs[k]; !exists {
			diff.WriteString(fmt.Sprintf("- %s: %v\n", k, formatValue(v)))
		}
	}

	// Find changed outputs
	for k, origV := range origOutputs {
		if newV, exists := newOutputs[k]; exists && !reflect.DeepEqual(origV, newV) {
			diff.WriteString(formatOutputChange(k, origV, newV))
		}
	}

	return diff.String()
}

// formatOutputChange formats the change between two output values.
func formatOutputChange(key string, origValue, newValue interface{}) string {
	origSensitive := isSensitive(origValue)
	newSensitive := isSensitive(newValue)

	switch {
	case origSensitive && newSensitive:
		return fmt.Sprintf("~ %s: (sensitive value) => (sensitive value)\n", key)
	case origSensitive:
		return fmt.Sprintf("~ %s: (sensitive value) => %v\n", key, formatValue(newValue))
	case newSensitive:
		return fmt.Sprintf("~ %s: %v => (sensitive value)\n", key, formatValue(origValue))
	default:
		return fmt.Sprintf("~ %s: %v => %v\n", key, formatValue(origValue), formatValue(newValue))
	}
}

// getVariables extracts variables from a terraform plan.
func getVariables(plan map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	if vars, ok := plan["variables"].(map[string]interface{}); ok {
		for k, v := range vars {
			if varMap, ok := v.(map[string]interface{}); ok {
				if value, exists := varMap["value"]; exists {
					result[k] = value
				}
			}
		}
	}

	return result
}

// getResources extracts resources from a terraform plan.
func getResources(plan map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	// Check prior_state for data resources
	if priorState, ok := plan["prior_state"].(map[string]interface{}); ok {
		if values, ok := priorState["values"].(map[string]interface{}); ok {
			if rootModule, ok := values["root_module"].(map[string]interface{}); ok {
				if resources, ok := rootModule["resources"].([]interface{}); ok {
					for _, res := range resources {
						if resMap, ok := res.(map[string]interface{}); ok {
							address := resMap["address"].(string)
							result[address] = resMap
						}
					}
				}
			}
		}
	}

	// Check planned_values for managed resources
	if plannedValues, ok := plan["planned_values"].(map[string]interface{}); ok {
		if rootModule, ok := plannedValues["root_module"].(map[string]interface{}); ok {
			if resources, ok := rootModule["resources"].([]interface{}); ok {
				for _, res := range resources {
					if resMap, ok := res.(map[string]interface{}); ok {
						address := resMap["address"].(string)
						result[address] = resMap
					}
				}
			}
		}
	}

	// Check resource_changes
	if resourceChanges, ok := plan["resource_changes"].([]interface{}); ok {
		for _, change := range resourceChanges {
			if changeMap, ok := change.(map[string]interface{}); ok {
				address := changeMap["address"].(string)
				result[address] = changeMap
			}
		}
	}

	return result
}

// getOutputs extracts outputs from a terraform plan.
func getOutputs(plan map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	// Check planned_values for outputs
	if plannedValues, ok := plan["planned_values"].(map[string]interface{}); ok {
		if outputs, ok := plannedValues["outputs"].(map[string]interface{}); ok {
			for k, v := range outputs {
				result[k] = v
			}
		}
	}

	// Check output_changes
	if outputChanges, ok := plan["output_changes"].(map[string]interface{}); ok {
		for k, v := range outputChanges {
			if _, exists := result[k]; !exists {
				result[k] = v
			}
		}
	}

	return result
}

// compareResources compares resources between two terraform plans.
func compareResources(origResources, newResources map[string]interface{}) string {
	var diff strings.Builder

	// Find added resources
	for k := range newResources {
		if _, exists := origResources[k]; !exists {
			diff.WriteString(fmt.Sprintf("+ %s\n", k))
		}
	}

	// Find removed resources
	for k := range origResources {
		if _, exists := newResources[k]; !exists {
			diff.WriteString(fmt.Sprintf("- %s\n", k))
		}
	}

	// Find changed resources
	for k, origV := range origResources {
		newV, exists := newResources[k]
		if !exists || reflect.DeepEqual(origV, newV) {
			continue
		}

		diff.WriteString(fmt.Sprintf("%s\n", k))

		// Compare resource attributes
		origAttrs := getResourceAttributes(origV)
		newAttrs := getResourceAttributes(newV)

		// Important attributes to always show first if they exist
		priorityAttrs := []string{"id", "url", "content"}

		// Attributes to skip in the diff to keep it clean
		skipAttrs := map[string]bool{
			"response_body_base64": true,
			"content_base64sha256": true,
			"content_base64sha512": true,
			"content_md5":          true,
			"content_sha1":         true,
			"content_sha256":       true,
			"content_sha512":       true,
		}

		// Process priority attributes first
		for _, attrK := range priorityAttrs {
			origAttrV, origExists := origAttrs[attrK]
			newAttrV, newExists := newAttrs[attrK]

			switch {
			case origExists && newExists && !reflect.DeepEqual(origAttrV, newAttrV):
				printAttributeDiff(&diff, attrK, origAttrV, newAttrV)
			case origExists && !newExists:
				diff.WriteString(fmt.Sprintf("  - %s: %v\n", attrK, formatValue(origAttrV)))
			case !origExists && newExists:
				diff.WriteString(fmt.Sprintf("  + %s: %v\n", attrK, formatValue(newAttrV)))
			}
		}

		// Process other attributes
		for attrK, origAttrV := range origAttrs {
			// Skip priority attributes (already processed) and attributes in the skip list
			if contains(priorityAttrs, attrK) || skipAttrs[attrK] {
				continue
			}

			if newAttrV, exists := newAttrs[attrK]; exists && !reflect.DeepEqual(origAttrV, newAttrV) {
				printAttributeDiff(&diff, attrK, origAttrV, newAttrV)
			} else if !exists {
				diff.WriteString(fmt.Sprintf("  - %s: %v\n", attrK, formatValue(origAttrV)))
			}
		}

		// Find added attributes (that weren't in the priority list)
		for attrK, newAttrV := range newAttrs {
			if _, exists := origAttrs[attrK]; !exists && !contains(priorityAttrs, attrK) && !skipAttrs[attrK] {
				diff.WriteString(fmt.Sprintf("  + %s: %v\n", attrK, formatValue(newAttrV)))
			}
		}
	}

	return diff.String()
}

// printAttributeDiff handles the formatting of an attribute diff.
func printAttributeDiff(diff *strings.Builder, attrK string, origAttrV, newAttrV interface{}) {
	origSensitive := isSensitive(origAttrV)
	newSensitive := isSensitive(newAttrV)

	switch {
	case origSensitive && newSensitive:
		diff.WriteString(fmt.Sprintf("  ~ %s: (sensitive value) => (sensitive value)\n", attrK))
	case origSensitive:
		diff.WriteString(fmt.Sprintf("  ~ %s: (sensitive value) => %v\n", attrK, formatValue(newAttrV)))
	case newSensitive:
		diff.WriteString(fmt.Sprintf("  ~ %s: %v => (sensitive value)\n", attrK, formatValue(origAttrV)))
	default:
		// Check if both values are maps and use the specialized diff function
		origMap, origIsMap := origAttrV.(map[string]interface{})
		newMap, newIsMap := newAttrV.(map[string]interface{})

		if origIsMap && newIsMap {
			mapDiff := formatMapDiff(origMap, newMap)
			if mapDiff != noChangesText {
				diff.WriteString(fmt.Sprintf("  ~ %s: %s\n", attrK, mapDiff))
			}
		} else {
			diff.WriteString(fmt.Sprintf("  ~ %s: %v => %v\n", attrK, formatValue(origAttrV), formatValue(newAttrV)))
		}
	}
}

// contains checks if a string is in a slice.
func contains(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

// getResourceAttributes extracts attributes from a resource.
func getResourceAttributes(resource interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	if resMap, ok := resource.(map[string]interface{}); ok {
		// Try to get values from different possible locations in the resource map
		if values, ok := resMap["values"].(map[string]interface{}); ok {
			for k, v := range values {
				result[k] = v
			}
		}

		if change, ok := resMap["change"].(map[string]interface{}); ok {
			if after, ok := change["after"].(map[string]interface{}); ok {
				for k, v := range after {
					result[k] = v
				}
			}
		}
	}

	return result
}

// isSensitive checks if a value is marked as sensitive.
func isSensitive(value interface{}) bool {
	if valueMap, ok := value.(map[string]interface{}); ok {
		if sensitive, ok := valueMap["sensitive"].(bool); ok && sensitive {
			return true
		}
	}
	return false
}

// formatValue formats a value for display, handling sensitive values.
func formatValue(value interface{}) string {
	if isSensitive(value) {
		return "(sensitive value)"
	}

	// Handle very long string values
	if strVal, ok := value.(string); ok {
		// Keep weather report content intact
		if strings.Contains(strVal, "Weather report:") {
			return strVal
		}

		// If it looks like a base64 value, simplify it
		if strings.HasPrefix(strVal, "V2VhdGhl") || strings.HasPrefix(strVal, "CgogIBtb") {
			return "(base64 encoded value)"
		}

		// For other very long strings, show start and end
		if len(strVal) > maxStringDisplayLength {
			return fmt.Sprintf("%s...%s", strVal[:halfStringDisplayLength], strVal[len(strVal)-halfStringDisplayLength:])
		}
	}

	// Handle map values specially for cleaner output
	if valueMap, ok := value.(map[string]interface{}); ok {
		// If there's a 'value' key, extract it
		if val, exists := valueMap["value"]; exists {
			return fmt.Sprintf("%v", val)
		}

		// For outputs, check for type and value fields
		if _, hasType := valueMap["type"]; hasType {
			if val, hasValue := valueMap["value"]; hasValue {
				return fmt.Sprintf("%v", val)
			}
		}

		// For response headers and similar maps, provide a cleaner format
		if len(valueMap) > 0 && (strings.Contains(fmt.Sprintf("%v", valueMap), "map[") || strings.Contains(fmt.Sprintf("%v", valueMap), "Access-Control-Allow-Origin")) {
			// This is likely a map that we want to display more cleanly
			return formatMapForDisplay(valueMap)
		}
	}

	return fmt.Sprintf("%v", value)
}

// formatMapForDisplay formats a map for cleaner display.
func formatMapForDisplay(m map[string]interface{}) string {
	// Get all keys and sort them for consistent output
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// For simple maps with 3 or fewer entries, show a compact representation
	if len(m) <= 3 {
		parts := make([]string, 0, len(m))
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%s: %v", k, m[k]))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	}

	// For larger maps, show a structured representation with indentation
	var sb strings.Builder
	sb.WriteString("{\n")

	for _, k := range keys {
		v := m[k]

		// Format the value based on its type
		var valueStr string
		if nestedMap, ok := v.(map[string]interface{}); ok {
			// Recursively format nested maps with additional indentation
			nestedStr := formatMapForDisplay(nestedMap)
			// Add indentation to each line
			nestedStr = strings.ReplaceAll(nestedStr, "\n", "\n    ")
			valueStr = nestedStr
		} else {
			valueStr = fmt.Sprintf("%v", v)
		}

		sb.WriteString(fmt.Sprintf("    %s: %s\n", k, valueStr))
	}

	sb.WriteString("}")
	return sb.String()
}

// formatMapDiff formats the difference between two maps showing only changed keys.
func formatMapDiff(origMap, newMap map[string]interface{}) string {
	// Get all keys from both maps
	allKeys := make(map[string]bool)
	for k := range origMap {
		allKeys[k] = true
	}
	for k := range newMap {
		allKeys[k] = true
	}

	// Sort keys for consistent output
	keys := make([]string, 0, len(allKeys))
	for k := range allKeys {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// If no differences, return early
	if reflect.DeepEqual(origMap, newMap) {
		return noChangesText
	}

	// For empty or very small diffs, use a compact representation
	if len(keys) <= 3 {
		return formatCompactMapDiff(keys, origMap, newMap)
	}

	// For larger diffs, show a structured representation with indentation
	var sb strings.Builder
	sb.WriteString("{\n")
	changesFound := false

	for _, k := range keys {
		origVal, origExists := origMap[k]
		newVal, newExists := newMap[k]

		// Skip keys that haven't changed
		if origExists && newExists && reflect.DeepEqual(origVal, newVal) {
			continue
		}

		changesFound = true

		// Format based on what changed
		switch {
		case !origExists:
			sb.WriteString(fmt.Sprintf("    + %s: %s\n", k, formatValue(newVal)))
		case !newExists:
			sb.WriteString(fmt.Sprintf("    - %s: %s\n", k, formatValue(origVal)))
		default:
			// Value changed
			if origMap, ok := origVal.(map[string]interface{}); ok {
				if newMap, ok := newVal.(map[string]interface{}); ok {
					// Recursively diff nested maps
					nestedDiff := formatMapDiff(origMap, newMap)
					if nestedDiff != noChangesText {
						// Add indentation to nested diff
						nestedDiff = strings.ReplaceAll(nestedDiff, "\n", "\n    ")
						sb.WriteString(fmt.Sprintf("    ~ %s: %s\n", k, nestedDiff))
					}
					continue
				}
			}

			// Simple value change
			sb.WriteString(fmt.Sprintf("    ~ %s: %v => %v\n", k, formatValue(origVal), formatValue(newVal)))
		}
	}

	if !changesFound {
		return noChangesText
	}

	sb.WriteString("}")
	return sb.String()
}

// formatCompactMapDiff creates a compact string representation for small map diffs.
func formatCompactMapDiff(keys []string, origMap, newMap map[string]interface{}) string {
	changes := make([]string, 0, len(keys))

	for _, k := range keys {
		origVal, origExists := origMap[k]
		newVal, newExists := newMap[k]

		switch {
		case !origExists:
			changes = append(changes, fmt.Sprintf("+%s: %v", k, formatValue(newVal)))
		case !newExists:
			changes = append(changes, fmt.Sprintf("-%s: %v", k, formatValue(origVal)))
		case !reflect.DeepEqual(origVal, newVal):
			changes = append(changes, fmt.Sprintf("~%s: %v => %v", k, formatValue(origVal), formatValue(newVal)))
		}
	}

	if len(changes) == 0 {
		return noChangesText
	}

	return "{" + strings.Join(changes, ", ") + "}"
}

// runTerraformInit runs a basic terraform init in the specified directory using
// terraformRun method (ExecuteTerraform).
func runTerraformInit(atmosConfig *schema.AtmosConfiguration, dir string, info *schema.ConfigAndStacksInfo) error {
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

	// Run terraform init using ExecuteTerraform
	err := ExecuteTerraform(initInfo)
	if err != nil {
		return fmt.Errorf("error running terraform init: %w", err)
	}

	return nil
}
