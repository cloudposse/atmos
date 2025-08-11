package initcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestNewInitCmd(t *testing.T) {
	cmd := NewInitCmd()

	if cmd.Use != "init [configuration] [target path]" {
		t.Errorf("Expected Use to be 'init [configuration] [target path]', got %s", cmd.Use)
	}

	if cmd.Short != "Initialize configurations and examples" {
		t.Errorf("Expected Short to be 'Initialize configurations and examples', got %s", cmd.Short)
	}

	// Check that flags exist
	forceFlag := cmd.Flags().Lookup("force")
	if forceFlag == nil {
		t.Error("Expected --force flag to exist")
	}

	updateFlag := cmd.Flags().Lookup("update")
	if updateFlag == nil {
		t.Error("Expected --update flag to exist")
	}

	valuesFlag := cmd.Flags().Lookup("values")
	if valuesFlag == nil {
		t.Error("Expected --values flag to exist")
	}

	if valuesFlag.Shorthand != "V" {
		t.Errorf("Expected --values shorthand to be 'V', got %s", valuesFlag.Shorthand)
	}
}

func TestExecuteInit_ValidArgs(t *testing.T) {
	cmd := &cobra.Command{}
	tempDir := t.TempDir()
	args := []string{"atmos.yaml", tempDir}

	err := executeInit(cmd, args, false, false, true, []string{"author=Test User", "year=2024", "license=MIT"}, 0)
	if err != nil {
		t.Fatalf("Expected no error for valid args, got: %v", err)
	}

	// Check that atmos.yaml was created
	atmosPath := filepath.Join(tempDir, "atmos.yaml")
	if _, err := os.Stat(atmosPath); os.IsNotExist(err) {
		t.Error("Expected atmos.yaml to be created")
	}
}

func TestExecuteInit_InvalidConfig(t *testing.T) {
	cmd := &cobra.Command{}
	tempDir := t.TempDir()
	args := []string{"nonexistent", tempDir}

	err := executeInit(cmd, args, false, false, true, []string{"author=Test User", "year=2024", "license=MIT"}, 0)
	if err == nil {
		t.Fatal("Expected error for invalid config")
	}

	if !contains(err.Error(), "configuration 'nonexistent' not found") {
		t.Errorf("Expected error about configuration not found, got: %v", err)
	}
}

func TestExecuteInit_RelativePath(t *testing.T) {
	cmd := &cobra.Command{}
	args := []string{"atmos.yaml", "./test"}

	err := executeInit(cmd, args, false, false, true, []string{"author=Test User", "year=2024", "license=MIT"}, 0)
	if err != nil {
		t.Fatalf("Expected no error for relative path, got: %v", err)
	}

	// Clean up
	os.RemoveAll("./test")
}

func TestExecuteInit_AbsolutePath(t *testing.T) {
	cmd := &cobra.Command{}
	tempDir := t.TempDir()
	args := []string{"atmos.yaml", tempDir}

	err := executeInit(cmd, args, false, false, true, []string{"author=Test User", "year=2024", "license=MIT"}, 0)
	if err != nil {
		t.Fatalf("Expected no error for absolute path, got: %v", err)
	}
}

func TestExecuteInit_ForceFlag(t *testing.T) {
	cmd := &cobra.Command{}
	tempDir := t.TempDir()
	args := []string{"atmos.yaml", tempDir}

	// First run
	err := executeInit(cmd, args, false, false, true, []string{"author=Test User", "year=2024", "license=MIT"}, 0)
	if err != nil {
		t.Fatalf("Expected no error for first run, got: %v", err)
	}

	// Second run with force
	err = executeInit(cmd, args, true, false, true, []string{"author=Test User", "year=2024", "license=MIT"}, 0)
	if err != nil {
		t.Fatalf("Expected no error with force flag, got: %v", err)
	}
}

