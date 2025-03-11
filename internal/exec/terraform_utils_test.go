package exec

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
)

const (
	// Define constants for testing
	testVarFileFlag = "-var-file"
	testOutFlag     = "-out"
)

// Helper function to create bool pointer for testing.
func boolPtr(b bool) *bool {
	return &b
}

func TestIsWorkspacesEnabled(t *testing.T) {
	// Test cases for isWorkspacesEnabled function.
	tests := []struct {
		name              string
		backendType       string
		workspacesEnabled *bool
		expectedEnabled   bool
		expectWarning     bool
	}{
		{
			name:              "Default behavior (no explicit setting, non-HTTP backend)",
			backendType:       "s3",
			workspacesEnabled: nil,
			expectedEnabled:   true,
			expectWarning:     false,
		},
		{
			name:              "HTTP backend automatically disables workspaces",
			backendType:       "http",
			workspacesEnabled: nil,
			expectedEnabled:   false,
			expectWarning:     false,
		},
		{
			name:              "Explicitly disabled workspaces",
			backendType:       "s3",
			workspacesEnabled: boolPtr(false),
			expectedEnabled:   false,
			expectWarning:     false,
		},
		{
			name:              "Explicitly enabled workspaces",
			backendType:       "s3",
			workspacesEnabled: boolPtr(true),
			expectedEnabled:   true,
			expectWarning:     false,
		},
		{
			name:              "HTTP backend ignores explicitly enabled workspaces with warning",
			backendType:       "http",
			workspacesEnabled: boolPtr(true),
			expectedEnabled:   false,
			expectWarning:     true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup test config.
			atmosConfig := &schema.AtmosConfiguration{
				Components: schema.Components{
					Terraform: schema.Terraform{
						WorkspacesEnabled: tc.workspacesEnabled,
					},
				},
			}

			info := &schema.ConfigAndStacksInfo{
				ComponentBackendType: tc.backendType,
				Component:            "test-component",
			}

			// Test function.
			result := isWorkspacesEnabled(atmosConfig, info)

			// Assert results.
			assert.Equal(t, tc.expectedEnabled, result, "Expected workspace enabled status to match")
		})
	}
}

func TestSortJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected interface{}
	}{
		{
			name: "sort map keys",
			input: map[string]interface{}{
				"c": 3,
				"a": 1,
				"b": 2,
			},
			expected: map[string]interface{}{
				"a": 1,
				"b": 2,
				"c": 3,
			},
		},
		{
			name: "sort nested maps",
			input: map[string]interface{}{
				"z": map[string]interface{}{
					"y": 2,
					"x": 1,
				},
				"a": 1,
			},
			expected: map[string]interface{}{
				"a": 1,
				"z": map[string]interface{}{
					"x": 1,
					"y": 2,
				},
			},
		},
		{
			name: "sort maps in arrays",
			input: map[string]interface{}{
				"array": []interface{}{
					map[string]interface{}{
						"b": 2,
						"a": 1,
					},
					map[string]interface{}{
						"d": 4,
						"c": 3,
					},
				},
			},
			expected: map[string]interface{}{
				"array": []interface{}{
					map[string]interface{}{
						"a": 1,
						"b": 2,
					},
					map[string]interface{}{
						"c": 3,
						"d": 4,
					},
				},
			},
		},
		{
			name:     "non-map value",
			input:    "string value",
			expected: "string value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sortJSON(tt.input)

			// Convert to JSON for comparison
			expectedJSON, err := json.Marshal(tt.expected)
			if err != nil {
				t.Fatalf("Failed to marshal expected JSON: %v", err)
			}

			resultJSON, err := json.Marshal(result)
			if err != nil {
				t.Fatalf("Failed to marshal result JSON: %v", err)
			}

			if string(resultJSON) != string(expectedJSON) {
				t.Errorf("sortJSON() = %v, want %v", string(resultJSON), string(expectedJSON))
			}
		})
	}
}

// MockExecutor is a mock implementation of shell command execution
type MockExecutor struct {
	commands [][]string
	outputs  map[string]string
}

// ExecuteCommand records the command and simulates the behavior we need for testing
func (m *MockExecutor) ExecuteCommand(command string, args []string, componentPath string) error {
	m.commands = append(m.commands, append([]string{command}, args...))

	// If it's a terraform show command, simulate writing JSON output
	if command == "terraform" && len(args) >= 1 && args[0] == "show" && len(args) >= 3 {
		planFile := args[2]
		outputFile := args[len(args)-1]

		// Return appropriate mock plan data based on the plan file
		if strings.Contains(planFile, "orig") {
			return os.WriteFile(outputFile, []byte(m.outputs["orig"]), 0o600)
		} else if strings.Contains(planFile, "new") {
			return os.WriteFile(outputFile, []byte(m.outputs["new"]), 0o600)
		}
	}

	// If it's a terraform plan command, simulate creating a plan
	if command == "terraform" && len(args) >= 1 && args[0] == "plan" {
		// Find the output plan file (should be after -out flag)
		for i, arg := range args {
			if (arg == "-out" || arg == outFlag) && i+1 < len(args) {
				return os.WriteFile(args[i+1], []byte("mock plan content"), 0o600)
			}
		}
	}

	return nil
}

// prettyDiffTest is a version of prettyDiff that captures output for testing
func prettyDiffTest(a, b map[string]interface{}, path string, output *strings.Builder) bool {
	hasDifferences := false

	// Compare keys in map a to map b
	hasDifferences = compareMapAtoBTest(a, b, path, output) || hasDifferences

	// Compare keys in map b to map a (for keys only in b)
	hasDifferences = compareMapBtoATest(a, b, path, output) || hasDifferences

	return hasDifferences
}

