package templating

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/experiments/init/internal/config"
)

// TestShouldSkipFile tests the ShouldSkipFile function directly
func TestShouldSkipFile(t *testing.T) {
	processor := NewProcessor()

	testCases := []struct {
		name     string
		path     string
		expected bool
	}{
		{
			name:     "empty path",
			path:     "",
			expected: true,
		},
		{
			name:     "false path",
			path:     "false",
			expected: true,
		},
		{
			name:     "null path",
			path:     "null",
			expected: true,
		},
		{
			name:     "no value path",
			path:     "<no value>",
			expected: true,
		},
		{
			name:     "path with double slash",
			path:     "default//deep.yaml",
			expected: true,
		},
		{
			name:     "path starting with slash",
			path:     "/default/deep.yaml",
			expected: true,
		},
		{
			name:     "path ending with slash",
			path:     "default/deep.yaml/",
			expected: true,
		},
		{
			name:     "valid path",
			path:     "default/deep.yaml",
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := processor.ShouldSkipFile(tc.path)
			if result != tc.expected {
				t.Errorf("ShouldSkipFile(%q) = %v, expected %v", tc.path, result, tc.expected)
			}
		})
	}
}

// TestProcessTemplate tests basic template processing
func TestProcessTemplate(t *testing.T) {
	processor := NewProcessor()

	template := `Hello {{.ScaffoldPath}}!`
	targetPath := "/tmp/test"

	result, err := processor.ProcessTemplate(template, targetPath, nil, nil)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	expected := "Hello /tmp/test!"
	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

// TestProcessTemplateWithRichConfig tests template processing with rich configuration
func TestProcessTemplateWithRichConfig(t *testing.T) {
	processor := NewProcessor()

	// For testing, we don't need a full scaffold config - the templating processor
	// can work with just user values
	var scaffoldConfig *config.ScaffoldConfig = nil

	userValues := map[string]interface{}{
		"name":       "test-project",
		"author":     "John Doe",
		"license":    "MIT",
		"regions":    []string{"us-east-1", "us-west-2"},
		"monitoring": true,
	}

	tests := []struct {
		name     string
		template string
		expected string
	}{
		{
			name:     "basic template variables",
			template: `Project: {{.Config.name}}, Author: {{.Config.author}}`,
			expected: `Project: test-project, Author: John Doe`,
		},
		{
			name:     "config function",
			template: `License: {{config "license"}}, Monitoring: {{.Config.monitoring}}`,
			expected: `License: MIT, Monitoring: true`,
		},
		{
			name:     "array values",
			template: `Regions: {{range .Config.regions}}{{.}} {{end}}`,
			expected: `Regions: us-east-1 us-west-2 `,
		},
		{
			name:     "default function",
			template: `Name: {{default .Config.name "default-name"}}, Missing: {{default .Config.missing "fallback"}}`,
			expected: `Name: default-name, Missing: fallback`,
		},
		{
			name:     "mixed variables",
			template: `Path: {{.ScaffoldPath}}, Template: {{.Config.name}}, Description: {{.TemplateDescription}}`,
			expected: `Path: /tmp/test, Template: test-project, Description: An Atmos scaffold template for managing infrastructure as code`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := processor.ProcessTemplate(tt.template, "/tmp/test", scaffoldConfig, userValues)
			if err != nil {
				t.Fatalf("Expected no error, got: %v", err)
			}

			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// TestTemplateFilenameProcessing tests that file paths with templates are processed correctly
func TestTemplateFilenameProcessing(t *testing.T) {
	processor := NewProcessor()
	tempDir := t.TempDir()

	// For testing, we don't need a full scaffold config - the templating processor
	// can work with just user values
	var scaffoldConfig *config.ScaffoldConfig = nil

	userValues := map[string]interface{}{
		"namespace":   "production",
		"environment": "staging",
		"author":      "test-user",
	}

	testCases := []struct {
		name         string
		filePath     string
		expectedPath string
		shouldError  bool
	}{
		{
			name:         "simple namespace substitution",
			filePath:     "{{.Config.namespace}}/config.yaml",
			expectedPath: "production/config.yaml",
			shouldError:  false,
		},
		{
			name:         "nested path with multiple variables",
			filePath:     "{{.Config.namespace}}/{{.Config.environment}}/app.yaml",
			expectedPath: "production/staging/app.yaml",
			shouldError:  false,
		},
		{
			name:         "no template syntax",
			filePath:     "static/config.yaml",
			expectedPath: "static/config.yaml",
			shouldError:  false,
		},
		{
			name:         "project name in path",
			filePath:     "{{.TemplateName}}/{{.Config.namespace}}/config.yaml",
			expectedPath: filepath.Join(filepath.Base(tempDir), "production", "config.yaml"),
			shouldError:  false,
		},
		{
			name:         "invalid template syntax",
			filePath:     "{{.Config.nonexistent}",
			expectedPath: "",
			shouldError:  true,
		},
		{
			name:         "empty template result",
			filePath:     "{{.Config.empty}}/config.yaml",
			expectedPath: "<no value>/config.yaml", // This would be skipped in real usage due to empty segment
			shouldError:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := processor.ProcessTemplate(tc.filePath, tempDir, scaffoldConfig, userValues)

			if tc.shouldError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if result != tc.expectedPath {
					t.Errorf("Expected path %s, got %s", tc.expectedPath, result)
				}
			}
		})
	}
}

// TestProcessTemplateErrorHandling tests error handling in template processing
func TestProcessTemplateErrorHandling(t *testing.T) {
	processor := NewProcessor()

	testCases := []struct {
		name        string
		template    string
		expectError bool
	}{
		{
			name:        "invalid template syntax",
			template:    "{{.Config.name}",
			expectError: true,
		},
		{
			name:        "missing closing brace",
			template:    "{{.Config.name",
			expectError: true,
		},
		{
			name:        "valid template",
			template:    "Hello {{.Config.name}}!",
			expectError: false,
		},
		{
			name:        "invalid gomplate function",
			template:    "{{invalidFunction}}",
			expectError: true,
		},
		{
			name:        "invalid math operation",
			template:    "{{add .Config.invalid 5}}",
			expectError: false, // This might not error depending on how gomplate handles it
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := processor.ProcessTemplate(tc.template, "/tmp/test", nil, map[string]interface{}{"name": "test"})

			if tc.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tc.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// TestProcessTemplateWithDifferentDataTypes tests template processing with various data types
func TestProcessTemplateWithDifferentDataTypes(t *testing.T) {
	processor := NewProcessor()

	userValues := map[string]interface{}{
		"string": "hello",
		"int":    42,
		"float":  3.14,
		"bool":   true,
		"array":  []string{"a", "b", "c"},
		"map":    map[string]string{"key": "value"},
		"nil":    nil,
	}

	template := `String: {{.Config.string}}, Int: {{.Config.int}}, Float: {{.Config.float}}, Bool: {{.Config.bool}}, Array: {{range .Config.array}}{{.}} {{end}}, Map: {{.Config.map.key}}, Nil: {{.Config.nil}}`

	result, err := processor.ProcessTemplate(template, "/tmp/test", nil, userValues)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	expected := "String: hello, Int: 42, Float: 3.14, Bool: true, Array: a b c , Map: value, Nil: <no value>"
	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

// TestProcessTemplateWithGomplateFunctions tests various gomplate functions
func TestProcessTemplateWithGomplateFunctions(t *testing.T) {
	processor := NewProcessor()

	userValues := map[string]interface{}{
		"name":    "test",
		"numbers": []int{1, 2, 3, 4, 5},
		"text":    "hello world",
		"items":   []string{"apple", "banana", "cherry"},
	}

	testCases := []struct {
		name     string
		template string
		expected string
	}{
		{
			name:     "text transformations",
			template: `{{.Config.text | upper}} {{.Config.text | lower}} {{.Config.text | title}}`,
			expected: "HELLO WORLD hello world Hello World",
		},
		{
			name:     "default function",
			template: `{{default .Config.missing "fallback"}} {{.Config.name}}`,
			expected: "fallback test",
		},
		{
			name:     "array operations with slice",
			template: `{{slice .Config.numbers 0 3 | join ", "}}`,
			expected: "1, 2, 3",
		},
		{
			name:     "string to array conversion",
			template: `{{splitList " " .Config.text | join "-"}}`,
			expected: "hello-world",
		},
		{
			name:     "type checking",
			template: `{{if kindIs "string" .Config.name}}string{{else}}not string{{end}}`,
			expected: "string",
		},
		{
			name:     "math operations",
			template: `{{add 5 3}} {{mul 4 6}} {{sub 10 4}}`,
			expected: "8 24 6",
		},
		{
			name:     "conditional with kindIs",
			template: `{{if kindIs "slice" .Config.numbers}}slice{{else}}not slice{{end}}`,
			expected: "slice",
		},
		{
			name:     "string operations",
			template: `{{trim .Config.text}} {{len .Config.text}}`,
			expected: "hello world 11",
		},
		{
			name:     "coalesce function",
			template: `{{coalesce .Config.missing .Config.name "default"}}`,
			expected: "test",
		},
		{
			name:     "date functions",
			template: `{{now | date "2006-01-02"}}`,
			expected: "", // This will vary based on current time, so we'll just check it doesn't error
		},
		{
			name:     "array functions",
			template: `{{len .Config.items}} {{index .Config.items 1}}`,
			expected: "3 banana",
		},
		{
			name:     "string manipulation",
			template: `{{replace .Config.text "world" "gomplate"}}`,
			expected: "gomplate",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := processor.ProcessTemplate(tc.template, "/tmp/test", nil, userValues)
			if err != nil {
				t.Fatalf("Expected no error, got: %v", err)
			}

			// For date functions, we can't predict the exact output, so just check it's not empty
			if tc.name == "date functions" {
				if result == "" {
					t.Errorf("Expected non-empty result for date function")
				}
			} else if result != tc.expected {
				t.Errorf("Expected %q, got %q", tc.expected, result)
			}
		})
	}
}

// TestProcessTemplateWithRichProjectTemplate tests a more complex project template
func TestProcessTemplateWithRichProjectTemplate(t *testing.T) {
	processor := NewProcessor()

	userValues := map[string]interface{}{
		"project_name": "my-awesome-project",
		"author":       "Jane Doe",
		"license":      "Apache-2.0",
		"description":  "A fantastic project",
		"version":      "1.0.0",
		"features":     []string{"feature1", "feature2", "feature3"},
		"environments": []string{"dev", "staging", "prod"},
	}

	template := `# {{.Config.project_name}}

## Description
{{.Config.description}}

## Author
{{.Config.author}}

## License
{{.Config.license}}

## Version
{{.Config.version}}

## Features
{{range .Config.features}}- {{.}}
{{end}}

## Environments
{{range .Config.environments}}- {{.}}
{{end}}

## Project Path
{{.ScaffoldPath}}

## Template Info
{{.TemplateDescription}}
`

	result, err := processor.ProcessTemplate(template, "/tmp/my-project", nil, userValues)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	expectedLines := []string{
		"# my-awesome-project",
		"## Description",
		"A fantastic project",
		"## Author",
		"Jane Doe",
		"## License",
		"Apache-2.0",
		"## Version",
		"1.0.0",
		"## Features",
		"- feature1",
		"- feature2",
		"- feature3",
		"## Environments",
		"- dev",
		"- staging",
		"- prod",
		"## Project Path",
		"/tmp/my-project",
		"## Template Info",
		"An Atmos scaffold template for managing infrastructure as code",
	}

	for _, expectedLine := range expectedLines {
		if !strings.Contains(result, expectedLine) {
			t.Errorf("Expected result to contain %q, but it doesn't", expectedLine)
		}
	}
}

// TestProcessFile tests the ProcessFile method
func TestProcessFile_NewFile(t *testing.T) {
	processor := NewProcessor()
	tempDir := t.TempDir()

	file := File{
		Path:        "test.txt",
		Content:     "Hello, World!",
		IsTemplate:  false,
		Permissions: 0644,
	}

	err := processor.ProcessFile(file, tempDir, false, false, nil, nil)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Check that file was created
	filePath := filepath.Join(tempDir, "test.txt")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Errorf("Expected file to be created at %s", filePath)
	}

	// Check file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	if string(content) != "Hello, World!" {
		t.Errorf("Expected content 'Hello, World!', got '%s'", string(content))
	}
}

// TestProcessFile_ExistingFile_NoFlags tests ProcessFile with existing file and no flags
func TestProcessFile_ExistingFile_NoFlags(t *testing.T) {
	processor := NewProcessor()
	tempDir := t.TempDir()

	// Create an existing file
	filePath := filepath.Join(tempDir, "test.txt")
	if err := os.WriteFile(filePath, []byte("existing content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	file := File{
		Path:        "test.txt",
		Content:     "new content",
		IsTemplate:  false,
		Permissions: 0644,
	}

	err := processor.ProcessFile(file, tempDir, false, false, nil, nil)
	if err == nil {
		t.Errorf("Expected error for existing file, got none")
	}

	// Check that file content wasn't changed
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	if string(content) != "existing content" {
		t.Errorf("Expected content to remain 'existing content', got '%s'", string(content))
	}
}