func TestExecuteInit_UpdateFlag(t *testing.T) {
	cmd := &cobra.Command{}
	tempDir := t.TempDir()
	args := []string{"atmos.yaml", tempDir}

	// First run
	err := executeInit(cmd, args, false, false, true, []string{"author=Test User", "year=2024", "license=MIT"}, 0)
	if err != nil {
		t.Fatalf("Expected no error for first run, got: %v", err)
	}

	// Second run with update
	err = executeInit(cmd, args, false, true, true, []string{"author=Test User", "year=2024", "license=MIT"}, 0)
	if err != nil {
		t.Fatalf("Expected no error with update flag, got: %v", err)
	}
}

func TestExecuteInit_DefaultConfig(t *testing.T) {
	cmd := &cobra.Command{}
	tempDir := t.TempDir()
	args := []string{"default", tempDir}

	err := executeInit(cmd, args, false, false, true, []string{"author=Test User", "year=2024", "license=MIT"}, 0)
	if err != nil {
		t.Fatalf("Expected no error for default config, got: %v", err)
	}

	// Check that README.md was created (default config only contains README.md)
	expectedFiles := []string{"README.md"}
	for _, file := range expectedFiles {
		filePath := filepath.Join(tempDir, file)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			t.Errorf("Expected file %s to be created", file)
		}
	}
}

func TestExecuteInit_DemoConfig(t *testing.T) {
	cmd := &cobra.Command{}
	tempDir := t.TempDir()
	args := []string{"examples/demo-stacks", tempDir}

	err := executeInit(cmd, args, false, false, true, []string{"author=Test User", "year=2024", "license=MIT"}, 0)
	if err != nil {
		t.Fatalf("Expected no error for demo config, got: %v", err)
	}

	// Check that demo files were created
	expectedFiles := []string{"stacks/orgs/cp/tenant/terraform/dev/us-east-2.yaml", "components/terraform/vpc/main.tf", "README.md"}
	for _, file := range expectedFiles {
		filePath := filepath.Join(tempDir, file)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			t.Errorf("Expected file %s to be created", file)
		}
	}
}

func TestGenerateHelpText(t *testing.T) {
	helpText := generateHelpText()

	if helpText == "" {
		t.Error("Expected help text to be non-empty")
	}

	// Check for expected content
	expectedSections := []string{
		"Initialize a typical project for atmos",
		"Initialize a local atmos CLI configuration file",
		"Initialize a local Editor Config file",
		"Initialize a recommend Git ignore file",
		"Demonstration of using Atmos stacks",
		"Demonstration of using Atmos with localstack",
		"Demonstration of using Atmos with Helmfile",
		"Force overwrite existing files",
		"Update existing files with 3-way merge",
	}

	for _, section := range expectedSections {
		if !contains(helpText, section) {
			t.Errorf("Expected help text to contain: %s", section)
		}
	}
}

func TestGenerateHelpText_Examples(t *testing.T) {
	helpText := generateHelpText()

	// Check for expected examples
	expectedExamples := []string{
		"$ atmos init default",
		"$ atmos init atmos.yaml",
		"$ atmos init examples/demo-localstack",
		"$ atmos init default --force",
		"$ atmos init default --update",
	}

	for _, example := range expectedExamples {
		if !contains(helpText, example) {
			t.Errorf("Expected help text to contain example: %s", example)
		}
	}
}

func TestExecuteInit_PathTemplating(t *testing.T) {
	cmd := &cobra.Command{}
	tempDir := t.TempDir()
	args := []string{"path-test", tempDir}

	templateValues := []string{
		"namespace=production",
		"author=test-user",
		"description=Integration test for path templating",
	}

	err := executeInit(cmd, args, false, false, true, templateValues, 0)
	if err != nil {
		t.Fatalf("Expected no error for path-test config, got: %v", err)
	}

	// Check that files were created with templated paths
	expectedFiles := []string{
		"production/config.yaml",
		"production/docs/README.md",
		"production-monitoring.yaml",
	}

	for _, file := range expectedFiles {
		filePath := filepath.Join(tempDir, file)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			t.Errorf("Expected file %s to be created", file)
		}
	}
}

// Helper function
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