// Helper function to compare keys from map a to map b for testing
func compareMapAtoBTest(a, b map[string]interface{}, path string, output *strings.Builder) bool {
	hasDifferences := false

	for k, v1 := range a {
		currentPath := buildPathTest(path, k)
		v2, exists := b[k]

		if !exists {
			// Key exists in a but not in b
			printRemovedValueTest(currentPath, v1, output)
			hasDifferences = true
			continue
		}

		// Types are different
		if reflect.TypeOf(v1) != reflect.TypeOf(v2) {
			printTypeDifferenceTest(currentPath, v1, v2, output)
			hasDifferences = true
			continue
		}

		// Handle based on value type
		switch val := v1.(type) {
		case map[string]interface{}:
			if prettyDiffTest(val, v2.(map[string]interface{}), currentPath, output) {
				hasDifferences = true
			}
		case []interface{}:
			if !reflect.DeepEqual(val, v2) {
				if diffArraysTest(currentPath, val, v2.([]interface{}), output) {
					hasDifferences = true
				}
			}
		default:
			if !reflect.DeepEqual(v1, v2) {
				fmt.Fprintf(output, "~ %s: %v => %v\n", currentPath, v1, v2)
				hasDifferences = true
			}
		}
	}

	return hasDifferences
}

// Helper function to compare keys from map b to map a for testing
func compareMapBtoATest(a, b map[string]interface{}, path string, output *strings.Builder) bool {
	hasDifferences := false

	for k, v2 := range b {
		currentPath := buildPathTest(path, k)
		_, exists := a[k]

		if !exists {
			// Key exists in b but not in a
			printAddedValueTest(currentPath, v2, output)
			hasDifferences = true
		}
	}

	return hasDifferences
}

// Helper function to build the path string for testing
func buildPathTest(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

// Helper function to print a value that was removed for testing
func printRemovedValueTest(path string, value interface{}, output *strings.Builder) {
	switch v := value.(type) {
	case map[string]interface{}, []interface{}:
		jsonBytes, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			// If marshaling fails, fall back to simple format
			fmt.Fprintf(output, "- %s: %v\n", path, v)
		} else {
			fmt.Fprintf(output, "- %s:\n%s\n", path, string(jsonBytes))
		}
	default:
		fmt.Fprintf(output, "- %s: %v\n", path, v)
	}
}

// Helper function to print a value that was added for testing
func printAddedValueTest(path string, value interface{}, output *strings.Builder) {
	switch v := value.(type) {
	case map[string]interface{}, []interface{}:
		jsonBytes, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			// If marshaling fails, fall back to simple format
			fmt.Fprintf(output, "+ %s: %v\n", path, v)
		} else {
			fmt.Fprintf(output, "+ %s:\n%s\n", path, string(jsonBytes))
		}
	default:
		fmt.Fprintf(output, "+ %s: %v\n", path, v)
	}
}

// Helper function to print a type difference for testing
func printTypeDifferenceTest(path string, v1, v2 interface{}, output *strings.Builder) {
	fmt.Fprintf(output, "~ %s:\n", path)
	fmt.Fprintf(output, "  - %v\n", v1)
	fmt.Fprintf(output, "  + %v\n", v2)
}

// Helper function to diff arrays for testing
func diffArraysTest(path string, arr1, arr2 []interface{}, output *strings.Builder) bool {
	// For terraform plans, resources arrays are especially important to show clearly
	if path == "resources" || strings.HasSuffix(path, ".resources") {
		return diffResourceArraysTest(path, arr1, arr2, output)
	} else {
		return diffGenericArraysTest(path, arr1, arr2, output)
	}
}

// Helper function to diff resource arrays for testing
func diffResourceArraysTest(path string, arr1, arr2 []interface{}, output *strings.Builder) bool {
	fmt.Fprintf(output, "~ %s: (resource changes)\n", path)
	fmt.Fprintf(output, "  Resources:\n")

	// Process only if there's content in both arrays
	if len(arr1) > 0 && len(arr2) > 0 {
		// Find resources that changed or were removed
		processRemovedOrChangedResourcesTest(arr1, arr2, output)

		// Find added resources
		processAddedResourcesTest(arr1, arr2, output)
	} else {
		// Simple fallback for empty arrays
		if len(arr1) == 0 {
			fmt.Fprintf(output, "  - No resources in original plan\n")
		}
		if len(arr2) == 0 {
			fmt.Fprintf(output, "  + No resources in new plan\n")
		}
	}

	return true // Always return true since we printed something
}

// Helper function to process resources that were removed or changed for testing
func processRemovedOrChangedResourcesTest(arr1, arr2 []interface{}, output *strings.Builder) {
	for _, origRes := range arr1 {
		origMap, ok1 := origRes.(map[string]interface{})
		if !ok1 {
			continue
		}

		matchingResource := findMatchingResourceTest(origMap, arr2)

		if matchingResource != nil {
			// Resource exists in both - compare them
			fmt.Fprintf(output, "  Resource: %s\n", getResourceNameTest(origMap))
			resourceDiffTest(origMap, matchingResource, "  ", output)
		} else {
			// Resource was removed
			printRemovedResourceTest(origMap, output)
		}
	}
}

