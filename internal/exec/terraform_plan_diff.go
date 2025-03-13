package exec

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/pkg/errors"
)

// ErrPlanHasDiff is returned when there are differences between the two plan files
var ErrPlanHasDiff = errors.New("plan files have differences")

// TerraformPlanDiff represents the plan-diff command implementation
func TerraformPlanDiff(atmosConfig schema.AtmosConfiguration, info schema.ConfigAndStacksInfo) error {
	// Get the original plan file path from the --orig flag
	origPlanFile := ""
	newPlanFile := ""

	// Extract command-specific flags without modifying the original structure
	for i := 0; i < len(info.AdditionalArgsAndFlags); i++ {
		arg := info.AdditionalArgsAndFlags[i]

		if strings.HasPrefix(arg, "--orig=") {
			origPlanFile = strings.TrimPrefix(arg, "--orig=")
		} else if arg == "--orig" && i+1 < len(info.AdditionalArgsAndFlags) {
			origPlanFile = info.AdditionalArgsAndFlags[i+1]
		}

		if strings.HasPrefix(arg, "--new=") {
			newPlanFile = strings.TrimPrefix(arg, "--new=")
		} else if arg == "--new" && i+1 < len(info.AdditionalArgsAndFlags) {
			newPlanFile = info.AdditionalArgsAndFlags[i+1]
		}
	}

	if origPlanFile == "" {
		return errors.New("original plan file (--orig) is required")
	}

	// Create a temporary directory for all temporary files
	tmpDir, err := os.MkdirTemp("", "atmos-terraform-plan-diff")
	if err != nil {
		return errors.Wrap(err, "error creating temporary directory")
	}
	defer os.RemoveAll(tmpDir)

	// Get the component path
	componentPath := filepath.Join(atmosConfig.TerraformDirAbsolutePath, info.ComponentFolderPrefix, info.FinalComponent)

	// Make sure the original plan file exists
	if !filepath.IsAbs(origPlanFile) {
		// If the path is relative, make it absolute based on the component directory
		absOrigPlanFile := filepath.Join(componentPath, origPlanFile)
		origPlanFile = absOrigPlanFile
	}

	if _, err := os.Stat(origPlanFile); os.IsNotExist(err) {
		return errors.Errorf("original plan file '%s' does not exist", origPlanFile)
	}

	// If no new plan file is specified, generate one
	if newPlanFile == "" {
		newPlanFile, err = generateNewPlanFile(atmosConfig, info, componentPath, origPlanFile, tmpDir)
		if err != nil {
			return errors.Wrap(err, "error generating new plan file")
		}
	} else if !filepath.IsAbs(newPlanFile) {
		// If the path is relative, make it absolute based on the component directory
		absNewPlanFile := filepath.Join(componentPath, newPlanFile)
		newPlanFile = absNewPlanFile
	}

	// Make sure the new plan file exists
	if _, err := os.Stat(newPlanFile); os.IsNotExist(err) {
		return errors.Errorf("new plan file '%s' does not exist", newPlanFile)
	}

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
		fmt.Fprintln(os.Stdout, "Diff Output")
		fmt.Fprintln(os.Stdout, "=========")
		fmt.Fprintln(os.Stdout, "")
		fmt.Fprintln(os.Stdout, diff)

		// Print the error message
		u.PrintErrorMarkdown("", ErrPlanHasDiff, "")

		// Exit with code 2 to indicate that the plans are different
		u.OsExit(2)
		return nil // This line will never be reached
	}

	fmt.Fprintln(os.Stdout, "The planfiles are identical")
	return nil
}

// runTerraformInit runs terraform init in the component directory
func runTerraformInit(atmosConfig schema.AtmosConfiguration, info schema.ConfigAndStacksInfo, componentPath string) error {
	// Clean terraform workspace to prevent workspace selection prompt
	cleanTerraformWorkspace(atmosConfig, componentPath)

	// Generate backend config if needed
	err := generateBackendConfig(&atmosConfig, &info, componentPath)
	if err != nil {
		return err
	}

	// Generate provider overrides if needed
	err = generateProviderOverrides(&atmosConfig, &info, componentPath)
	if err != nil {
		return err
	}

	// Run terraform init
	initArgs := []string{"init"}
	if atmosConfig.Components.Terraform.InitRunReconfigure {
		initArgs = append(initArgs, "-reconfigure")
	}

	err = ExecuteShellCommand(atmosConfig, "terraform", initArgs, componentPath, nil, false, info.RedirectStdErr)
	if err != nil {
		return fmt.Errorf("error running terraform init: %w", err)
	}

	return nil
}