// Tests for --values functionality
func TestParseTemplateValues_ValidValues(t *testing.T) {
	testCases := []struct {
		name     string
		values   []string
		expected map[string]interface{}
	}{
		{
			name:   "string values",
			values: []string{"key1=value1", "key2=value2"},
			expected: map[string]interface{}{
				"key1": "value1",
				"key2": "value2",
			},
		},
		{
			name:   "boolean values",
			values: []string{"enabled=true", "disabled=false", "yes=yes", "no=no", "one=1", "zero=0"},
			expected: map[string]interface{}{
				"enabled":  true,
				"disabled": false,
				"yes":      true,
				"no":       false,
				"one":      true,
				"zero":     false,
			},
		},
		{
			name:   "numeric values",
			values: []string{"int=42", "float=3.14", "negative=-10"},
			expected: map[string]interface{}{
				"int":      42,
				"float":    3.14,
				"negative": -10,
			},
		},
		{
			name:   "mixed types",
			values: []string{"name=John", "age=30", "active=true", "score=95.5"},
			expected: map[string]interface{}{
				"name":   "John",
				"age":    30,
				"active": true,
				"score":  95.5,
			},
		},
		{
			name:     "empty values",
			values:   []string{},
			expected: map[string]interface{}{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := parseTemplateValues(tc.values)
			if err != nil {
				t.Fatalf("Expected no error, got: %v", err)
			}

			if len(result) != len(tc.expected) {
				t.Errorf("Expected %d values, got %d", len(tc.expected), len(result))
			}

			for key, expectedValue := range tc.expected {
				if result[key] != expectedValue {
					t.Errorf("Expected %s=%v, got %v", key, expectedValue, result[key])
				}
			}
		})
	}
}

func TestParseTemplateValues_InvalidValues(t *testing.T) {
	testCases := []struct {
		name        string
		values      []string
		expectedErr string
	}{
		{
			name:        "missing equals sign",
			values:      []string{"key1=value1", "key2value2"},
			expectedErr: "invalid template value format: key2value2 (expected key=value)",
		},
		{
			name:        "empty key",
			values:      []string{"=value1", "key2=value2"},
			expectedErr: "empty key in template value: =value1",
		},
		{
			name:        "whitespace in key",
			values:      []string{" key1=value1"},
			expectedErr: "", // Whitespace is trimmed, so this should succeed
		},
		{
			name:        "multiple equals signs",
			values:      []string{"key1=value1=extra"},
			expectedErr: "invalid template value format: key1=value1=extra (expected key=value)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseTemplateValues(tc.values)
			if tc.expectedErr == "" {
				// Expect success
				if err != nil {
					t.Errorf("Expected no error, got: %v", err)
				}
			} else {
				// Expect error
				if err == nil {
					t.Fatal("Expected error, got nil")
				}

				if !contains(err.Error(), tc.expectedErr) {
					t.Errorf("Expected error to contain '%s', got: %v", tc.expectedErr, err)
				}
			}
		})
	}
}

func TestExecuteInit_WithValues(t *testing.T) {
	cmd := &cobra.Command{}
	tempDir := t.TempDir()
	args := []string{"atmos.yaml", tempDir}
	templateValues := []string{"author=John", "year=2024", "license=MIT"}

	err := executeInit(cmd, args, false, false, true, templateValues, 0)
	if err != nil {
		t.Fatalf("Expected no error for valid template values, got: %v", err)
	}

	// Check that atmos.yaml was created
	atmosPath := filepath.Join(tempDir, "atmos.yaml")
	if _, err := os.Stat(atmosPath); os.IsNotExist(err) {
		t.Error("Expected atmos.yaml to be created")
	}
}

func TestExecuteInit_WithInvalidValues(t *testing.T) {
	cmd := &cobra.Command{}
	tempDir := t.TempDir()
	args := []string{"atmos.yaml", tempDir}
	invalidValues := []string{"author=John", "invalid-format"}

	err := executeInit(cmd, args, false, false, true, invalidValues, 0)
	if err == nil {
		t.Fatal("Expected error for invalid template values")
	}

	if !contains(err.Error(), "failed to parse template values") {
		t.Errorf("Expected error about parsing template values, got: %v", err)
	}
}