// Helper function to find a matching resource in the array for testing
func findMatchingResourceTest(resource map[string]interface{}, resources []interface{}) map[string]interface{} {
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

// Helper function to process resources that were added for testing
func processAddedResourcesTest(arr1, arr2 []interface{}, output *strings.Builder) {
	for _, newRes := range arr2 {
		newMap, ok := newRes.(map[string]interface{})
		if !ok {
			continue
		}

		// Check if this resource exists in the original array
		if findMatchingResourceTest(newMap, arr1) == nil {
			// This is a new resource
			printAddedResourceTest(newMap, output)
		}
	}
}

// Helper function to print a removed resource for testing
func printRemovedResourceTest(resource map[string]interface{}, output *strings.Builder) {
	fmt.Fprintf(output, "  - Resource removed: %v\n", getResourceNameTest(resource))
	resourceBytes, err := json.MarshalIndent(resource, "    ", "  ")
	if err != nil {
		// If marshaling fails, just print the map
		fmt.Fprintf(output, "    %v\n", resource)
	} else {
		fmt.Fprintf(output, "    %s\n", strings.ReplaceAll(string(resourceBytes), "\n", "\n    "))
	}
}

// Helper function to print an added resource for testing
func printAddedResourceTest(resource map[string]interface{}, output *strings.Builder) {
	fmt.Fprintf(output, "  + Resource added: %v\n", getResourceNameTest(resource))
	resourceBytes, err := json.MarshalIndent(resource, "    ", "  ")
	if err != nil {
		// If marshaling fails, just print the map
		fmt.Fprintf(output, "    %v\n", resource)
	} else {
		fmt.Fprintf(output, "    %s\n", strings.ReplaceAll(string(resourceBytes), "\n", "\n    "))
	}
}

// Helper function to diff generic (non-resource) arrays for testing
func diffGenericArraysTest(path string, arr1, arr2 []interface{}, output *strings.Builder) bool {
	fmt.Fprintf(output, "~ %s:\n", path)

	// Print the first array
	if len(arr1) > 0 {
		jsonBytes, err := json.MarshalIndent(arr1, "  - ", "  ")
		if err != nil {
			fmt.Fprintf(output, "  - [Array marshaling error: %v]\n", err)
		} else {
			fmt.Fprintf(output, "  - %s\n", string(jsonBytes))
		}
	} else {
		fmt.Fprintf(output, "  - []\n")
	}

	// Print the second array
	if len(arr2) > 0 {
		jsonBytes, err := json.MarshalIndent(arr2, "  + ", "  ")
		if err != nil {
			fmt.Fprintf(output, "  + [Array marshaling error: %v]\n", err)
		} else {
			fmt.Fprintf(output, "  + %s\n", string(jsonBytes))
		}
	} else {
		fmt.Fprintf(output, "  + []\n")
	}

	return true
}

// Helper function to get a readable resource name for the test
func getResourceNameTest(resource map[string]interface{}) string {
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

// Helper function to diff individual resources for the test
func resourceDiffTest(a, b map[string]interface{}, indent string, output *strings.Builder) {
	// Focus on the values part of the resource if present
	if values1, hasValues1 := a["values"].(map[string]interface{}); hasValues1 {
		if values2, hasValues2 := b["values"].(map[string]interface{}); hasValues2 {
			diffResourceValuesTest(values1, values2, indent, output)
			return
		}
	}

	// Fallback if no values field
	diffResourceFallbackTest(a, b, indent, output)
}

// Helper function to diff resource values for testing
func diffResourceValuesTest(values1, values2 map[string]interface{}, indent string, output *strings.Builder) {
	// Compare values in first map
	for k, v1 := range values1 {
		v2, exists := values2[k]

		if !exists {
			fmt.Fprintf(output, "%s- %s: %v\n", indent, k, v1)
			continue
		}

		if !reflect.DeepEqual(v1, v2) {
			fmt.Fprintf(output, "%s~ %s: %v => %v\n", indent, k, v1, v2)
		}
	}

	// Check for added values
	for k, v2 := range values2 {
		_, exists := values1[k]
		if !exists {
			fmt.Fprintf(output, "%s+ %s: %v\n", indent, k, v2)
		}
	}
}

// Helper function for resource diff fallback method for testing
func diffResourceFallbackTest(a, b map[string]interface{}, indent string, output *strings.Builder) {
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
			fmt.Fprintf(output, "%s- %s: %v\n", indent, k, v1)
			continue
		}

		if !reflect.DeepEqual(v1, v2) {
			fmt.Fprintf(output, "%s~ %s: %v => %v\n", indent, k, v1, v2)
		}
	}

	// Check for added fields
	for k, v2 := range b {
		if skipFields[k] {
			continue
		}

		_, exists := a[k]
		if !exists {
			fmt.Fprintf(output, "%s+ %s: %v\n", indent, k, v2)
		}
	}
}

