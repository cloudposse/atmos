package exec

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
)

const (
	// Define constants for testing.
	testVarFileFlag = "-var-file"
	testOutFlag     = "-out"
)

// Helper function to create bool pointer for testing.
func diffBoolPtr(b bool) *bool {
	return &b
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

// MockExecutor is a mock implementation of shell command execution.
type MockExecutor struct {
	commands [][]string
	outputs  map[string]string
}

// ExecuteCommand records the command and simulates the behavior we need for testing.
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

// prettyDiffTest is a version of prettyDiff that captures output for testing.
func prettyDiffTest(a, b map[string]interface{}, path string, output *strings.Builder) bool {
	hasDifferences := false

	// Compare keys in map a to map b
	hasDifferences = compareMapAtoBTest(a, b, path, output) || hasDifferences

	// Compare keys in map b to map a (for keys only in b)
	hasDifferences = compareMapBtoATest(a, b, path, output) || hasDifferences

	return hasDifferences
}

// Helper function to compare keys from map a to map b for testing.
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

// Helper function to compare keys from map b to map a for testing.
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

// Helper function to build the path string for testing.
func buildPathTest(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

// Helper function to print a value that was removed for testing.
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

// Helper function to print a value that was added for testing.
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

// Helper function to print a type difference for testing.
func printTypeDifferenceTest(path string, v1, v2 interface{}, output *strings.Builder) {
	fmt.Fprintf(output, "~ %s:\n", path)
	fmt.Fprintf(output, "  - %v\n", v1)
	fmt.Fprintf(output, "  + %v\n", v2)
}

// Helper function to diff arrays for testing.
func diffArraysTest(path string, arr1, arr2 []interface{}, output *strings.Builder) bool {
	// For terraform plans, resources arrays are especially important to show clearly
	if path == "resources" || strings.HasSuffix(path, ".resources") {
		return diffResourceArraysTest(path, arr1, arr2, output)
	} else {
		return diffGenericArraysTest(path, arr1, arr2, output)
	}
}

// Helper function to diff resource arrays for testing.
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

// Helper function to process resources that were removed or changed for testing.
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

// Helper function to find a matching resource in the array for testing.
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

// Helper function to process resources that were added for testing.
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

// Helper function to print a removed resource for testing.
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

// Helper function to print an added resource for testing.
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

// Helper function to diff generic (non-resource) arrays for testing.
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

// Helper function to get a readable resource name for the test.
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

// Helper function to diff individual resources for the test.
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

// Helper function to diff resource values for testing.
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

// Helper function for resource diff fallback method for testing.
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

// testExecuteTerraformPlanDiff is a testable version of executeTerraformPlanDiff that uses the MockExecutor.
func testExecuteTerraformPlanDiff(executor *MockExecutor, info *schema.ConfigAndStacksInfo, componentPath, varFile, planFile string) error {
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

	// Run each test
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create executor with mock data
			executor := createMockExecutor(tt.origPlanJSON, tt.newPlanJSON)

			// Create info with additional args
			info := schema.ConfigAndStacksInfo{
				AdditionalArgsAndFlags: tt.additionalArgs,
			}

			// If the test includes both orig and new flags, run with both plans
			if strings.Contains(tt.name, "both plans") {
				runWithBothPlansTest(t, tt, executor, &info, tempDir)
			} else {
				runStandardPlanDiffTest(t, tt, executor, &info, tempDir)
			}
		})
	}
}