func TestExecuteInit_WithProjectValues(t *testing.T) {
	cmd := &cobra.Command{}
	tempDir := t.TempDir()
	args := []string{"rich-project", tempDir}
	templateValues := []string{
		"project_name=test-project",
		"author=John Doe",
		"year=2024",
		"license=MIT",
		"cloud_provider=aws",
		"enable_monitoring=true",
	}

	err := executeInit(cmd, args, false, false, true, templateValues, 0)
	if err != nil {
		t.Fatalf("Expected no error for project with template values, got: %v", err)
	}

	// Check that the project was created with the template values
	atmosPath := filepath.Join(tempDir, "atmos.yaml")
	if _, err := os.Stat(atmosPath); os.IsNotExist(err) {
		t.Error("Expected atmos.yaml to be created")
	}

	// Check that user values were saved
	valuesPath := filepath.Join(tempDir, ".atmos", "config.yaml")
	if _, err := os.Stat(valuesPath); os.IsNotExist(err) {
		t.Error("Expected .atmos/config.yaml to be created")
	}
}

func TestGenerateHelpText_ValuesExamples(t *testing.T) {
	helpText := generateHelpText()

	// Check for --values examples in help text
	expectedExamples := []string{
		"--values author=John --values year=2024 --values license=MIT",
		"--values project_name=my-project --values cloud_provider=aws --values enable_monitoring=true",
		"Set template values via command line",
		"Set template values and skip prompts",
	}

	for _, example := range expectedExamples {
		if !contains(helpText, example) {
			t.Errorf("Expected help text to contain: %s", example)
		}
	}
}

func TestParseValue_EdgeCases(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected interface{}
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "whitespace string",
			input:    "   ",
			expected: "   ",
		},
		{
			name:     "zero as string",
			input:    "0",
			expected: false, // Parsed as boolean
		},
		{
			name:     "zero as boolean",
			input:    "0",
			expected: false, // Parsed as boolean
		},
		{
			name:     "one as string",
			input:    "1",
			expected: true, // Parsed as boolean
		},
		{
			name:     "negative zero",
			input:    "-0",
			expected: 0,
		},
		{
			name:     "large number",
			input:    "123456789",
			expected: 123456789,
		},
		{
			name:     "decimal with leading zero",
			input:    "0.5",
			expected: 0.5,
		},
		{
			name:     "scientific notation",
			input:    "1.23e-4",
			expected: 1.23e-4,
		},
		{
			name:     "string that looks like number",
			input:    "123abc",
			expected: "123abc",
		},
		{
			name:     "boolean true variations",
			input:    "TRUE",
			expected: true, // Parsed as boolean
		},
		{
			name:     "boolean false variations",
			input:    "FALSE",
			expected: false, // Parsed as boolean
		},
		{
			name:     "special characters",
			input:    "!@#$%^&*()",
			expected: "!@#$%^&*()",
		},
		{
			name:     "unicode string",
			input:    "café",
			expected: "café",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := parseValue(tc.input)
			if err != nil {
				t.Fatalf("Expected no error for '%s', got: %v", tc.input, err)
			}

			if result != tc.expected {
				t.Errorf("Expected '%s' to parse as %v (%T), got %v (%T)",
					tc.input, tc.expected, tc.expected, result, result)
			}
		})
	}
}

func TestParseValue_ErrorCases(t *testing.T) {
	// parseValue should never return an error for valid strings
	// but let's test some edge cases
	testCases := []struct {
		name  string
		input string
	}{
		{
			name:  "very long string",
			input: strings.Repeat("a", 10000),
		},
		{
			name:  "string with null bytes",
			input: "hello\x00world",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseValue(tc.input)
			if err != nil {
				t.Errorf("Expected no error for '%s', got: %v", tc.input, err)
			}
		})
	}
}