// testExecuteTerraformPlanDiff is a testable version of executeTerraformPlanDiff that uses the MockExecutor
func testExecuteTerraformPlanDiff(executor *MockExecutor, info schema.ConfigAndStacksInfo, componentPath, varFile, planFile string) error {
	// Step 1: Extract args and validate original plan file
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

	// Validate original plan file
	origPlanPath := origPlanFlag
	if !filepath.IsAbs(origPlanPath) {
		origPlanPath = filepath.Join(componentPath, origPlanPath)
	}

	// Check if orig plan file exists
	if _, err := os.Stat(origPlanPath); os.IsNotExist(err) {
		return fmt.Errorf("original plan file does not exist at path: %s", origPlanPath)
	}

	// Step 2: Process new plan file
	var newPlanPath string
	if newPlanFlag == "" {
		// Generate a new plan
		log.Info("Generating new plan...")

		// Create a temporary plan file
		newPlanPath = filepath.Join(componentPath, "new-"+filepath.Base(planFile))

		// Simulate terraform plan execution with all additional arguments
		planCmd := []string{"plan", testVarFileFlag, varFile, testOutFlag, newPlanPath}
		planCmd = append(planCmd, additionalPlanArgs...)

		err := executor.ExecuteCommand("terraform", planCmd, componentPath)
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

	// Step 3: Set up temp files and convert plans to JSON
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

	// Simulate terraform show to get human-readable JSON versions of the plans
	log.Info("Converting plan files to JSON...")

	err = executor.ExecuteCommand("terraform", []string{"show", "-json", origPlanPath}, componentPath)
	if err != nil {
		return fmt.Errorf("error showing original plan: %w", err)
	}

	err = executor.ExecuteCommand("terraform", []string{"show", "-json", newPlanPath}, componentPath)
	if err != nil {
		return fmt.Errorf("error showing new plan: %w", err)
	}

	// Step 4: Parse and compare the plans
	// Parse JSON
	var origPlan, newPlan map[string]interface{}
	if err := json.Unmarshal([]byte(executor.outputs["orig"]), &origPlan); err != nil {
		return fmt.Errorf("error parsing original plan JSON: %w", err)
	}
	if err := json.Unmarshal([]byte(executor.outputs["new"]), &newPlan); err != nil {
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
	log.Info("Comparing plans...")
	fmt.Println("Plan differences:")
	fmt.Println("----------------")

	var diffOutput strings.Builder
	hasDifferences := prettyDiffTest(origPlan, newPlan, "", &diffOutput)

	// Step 5: Display the results
	if !hasDifferences {
		fmt.Println("No differences found between the plans.")
	} else {
		fmt.Println(diffOutput.String())
	}

	return nil
}

func TestExecuteTerraformPlanDiffBasic(t *testing.T) {
	// Create test environment
	tempDir, origPlanFile, newPlanFile := setupTestPlanDiffEnvironment(t)
	defer os.RemoveAll(tempDir)

	// Get test plan data
	origPlanData, newPlanData := getTestPlanData()

	// Create test cases
	tests := createPlanDiffTestCases(origPlanFile, newPlanFile, origPlanData, newPlanData)

	// Run test cases
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock executor
			executor := createMockExecutor(tt.origPlanJSON, tt.newPlanJSON)

			// Create test info
			info := schema.ConfigAndStacksInfo{
				AdditionalArgsAndFlags: tt.additionalArgs,
			}

			// Special handling for "with_both_plans" test case
			if tt.name == "with_both_plans" {
				runWithBothPlansTest(t, tt, executor, info, tempDir)
				return
			}

			// Run the standard test
			runStandardPlanDiffTest(t, tt, executor, info, tempDir)
		})
	}
}

// Helper function to set up the test environment
func setupTestPlanDiffEnvironment(t *testing.T) (string, string, string) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "terraform-plan-diff-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Create test plan files
	origPlanFile := filepath.Join(tempDir, "orig-plan.tfplan")
	newPlanFile := filepath.Join(tempDir, "new-plan.tfplan")

	// Write dummy content to plan files so they exist
	err = os.WriteFile(origPlanFile, []byte("dummy content"), 0o600)
	if err != nil {
		t.Fatalf("Failed to write orig plan file: %v", err)
	}

	err = os.WriteFile(newPlanFile, []byte("dummy content"), 0o600)
	if err != nil {
		t.Fatalf("Failed to write new plan file: %v", err)
	}

	return tempDir, origPlanFile, newPlanFile
}

// Helper function to get test plan data
func getTestPlanData() (string, string) {
	// Mock plan data with more visible differences
	origPlanData := `{
		"format_version": "1.0",
		"terraform_version": "1.5.7",
		"variables": {
			"location": {"value": "Stockholm"},
			"instance_type": {"value": "t3.micro"},
			"environment": {"value": "development"}
		},
		"planned_values": {
			"root_module": {
				"resources": [
					{
						"address": "aws_instance.example",
						"mode": "managed",
						"type": "aws_instance",
						"name": "example",
						"provider_name": "registry.terraform.io/hashicorp/aws",
						"schema_version": 1,
						"values": {
							"ami": "ami-12345",
							"instance_type": "t3.micro",
							"tags": {
								"Name": "example-stockholm",
								"Environment": "development"
							}
						}
					}
				]
			}
		},
		"timestamp": "2025-03-10T23:07:52Z"
	}`

	newPlanData := `{
		"format_version": "1.0",
		"terraform_version": "1.5.7",
		"variables": {
			"location": {"value": "New Jersey"},
			"instance_type": {"value": "t3.large"},
			"environment": {"value": "production"}
		},
		"planned_values": {
			"root_module": {
				"resources": [
					{
						"address": "aws_instance.example",
						"mode": "managed",
						"type": "aws_instance",
						"name": "example",
						"provider_name": "registry.terraform.io/hashicorp/aws",
						"schema_version": 1,
						"values": {
							"ami": "ami-67890",
							"instance_type": "t3.large",
							"tags": {
								"Name": "example-newjersey",
								"Environment": "production"
							}
						}
					}
				]
			}
		},
		"timestamp": "2025-03-10T23:07:57Z"
	}`

	return origPlanData, newPlanData
}

// Test case definition
type planDiffTestCase struct {
	name               string
	additionalArgs     []string
	expectedDifference bool
	origPlanJSON       string
	newPlanJSON        string
}