// setupTestPlanDiffEnvironment creates a temporary directory for testing with necessary files.
func setupTestPlanDiffEnvironment(t *testing.T) (string, string, string) {
	// Create a temporary directory for tests
	tempDir, err := os.MkdirTemp("", "tf-plan-diff-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Create mock plan files
	origPlanFile := filepath.Join(tempDir, "orig.tfplan")
	err = os.WriteFile(origPlanFile, []byte("mock plan content"), 0o600)
	if err != nil {
		t.Fatalf("Failed to create orig plan file: %v", err)
	}

	newPlanFile := filepath.Join(tempDir, "new.tfplan")
	err = os.WriteFile(newPlanFile, []byte("mock plan content"), 0o600)
	if err != nil {
		t.Fatalf("Failed to create new plan file: %v", err)
	}

	return tempDir, origPlanFile, newPlanFile
}

// getTestPlanData returns test data for original and new plans.
func getTestPlanData() (string, string) {
	// Original plan JSON
	origPlanJSON := `{
		"format_version": "1.0",
		"terraform_version": "1.0.0",
		"prior_state": {
			"values": {
				"root_module": {
					"resources": [
						{
							"address": "aws_instance.example",
							"type": "aws_instance",
							"name": "example",
							"provider_name": "registry.terraform.io/hashicorp/aws",
							"values": {
								"ami": "ami-123456",
								"instance_type": "t2.micro",
								"tags": {
									"Name": "example-instance"
								}
							}
						}
					]
				}
			}
		},
		"planned_values": {
			"root_module": {
				"resources": [
					{
						"address": "aws_instance.example",
						"type": "aws_instance",
						"name": "example",
						"provider_name": "registry.terraform.io/hashicorp/aws",
						"values": {
							"ami": "ami-123456",
							"instance_type": "t2.micro",
							"tags": {
								"Name": "example-instance"
							}
						}
					}
				]
			}
		}
	}`

	// New plan JSON with a different instance type
	newPlanJSON := `{
		"format_version": "1.0",
		"terraform_version": "1.0.0",
		"prior_state": {
			"values": {
				"root_module": {
					"resources": [
						{
							"address": "aws_instance.example",
							"type": "aws_instance",
							"name": "example",
							"provider_name": "registry.terraform.io/hashicorp/aws",
							"values": {
								"ami": "ami-123456",
								"instance_type": "t2.micro",
								"tags": {
									"Name": "example-instance"
								}
							}
						}
					]
				}
			}
		},
		"planned_values": {
			"root_module": {
				"resources": [
					{
						"address": "aws_instance.example",
						"type": "aws_instance",
						"name": "example",
						"provider_name": "registry.terraform.io/hashicorp/aws",
						"values": {
							"ami": "ami-123456",
							"instance_type": "t2.large",
							"tags": {
								"Name": "example-instance",
								"Environment": "production"
							}
						}
					},
					{
						"address": "aws_s3_bucket.logs",
						"type": "aws_s3_bucket",
						"name": "logs",
						"provider_name": "registry.terraform.io/hashicorp/aws",
						"values": {
							"bucket": "example-logs",
							"acl": "private"
						}
					}
				]
			}
		}
	}`

	return origPlanJSON, newPlanJSON
}

// planDiffTestCase represents a test case for plan diff functionality.
type planDiffTestCase struct {
	name               string
	additionalArgs     []string
	expectedDifference bool
	origPlanJSON       string
	newPlanJSON        string
}

// createPlanDiffTestCases creates test cases for the plan diff function.
func createPlanDiffTestCases(origPlanFile, newPlanFile, origPlanData, newPlanData string) []planDiffTestCase {
	return []planDiffTestCase{
		{
			name:               "basic diff with generated new plan",
			additionalArgs:     []string{"--orig", origPlanFile},
			expectedDifference: true,
			origPlanJSON:       origPlanData,
			newPlanJSON:        newPlanData,
		},
		{
			name:               "using both plans",
			additionalArgs:     []string{"--orig", origPlanFile, "--new", newPlanFile},
			expectedDifference: true,
			origPlanJSON:       origPlanData,
			newPlanJSON:        newPlanData,
		},
		{
			name:               "identical plans",
			additionalArgs:     []string{"--orig", origPlanFile},
			expectedDifference: false,
			origPlanJSON:       origPlanData,
			newPlanJSON:        origPlanData, // Same content for both plans
		},
		{
			name:               "with additional terraform arguments",
			additionalArgs:     []string{"--orig", origPlanFile, "-target=aws_instance.example"},
			expectedDifference: true,
			origPlanJSON:       origPlanData,
			newPlanJSON:        newPlanData,
		},
	}
}

// createMockExecutor creates a mock executor with the specified plan JSON data.
func createMockExecutor(origPlanJSON, newPlanJSON string) *MockExecutor {
	return &MockExecutor{
		commands: make([][]string, 0),
		outputs: map[string]string{
			"orig": origPlanJSON,
			"new":  newPlanJSON,
		},
	}
}

// runWithBothPlansTest runs a test with both original and new plan files specified.
func runWithBothPlansTest(t *testing.T, tt planDiffTestCase, executor *MockExecutor, info *schema.ConfigAndStacksInfo, tempDir string) {
	// Redirect stdout to capture output
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Run testable version of the plan diff function
	err := testExecuteTerraformPlanDiff(executor, info, tempDir, "vars.tfvars", "orig.tfplan")

	// Restore stdout and capture output
	w.Close()
	os.Stdout = oldStdout
	var output bytes.Buffer
	io.Copy(&output, r)

	// Verify
	assert.NoError(t, err)

	// Check that both plans were properly processed (no terraform plan command)
	planCmdCount := 0
	for _, cmd := range executor.commands {
		if len(cmd) > 1 && cmd[0] == "terraform" && cmd[1] == "plan" {
			planCmdCount++
		}
	}
	assert.Equal(t, 0, planCmdCount, "No terraform plan command should be executed when --new is provided")

	// Verify the output is as expected
	verifyPlanDiffOutput(t, output.String(), tt.expectedDifference)
}

// runStandardPlanDiffTest runs a test with only the original plan specified.
func runStandardPlanDiffTest(t *testing.T, tt planDiffTestCase, executor *MockExecutor, info *schema.ConfigAndStacksInfo, tempDir string) {
	// Redirect stdout to capture output
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Run testable version of the plan diff function
	err := testExecuteTerraformPlanDiff(executor, info, tempDir, "vars.tfvars", "orig.tfplan")

	// Restore stdout and capture output
	w.Close()
	os.Stdout = oldStdout
	var output bytes.Buffer
	io.Copy(&output, r)

	// Verify
	assert.NoError(t, err)

	// Check that terraform plan was executed
	planCmdCount := 0
	for _, cmd := range executor.commands {
		if len(cmd) > 1 && cmd[0] == "terraform" && cmd[1] == "plan" {
			planCmdCount++
			// If extra args were passed, check they were included
			if tt.name == "with additional terraform arguments" {
				verifyAdditionalVarArgs(t, executor, tt.additionalArgs)
			}
		}
	}
	assert.Equal(t, 1, planCmdCount, "Terraform plan command should be executed exactly once")

	// Verify the output is as expected
	verifyPlanDiffOutput(t, output.String(), tt.expectedDifference)
}

// verifyAdditionalVarArgs verifies that additional arguments were properly passed to terraform.
func verifyAdditionalVarArgs(t *testing.T, executor *MockExecutor, expectedArgs []string) {
	for _, cmd := range executor.commands {
		if len(cmd) > 1 && cmd[0] == "terraform" && cmd[1] == "plan" {
			found := false
			for _, arg := range cmd {
				for _, expectedArg := range expectedArgs {
					if arg == expectedArg {
						found = true
						break
					}
				}
			}
			assert.True(t, found, "Additional arguments should be passed to terraform plan")
		}
	}
}

// verifyPlanDiffOutput checks the output contains expected diff results.
func verifyPlanDiffOutput(t *testing.T, output string, expectedDifference bool) {
	if expectedDifference {
		assert.NotContains(t, output, "No differences found between the plans")
	} else {
		assert.Contains(t, output, "No differences found between the plans")
	}
}

type UnmarshalableType struct{}

// MarshalJSON returns an error when trying to marshal this type.
func (u UnmarshalableType) MarshalJSON() ([]byte, error) {
	return nil, errors.New("cannot marshal this type")
}

func TestPrettyDiffErrorHandling(t *testing.T) {
	// Test handling of marshaling errors
	t.Run("marshalError", func(t *testing.T) {
		var output strings.Builder
		v1 := map[string]interface{}{"test": UnmarshalableType{}}
		v2 := map[string]interface{}{}

		// Redirect stdout to capture output
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		// Call function
		prettyDiffTest(v1, v2, "", &output)

		// Restore stdout
		w.Close()
		os.Stdout = oldStdout
		var capturedOutput bytes.Buffer
		io.Copy(&capturedOutput, r)

		// The function should handle the error gracefully
		assert.Contains(t, output.String(), "- test:", "Should indicate removed item")
	})

	// Test comparison of different types
	t.Run("differentTypes", func(t *testing.T) {
		var output strings.Builder
		v1 := map[string]interface{}{"test": "string"}
		v2 := map[string]interface{}{"test": 123}

		// Should handle different types correctly
		prettyDiffTest(v1, v2, "", &output)

		assert.Contains(t, output.String(), "~ test:", "Should indicate type difference")
	})
}