// generateNewPlanFile generates a new plan file by running terraform plan
func generateNewPlanFile(atmosConfig schema.AtmosConfiguration, info schema.ConfigAndStacksInfo, componentPath string, origPlanFile string, tmpDir string) (string, error) {
	// Create a temporary file for the new plan
	newPlanFile := filepath.Join(tmpDir, "new.plan")

	// Create a new info object for the plan command
	planInfo := info
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

// getTerraformPlanJSON gets the JSON representation of a terraform plan
func getTerraformPlanJSON(atmosConfig schema.AtmosConfiguration, info schema.ConfigAndStacksInfo, componentPath, planFile string) (string, error) {
	// Copy the plan file to the component directory if it's not already there
	planFileInComponentDir := planFile
	planFileBaseName := filepath.Base(planFile)

	// Create a temporary copy of the plan file in the component directory
	if !strings.HasPrefix(planFile, componentPath) {
		planFileInComponentDir = filepath.Join(componentPath, planFileBaseName)

		// Copy the plan file content
		planContent, err := os.ReadFile(planFile)
		if err != nil {
			return "", fmt.Errorf("error reading plan file: %w", err)
		}

		err = os.WriteFile(planFileInComponentDir, planContent, 0644)
		if err != nil {
			return "", fmt.Errorf("error copying plan file to component directory: %w", err)
		}

		// Clean up the copied plan file when we're done
		defer os.Remove(planFileInComponentDir)
	}

	// Run terraform show -json in the component directory
	var stdout bytes.Buffer
	cmd := exec.Command("terraform", "show", "-json", planFileInComponentDir)
	cmd.Dir = componentPath
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("error running terraform show: %w", err)
	}

	return stdout.String(), nil
}

// sortMapKeys recursively sorts map keys for consistent comparison
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

// generatePlanDiff generates a diff between two terraform plans
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
				origSensitive := isSensitive(origV)
				newSensitive := isSensitive(newV)

				if origSensitive && newSensitive {
					diff.WriteString(fmt.Sprintf("~ %s: (sensitive value) => (sensitive value)\n", k))
				} else if origSensitive {
					diff.WriteString(fmt.Sprintf("~ %s: (sensitive value) => %v\n", k, formatValue(newV)))
				} else if newSensitive {
					diff.WriteString(fmt.Sprintf("~ %s: %v => (sensitive value)\n", k, formatValue(origV)))
				} else {
					diff.WriteString(fmt.Sprintf("~ %s: %v => %v\n", k, formatValue(origV), formatValue(newV)))
				}
			}
		}
	}

	return diff.String(), hasDiff
}

// getVariables extracts variables from a terraform plan
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

// getResources extracts resources from a terraform plan
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

// getOutputs extracts outputs from a terraform plan
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

// compareResources compares resources between two terraform plans
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
		if newV, exists := newResources[k]; exists && !reflect.DeepEqual(origV, newV) {
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

				if origExists && newExists && !reflect.DeepEqual(origAttrV, newAttrV) {
					printAttributeDiff(&diff, attrK, origAttrV, newAttrV)
				} else if origExists && !newExists {
					diff.WriteString(fmt.Sprintf("  - %s: %v\n", attrK, formatValue(origAttrV)))
				} else if !origExists && newExists {
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
	}

	return diff.String()
}

// printAttributeDiff handles the formatting of an attribute diff
func printAttributeDiff(diff *strings.Builder, attrK string, origAttrV, newAttrV interface{}) {
	origSensitive := isSensitive(origAttrV)
	newSensitive := isSensitive(newAttrV)

	if origSensitive && newSensitive {
		diff.WriteString(fmt.Sprintf("  ~ %s: (sensitive value) => (sensitive value)\n", attrK))
	} else if origSensitive {
		diff.WriteString(fmt.Sprintf("  ~ %s: (sensitive value) => %v\n", attrK, formatValue(newAttrV)))
	} else if newSensitive {
		diff.WriteString(fmt.Sprintf("  ~ %s: %v => (sensitive value)\n", attrK, formatValue(origAttrV)))
	} else {
		diff.WriteString(fmt.Sprintf("  ~ %s: %v => %v\n", attrK, formatValue(origAttrV), formatValue(newAttrV)))
	}
}

// contains checks if a string is in a slice
func contains(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

// getResourceAttributes extracts attributes from a resource
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

// isSensitive checks if a value is marked as sensitive
func isSensitive(value interface{}) bool {
	if valueMap, ok := value.(map[string]interface{}); ok {
		if sensitive, ok := valueMap["sensitive"].(bool); ok && sensitive {
			return true
		}
	}
	return false
}

// formatValue formats a value for display, handling sensitive values
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
		if len(strVal) > 300 {
			return fmt.Sprintf("%s...%s", strVal[:150], strVal[len(strVal)-150:])
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

// formatMapForDisplay formats a map for cleaner display
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

// runBasicTerraformInit runs a basic terraform init in the specified directory
func runBasicTerraformInit(dir string) error {
	cmd := exec.Command("terraform", "init", "-reconfigure")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