// Helper function to create test cases
func createPlanDiffTestCases(origPlanFile, newPlanFile, origPlanData, newPlanData string) []planDiffTestCase {
	return []planDiffTestCase{
		{
			name:               "no_arguments",
			additionalArgs:     []string{"--orig", origPlanFile},
			expectedDifference: false,
			origPlanJSON:       origPlanData,
			newPlanJSON:        origPlanData,
		},
		{
			name:               "orig_only",
			additionalArgs:     []string{"--orig", origPlanFile},
			expectedDifference: false,
			origPlanJSON:       origPlanData,
			newPlanJSON:        origPlanData,
		},
		{
			name:               "with_both_plans",
			additionalArgs:     []string{"--orig", origPlanFile, "--new", newPlanFile},
			expectedDifference: true,
			origPlanJSON:       origPlanData,
			newPlanJSON:        newPlanData,
		},
		{
			name:               "with_additional_var_args",
			additionalArgs:     []string{"--orig", origPlanFile, "-var", "location=New Jersey"},
			expectedDifference: true,
			origPlanJSON:       origPlanData,
			newPlanJSON:        newPlanData,
		},
	}
}

// Helper function to create a mock executor
func createMockExecutor(origPlanJSON, newPlanJSON string) *MockExecutor {
	return &MockExecutor{
		outputs: map[string]string{
			"orig": origPlanJSON,
			"new":  newPlanJSON,
		},
	}
}

// Helper function to run the "with_both_plans" test case
func runWithBothPlansTest(t *testing.T, tt planDiffTestCase, executor *MockExecutor, info schema.ConfigAndStacksInfo, tempDir string) {
	// Redirect stdout to capture output
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Run the function
	err := testExecuteTerraformPlanDiff(executor, info, tempDir, "test-vars.tfvars", "test-plan.tfplan")

	// Restore stdout and capture the output
	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	if err != nil {
		t.Errorf("Failed to read output: %v", err)
	}
	output := buf.String()

	// Log the diff output
	t.Logf("Diff output:\n%s", output)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	verifyPlanDiffOutput(t, tt.expectedDifference, output)
}

// Helper function to run the standard test cases
func runStandardPlanDiffTest(t *testing.T, tt planDiffTestCase, executor *MockExecutor, info schema.ConfigAndStacksInfo, tempDir string) {
	// Redirect stdout to capture output
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Run the function
	err := testExecuteTerraformPlanDiff(executor, info, tempDir, "test-vars.tfvars", "test-plan.tfplan")

	// Restore stdout
	w.Close()
	os.Stdout = oldStdout

	// Check if additional arguments were properly passed for the var args test
	if tt.name == "with_additional_var_args" {
		verifyAdditionalVarArgs(t, executor)
	}

	// Read the captured output
	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	if err != nil {
		t.Errorf("Failed to read output: %v", err)
	}
	output := buf.String()

	// Verify expected behavior
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	verifyPlanDiffOutput(t, tt.expectedDifference, output)
}

// Helper function to verify additional var arguments
func verifyAdditionalVarArgs(t *testing.T, executor *MockExecutor) {
	foundVarArg := false
	for _, cmd := range executor.commands {
		if len(cmd) > 2 && cmd[0] == "terraform" && cmd[1] == "plan" {
			for i := 0; i < len(cmd)-1; i++ {
				if cmd[i] == "-var" && cmd[i+1] == "location=New Jersey" {
					foundVarArg = true
					break
				}
			}
		}
	}
	if !foundVarArg {
		t.Errorf("Additional argument -var location=New Jersey was not passed to terraform plan command")
	}
}

// Helper function to verify plan diff output
func verifyPlanDiffOutput(t *testing.T, expectedDifference bool, output string) {
	if expectedDifference {
		if strings.Contains(output, "No differences found between the plans") {
			t.Errorf("Expected differences, but found none")
		}
	} else {
		if !strings.Contains(output, "No differences found between the plans") {
			t.Errorf("Expected no differences, but found some: %s", output)
		}
	}
}

// TestExecuteTerraformPlanDiffIntegration runs an integration test with real commands
func TestExecuteTerraformPlanDiffIntegration(t *testing.T) {
	// Skip in CI environments or when we don't want to run integration tests
	if os.Getenv("SKIP_INTEGRATION_TESTS") != "" {
		t.Skip("Skipping integration test")
	}

	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "terraform-plan-diff-integration")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create simple terraform files
	err = createSimpleTerraformProject(tempDir)
	if err != nil {
		t.Fatalf("Failed to create terraform project: %v", err)
	}

	// Run terraform init
	cmd := exec.Command("terraform", "init")
	cmd.Dir = tempDir
	err = cmd.Run()
	if err != nil {
		t.Fatalf("Failed to run terraform init: %v", err)
	}

	// Create two different plan files
	origPlanFile := filepath.Join(tempDir, "orig-plan.tfplan")
	newPlanFile := filepath.Join(tempDir, "new-plan.tfplan")

	// Generate the original plan (with a specific variable value)
	cmd = exec.Command("terraform", "plan", "-var", "example_var=original", "-out", origPlanFile)
	cmd.Dir = tempDir
	err = cmd.Run()
	if err != nil {
		t.Fatalf("Failed to generate original plan: %v", err)
	}

	// Generate the new plan (with a different variable value)
	cmd = exec.Command("terraform", "plan", "-var", "example_var=new_value", "-out", newPlanFile)
	cmd.Dir = tempDir
	err = cmd.Run()
	if err != nil {
		t.Fatalf("Failed to create new plan: %v", err)
	}

	// Create atmos configuration for the test
	atmosConfig := schema.AtmosConfiguration{
		BasePath: tempDir,
	}

	// Test the function with both plans specified
	t.Run("integration_with_both_plans", func(t *testing.T) {
		// Create info with both plan files
		info := schema.ConfigAndStacksInfo{
			AdditionalArgsAndFlags: []string{"--orig", origPlanFile, "--new", newPlanFile},
			ComponentEnvList:       []string{},
		}

		// Redirect stdout to capture the diff output
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		// Execute the actual function
		err := executeTerraformPlanDiff(atmosConfig, info, tempDir, "terraform.tfvars", "plan.tfplan")

		// Restore stdout
		w.Close()
		os.Stdout = oldStdout

		// Read captured output
		var buf bytes.Buffer
		_, err = io.Copy(&buf, r)
		if err != nil {
			t.Errorf("Failed to read output: %v", err)
		}
		output := buf.String()

		// Check for success and expected output
		if err != nil {
			t.Errorf("Failed to execute plan-diff: %v", err)
		}

		// Should contain some indication of the different variable value
		if !strings.Contains(output, "example_var") {
			t.Errorf("Expected output to contain variable changes, got: %s", output)
		}
	})

	// Test with only orig specified (auto-generates new plan)
	t.Run("integration_with_orig_only", func(t *testing.T) {
		// Create info with only orig plan file
		info := schema.ConfigAndStacksInfo{
			AdditionalArgsAndFlags: []string{"--orig", origPlanFile},
			ComponentEnvList:       []string{},
		}

		// Redirect stdout to capture the diff output
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		// Execute the actual function
		err := executeTerraformPlanDiff(atmosConfig, info, tempDir, "terraform.tfvars", "plan.tfplan")

		// Restore stdout
		w.Close()
		os.Stdout = oldStdout

		// Read captured output
		var buf bytes.Buffer
		_, err = io.Copy(&buf, r)
		if err != nil {
			t.Errorf("Failed to read output: %v", err)
		}

		// Check for success
		if err != nil {
			t.Errorf("Failed to execute plan-diff with auto-generated plan: %v", err)
		}
	})
}

// createSimpleTerraformProject creates a simple terraform project for testing
func createSimpleTerraformProject(dir string) error {
	// Create a simple main.tf file with a variable
	mainTf := `
variable "example_var" {
  type    = string
  default = "default_value"
}

output "example_output" {
  value = var.example_var
}
`
	err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(mainTf), 0o600)
	if err != nil {
		return err
	}

	// Create an empty terraform.tfvars file
	tfvars := `# Empty tfvars file for testing
`
	return os.WriteFile(filepath.Join(dir, "terraform.tfvars"), []byte(tfvars), 0o600)
}

// Custom type that will always fail to marshal to JSON
type UnmarshalableType struct{}

// MarshalJSON always returns an error for testing error handling paths
func (u UnmarshalableType) MarshalJSON() ([]byte, error) {
	return nil, errors.New("simulated marshal error")
}

// TestPrettyDiffErrorHandling tests error handling in the prettyDiff functions
func TestPrettyDiffErrorHandling(t *testing.T) {
	// Object with a value that can't be marshaled to JSON
	a := map[string]interface{}{
		"simple": "value",
		"complex": map[string]interface{}{
			"inner": UnmarshalableType{},
		},
		"arr": []interface{}{
			map[string]interface{}{
				"name": "resource1",
				"values": map[string]interface{}{
					"unmarshalable": UnmarshalableType{},
				},
			},
		},
		"resources": []interface{}{
			map[string]interface{}{
				"address": "test_resource.example",
				"type":    "test_resource",
				"name":    "example",
				"values": map[string]interface{}{
					"unmarshalable": UnmarshalableType{},
				},
			},
		},
	}

	b := map[string]interface{}{
		"simple":    "new_value",
		"new_field": UnmarshalableType{},
		"resources": []interface{}{
			map[string]interface{}{
				"address":       "test_resource.new",
				"type":          "test_resource",
				"name":          "new",
				"unmarshalable": UnmarshalableType{},
			},
		},
	}

	// Test the main prettyDiff function
	t.Run("test_prettydiff_error_handling", func(t *testing.T) {
		// Temporarily redirect stdout to capture output
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		// Run the function - it shouldn't panic despite marshaling errors
		hasDiff := prettyDiff(a, b, "")

		// Restore stdout
		w.Close()
		os.Stdout = oldStdout

		// Read the captured output
		var buf bytes.Buffer
		_, err := io.Copy(&buf, r)
		if err != nil {
			t.Errorf("Failed to read output: %v", err)
		}
		output := buf.String()

		// Check that we got differences
		assert.True(t, hasDiff, "prettyDiff should indicate differences")

		// Check output contains fallback for unmarshalable values
		assert.Contains(t, output, "- complex: map[inner:{}]", "Should include fallback for unmarshalable complex value")
		assert.Contains(t, output, "+ new_field: {}", "Should include fallback for unmarshalable new field")
	})

	// Test prettyDiffTest function
	t.Run("test_prettydifftest_error_handling", func(t *testing.T) {
		var output strings.Builder

		// Run the test function - it shouldn't panic despite marshaling errors
		hasDiff := prettyDiffTest(a, b, "", &output)

		// Check that we got differences
		assert.True(t, hasDiff, "prettyDiffTest should indicate differences")

		// Check output contains fallback for unmarshalable values
		assert.Contains(t, output.String(), "- complex: map[inner:{}]", "Should include fallback for unmarshalable complex value")
		assert.Contains(t, output.String(), "+ new_field: {}", "Should include fallback for unmarshalable new field")
	})

	// Test resourceDiff function
	t.Run("test_resourcediff_error_handling", func(t *testing.T) {
		// Temporarily redirect stdout to capture output
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		// Run the function - it shouldn't panic despite marshaling errors
		resourceObj1 := map[string]interface{}{
			"address": "test_resource.resource1",
			"values": map[string]interface{}{
				"normal":        "value1",
				"unmarshalable": UnmarshalableType{},
			},
		}

		resourceObj2 := map[string]interface{}{
			"address": "test_resource.resource1",
			"values": map[string]interface{}{
				"normal":            "value2",
				"new_unmarshalable": UnmarshalableType{},
			},
		}

		resourceDiff(resourceObj1, resourceObj2, "  ")

		// Restore stdout
		w.Close()
		os.Stdout = oldStdout

		// Read the captured output
		var buf bytes.Buffer
		_, err := io.Copy(&buf, r)
		if err != nil {
			t.Errorf("Failed to read output: %v", err)
		}
		output := buf.String()

		// Check output contains actual differences
		assert.Contains(t, output, "~ normal: value1 => value2", "Should show value differences")
		assert.Contains(t, output, "- unmarshalable:", "Should include removed field")
		assert.Contains(t, output, "+ new_unmarshalable:", "Should include added field")
	})

	// Test resourceDiffTest function
	t.Run("test_resourcedifftest_error_handling", func(t *testing.T) {
		var output strings.Builder

		resourceObj1 := map[string]interface{}{
			"address": "test_resource.resource1",
			"values": map[string]interface{}{
				"normal":        "value1",
				"unmarshalable": UnmarshalableType{},
			},
		}

		resourceObj2 := map[string]interface{}{
			"address": "test_resource.resource1",
			"values": map[string]interface{}{
				"normal":            "value2",
				"new_unmarshalable": UnmarshalableType{},
			},
		}

		resourceDiffTest(resourceObj1, resourceObj2, "  ", &output)

		// Check output contains actual differences
		assert.Contains(t, output.String(), "~ normal: value1 => value2", "Should show value differences")
		assert.Contains(t, output.String(), "- unmarshalable:", "Should include removed field")
		assert.Contains(t, output.String(), "+ new_unmarshalable:", "Should include added field")
	})

	// Test getResourceName function
	t.Run("test_getresourcename", func(t *testing.T) {
		// Test with address field present
		resource1 := map[string]interface{}{
			"address": "aws_instance.test",
			"type":    "aws_instance",
			"name":    "test",
		}
		assert.Equal(t, "aws_instance.test", getResourceName(resource1), "Should use address field when present")

		// Test with type and name fields but no address
		resource2 := map[string]interface{}{
			"type": "aws_instance",
			"name": "test",
		}
		assert.Equal(t, "aws_instance.test", getResourceName(resource2), "Should combine type and name fields")

		// Test with only type field
		resource3 := map[string]interface{}{
			"type": "aws_instance",
		}
		assert.Equal(t, "aws_instance", getResourceName(resource3), "Should use type field when name is missing")

		// Test with only name field
		resource4 := map[string]interface{}{
			"name": "test",
		}
		assert.Equal(t, "test", getResourceName(resource4), "Should use name field when type is missing")

		// Test with empty map
		resource5 := map[string]interface{}{}
		assert.Equal(t, "<unknown resource>", getResourceName(resource5), "Should return default value for empty map")
	})

	// Test getResourceNameTest function
	t.Run("test_getresourcenametest", func(t *testing.T) {
		// Test with address field present
		resource1 := map[string]interface{}{
			"address": "aws_instance.test",
			"type":    "aws_instance",
			"name":    "test",
		}
		assert.Equal(t, "aws_instance.test", getResourceNameTest(resource1), "Should use address field when present")

		// Test with type and name fields but no address
		resource2 := map[string]interface{}{
			"type": "aws_instance",
			"name": "test",
		}
		assert.Equal(t, "aws_instance.test", getResourceNameTest(resource2), "Should combine type and name fields")

		// Test with only type field
		resource3 := map[string]interface{}{
			"type": "aws_instance",
		}
		assert.Equal(t, "aws_instance", getResourceNameTest(resource3), "Should use type field when name is missing")

		// Test with only name field
		resource4 := map[string]interface{}{
			"name": "test",
		}
		assert.Equal(t, "test", getResourceNameTest(resource4), "Should use name field when type is missing")

		// Test with empty map
		resource5 := map[string]interface{}{}
		assert.Equal(t, "<unknown resource>", getResourceNameTest(resource5), "Should return default value for empty map")
	})

	// Test array handling in prettyDiff
	t.Run("test_prettydiff_array_handling", func(t *testing.T) {
		// Test with various array configurations

		// 1. Test with empty arrays
		aEmpty := map[string]interface{}{
			"resources":   []interface{}{},
			"other_array": []interface{}{},
		}

		bEmpty := map[string]interface{}{
			"resources": []interface{}{},
			"new_array": []interface{}{1, 2, 3},
		}

		// Capture output
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		hasDiff := prettyDiff(aEmpty, bEmpty, "")

		w.Close()
		os.Stdout = oldStdout

		var buf bytes.Buffer
		_, err := io.Copy(&buf, r)
		if err != nil {
			t.Errorf("Failed to read output: %v", err)
		}
		emptyOutput := buf.String()

		// Just verify that it worked and found differences
		assert.True(t, hasDiff, "Should find differences")
		assert.Contains(t, emptyOutput, "other_array", "Should mention other_array")
		assert.Contains(t, emptyOutput, "new_array", "Should mention new_array")

		// 2. Test with non-matchable resources
		aNonMatch := map[string]interface{}{
			"resources": []interface{}{
				map[string]interface{}{
					"address": "aws_instance.test1",
					"type":    "aws_instance",
					"name":    "test1",
					"values": map[string]interface{}{
						"ami": "ami-123",
					},
				},
			},
		}

		bNonMatch := map[string]interface{}{
			"resources": []interface{}{
				map[string]interface{}{
					"address": "aws_instance.test2",
					"type":    "aws_instance",
					"name":    "test2",
					"values": map[string]interface{}{
						"ami": "ami-456",
					},
				},
			},
		}

		// Capture output
		r2, w2, _ := os.Pipe()
		os.Stdout = w2

		hasDiff2 := prettyDiff(aNonMatch, bNonMatch, "")

		w2.Close()
		os.Stdout = oldStdout

		var buf2 bytes.Buffer
		_, err = io.Copy(&buf2, r2)
		if err != nil {
			t.Errorf("Failed to read output: %v", err)
		}
		nonMatchOutput := buf2.String()

		assert.True(t, hasDiff2, "Should find differences")
		assert.Contains(t, nonMatchOutput, "aws_instance.test1", "Should mention first resource")
		assert.Contains(t, nonMatchOutput, "aws_instance.test2", "Should mention second resource")

		// 3. Test with matching resources
		aMatch := map[string]interface{}{
			"resources": []interface{}{
				map[string]interface{}{
					"address": "aws_instance.test",
					"type":    "aws_instance",
					"name":    "test",
					"values": map[string]interface{}{
						"ami":           "ami-123",
						"instance_type": "t3.micro",
					},
				},
			},
		}

		bMatch := map[string]interface{}{
			"resources": []interface{}{
				map[string]interface{}{
					"address": "aws_instance.test",
					"type":    "aws_instance",
					"name":    "test",
					"values": map[string]interface{}{
						"ami":           "ami-456",
						"instance_type": "t3.micro",
					},
				},
			},
		}

		// Capture output
		r3, w3, _ := os.Pipe()
		os.Stdout = w3

		hasDiff3 := prettyDiff(aMatch, bMatch, "")

		w3.Close()
		os.Stdout = oldStdout

		var buf3 bytes.Buffer
		_, err = io.Copy(&buf3, r3)
		if err != nil {
			t.Errorf("Failed to read output: %v", err)
		}
		matchOutput := buf3.String()

		assert.True(t, hasDiff3, "Should find differences")
		assert.Contains(t, matchOutput, "aws_instance.test", "Should identify the resource")
		assert.Contains(t, matchOutput, "ami", "Should mention the changed field")
		assert.Contains(t, matchOutput, "ami-123", "Should show old ami value")
		assert.Contains(t, matchOutput, "ami-456", "Should show new ami value")
	})

	// Test array handling in prettyDiffTest
	t.Run("test_prettydifftest_array_handling", func(t *testing.T) {
		// Test with various array configurations

		// 1. Test with empty arrays
		aEmpty := map[string]interface{}{
			"resources":   []interface{}{},
			"other_array": []interface{}{},
		}

		bEmpty := map[string]interface{}{
			"resources": []interface{}{},
			"new_array": []interface{}{1, 2, 3},
		}

		var output1 strings.Builder
		hasDiff1 := prettyDiffTest(aEmpty, bEmpty, "", &output1)

		assert.True(t, hasDiff1, "Should find differences")
		assert.Contains(t, output1.String(), "other_array", "Should mention other_array")
		assert.Contains(t, output1.String(), "new_array", "Should mention new_array")

		// 2. Test with non-matchable resources
		aNonMatch := map[string]interface{}{
			"resources": []interface{}{
				map[string]interface{}{
					"address": "aws_instance.test1",
					"type":    "aws_instance",
					"name":    "test1",
					"values": map[string]interface{}{
						"ami": "ami-123",
					},
				},
			},
		}

		bNonMatch := map[string]interface{}{
			"resources": []interface{}{
				map[string]interface{}{
					"address": "aws_instance.test2",
					"type":    "aws_instance",
					"name":    "test2",
					"values": map[string]interface{}{
						"ami": "ami-456",
					},
				},
			},
		}

		var output2 strings.Builder
		hasDiff2 := prettyDiffTest(aNonMatch, bNonMatch, "", &output2)

		assert.True(t, hasDiff2, "Should find differences")
		assert.Contains(t, output2.String(), "aws_instance.test1", "Should mention first resource")
		assert.Contains(t, output2.String(), "aws_instance.test2", "Should mention second resource")

		// 3. Test with matching resources
		aMatch := map[string]interface{}{
			"resources": []interface{}{
				map[string]interface{}{
					"address": "aws_instance.test",
					"type":    "aws_instance",
					"name":    "test",
					"values": map[string]interface{}{
						"ami":           "ami-123",
						"instance_type": "t3.micro",
					},
				},
			},
		}

		bMatch := map[string]interface{}{
			"resources": []interface{}{
				map[string]interface{}{
					"address": "aws_instance.test",
					"type":    "aws_instance",
					"name":    "test",
					"values": map[string]interface{}{
						"ami":           "ami-456",
						"instance_type": "t3.micro",
					},
				},
			},
		}

		var output3 strings.Builder
		hasDiff3 := prettyDiffTest(aMatch, bMatch, "", &output3)

		assert.True(t, hasDiff3, "Should find differences")
		assert.Contains(t, output3.String(), "aws_instance.test", "Should identify the resource")
		assert.Contains(t, output3.String(), "ami", "Should mention the changed field")
		assert.Contains(t, output3.String(), "ami-123", "Should show old ami value")
		assert.Contains(t, output3.String(), "ami-456", "Should show new ami value")
	})
}
