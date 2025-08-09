package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/experiments/init/embeds"
	"github.com/cloudposse/atmos/experiments/init/internal/config"
)

func TestNewInitUI(t *testing.T) {
	ui := NewInitUI()

	if ui.checkmark != "✓" {
		t.Errorf("Expected checkmark to be ✓, got %s", ui.checkmark)
	}

	if ui.xMark != "✗" {
		t.Errorf("Expected xMark to be ✗, got %s", ui.xMark)
	}

	if ui.maxChanges != 10 {
		t.Errorf("Expected maxChanges to be 10, got %d", ui.maxChanges)
	}
}

func TestProcessTemplate(t *testing.T) {
	ui := NewInitUI()

	template := `Hello {{.TargetPath}}!`
	targetPath := "/tmp/test"

	result, err := ui.processTemplate(template, targetPath, nil, nil)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	expected := "Hello /tmp/test!"
	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestProcessTemplateWithRichConfig(t *testing.T) {
	ui := NewInitUI()

	projectConfig := &config.ProjectConfig{
		Name:        "Test Project",
		Description: "Test project configuration",
		Fields: map[string]config.FieldDefinition{
			"project_name": {
				Key:     "project_name",
				Type:    "input",
				Label:   "Project Name",
				Default: "my-project",
			},
			"author": {
				Key:     "author",
				Type:    "input",
				Label:   "Author",
				Default: "Test Author",
			},
		},
	}

	userValues := map[string]interface{}{
		"project_name": "test-project",
		"author":       "John Doe",
		"license":      "MIT",
		"regions":      []string{"us-east-1", "us-west-2"},
		"monitoring":   true,
	}

	tests := []struct {
		name     string
		template string
		expected string
	}{
		{
			name:     "basic template variables",
			template: `Project: {{.Config.project_name}}, Author: {{.Config.author}}`,
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
			template: `Name: {{default .Config.project_name "default-name"}}, Missing: {{default .Config.missing "fallback"}}`,
			expected: `Name: default-name, Missing: fallback`,
		},
		{
			name:     "mixed variables",
			template: `Path: {{.TargetPath}}, Project: {{.Config.project_name}}, Description: {{.ProjectDescription}}`,
			expected: `Path: /tmp/test, Project: test-project, Description: An Atmos project for managing infrastructure as code`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ui.processTemplate(tt.template, "/tmp/test", projectConfig, userValues)
			if err != nil {
				t.Fatalf("Expected no error, got: %v", err)
			}

			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestProcessTemplateErrorHandling(t *testing.T) {
	ui := NewInitUI()

	tests := []struct {
		name        string
		template    string
		expectError bool
	}{
		{
			name:        "invalid template syntax",
			template:    `Hello {{.Invalid syntax}}`,
			expectError: true,
		},
		{
			name:        "missing closing brace",
			template:    `Hello {{.Config.project_name`,
			expectError: true,
		},
		{
			name:        "valid template",
			template:    `Hello {{.Config.project_name}}`,
			expectError: false,
		},
		{
			name:        "invalid gomplate function",
			template:    `Hello {{invalidFunction .Config.project_name}}`,
			expectError: true,
		},
		{
			name:        "invalid math operation",
			template:    `Result: {{div .Config.project_name 0}}`,
			expectError: true,
		},
	}

	userValues := map[string]interface{}{
		"project_name": "test-project",
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ui.processTemplate(tt.template, "/tmp/test", nil, userValues)
			if tt.expectError && err == nil {
				t.Error("Expected error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error, got: %v", err)
			}
		})
	}
}

func TestProcessTemplateWithDifferentDataTypes(t *testing.T) {
	ui := NewInitUI()

	userValues := map[string]interface{}{
		"string_value": "hello",
		"int_value":    42,
		"float_value":  3.14,
		"bool_value":   true,
		"array_value":  []string{"a", "b", "c"},
		"map_value":    map[string]interface{}{"key": "value"},
	}

	template := `String: {{.Config.string_value}}, Int: {{.Config.int_value}}, Float: {{.Config.float_value}}, Bool: {{.Config.bool_value}}, Array: {{range .Config.array_value}}{{.}}{{end}}, Map: {{.Config.map_value.key}}`

	result, err := ui.processTemplate(template, "/tmp/test", nil, userValues)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	expected := `String: hello, Int: 42, Float: 3.14, Bool: true, Array: abc, Map: value`
	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestProcessTemplateWithGomplateFunctions(t *testing.T) {
	ui := NewInitUI()

	userValues := map[string]interface{}{
		"project_name":   "my-awesome-project",
		"author":         "john doe",
		"cloud_provider": "aws",
		"environment":    "dev",
		"regions":        []string{"us-east-1", "us-west-2"},
		"regions_string": "us-east-1,us-west-2,eu-west-1",
		"number":         42,
		"empty_value":    "",
	}

	tests := []struct {
		name     string
		template string
		expected string
	}{
		{
			name:     "text transformations",
			template: `Title: {{.Config.project_name | title}}, Upper: {{.Config.author | upper}}, Lower: {{.Config.cloud_provider | lower}}`,
			expected: `Title: My-Awesome-Project, Upper: JOHN DOE, Lower: aws`,
		},
		{
			name:     "default function",
			template: `Author: {{.Config.author | default "Unknown"}}, Empty: {{.Config.empty_value | default "Default Value"}}`,
			expected: `Author: john doe, Empty: Default Value`,
		},
		{
			name:     "array operations with slice",
			template: `Regions: {{range .Config.regions}}{{. | upper}} {{end}}, Count: {{len .Config.regions}}`,
			expected: `Regions: US-EAST-1 US-WEST-2 , Count: 2`,
		},
		{
			name:     "string to array conversion",
			template: `Regions: {{range (splitList "," .Config.regions_string)}}{{trim . | upper}} {{end}}, Count: {{len (splitList "," .Config.regions_string)}}`,
			expected: `Regions: US-EAST-1 US-WEST-2 EU-WEST-1 , Count: 3`,
		},
		{
			name:     "type checking",
			template: `IsSlice: {{kindIs "slice" .Config.regions}}, IsString: {{kindIs "string" .Config.author}}`,
			expected: `IsSlice: true, IsString: true`,
		},
		{
			name:     "math operations",
			template: `Number: {{.Config.number}}, Doubled: {{mul .Config.number 2}}, Half: {{div .Config.number 2}}`,
			expected: `Number: 42, Doubled: 84, Half: 21`,
		},
		{
			name:     "conditional with kindIs",
			template: `{{if (kindIs "slice" .Config.regions)}}Slice: {{len .Config.regions}} items{{else}}String: {{.Config.regions}}{{end}}`,
			expected: `Slice: 2 items`,
		},
		{
			name:     "string operations",
			template: `Original: {{.Config.author}}, Title: {{.Config.author | title}}, Trimmed: {{trim "  hello  "}}`,
			expected: `Original: john doe, Title: John Doe, Trimmed: hello`,
		},
		{
			name:     "coalesce function",
			template: `Value1: {{coalesce .Config.missing .Config.author "fallback"}}, Value2: {{coalesce .Config.project_name .Config.author}}`,
			expected: `Value1: john doe, Value2: my-awesome-project`,
		},
		{
			name:     "date functions",
			template: `Now: {{now | date "2006"}}, Length: {{len (now | date "2006-01-02")}}`,
			expected: `Now: 2025, Length: 10`,
		},
		{
			name:     "array functions",
			template: `First: {{first .Config.regions}}, Last: {{last .Config.regions}}, Rest: {{range (rest .Config.regions)}}{{.}} {{end}}`,
			expected: `First: us-east-1, Last: us-west-2, Rest: us-west-2 `,
		},
		{
			name:     "string manipulation",
			template: `Original: {{.Config.project_name}}, Replace: {{replace .Config.project_name "-" "_"}}, Repeat: {{repeat 3 "x"}}`,
			expected: `Original: my-awesome-project, Replace: _, Repeat: xxx`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ui.processTemplate(tt.template, "/tmp/test", nil, userValues)
			if err != nil {
				t.Fatalf("Expected no error, got: %v", err)
			}

			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestProcessTemplateWithRichProjectTemplate(t *testing.T) {
	ui := NewInitUI()

	// Test data that matches what the rich-project template expects
	userValues := map[string]interface{}{
		"project_name":        "my-awesome-project",
		"project_description": "An Atmos project for managing infrastructure as code",
		"author":              "John Doe",
		"year":                "2024",
		"license":             "MIT",
		"cloud_provider":      "aws",
		"environment":         "dev",
		"terraform_version":   "1.5.0",
		"regions":             []string{"us-east-1", "us-west-2", "eu-west-1"},
		"enable_monitoring":   true,
		"enable_logging":      true,
	}

	// This is a simplified version of the rich-project README template
	richProjectTemplate := `# {{ .Config.project_name | title }}

{{ .Config.project_description }}

## Project Information

- **Author**: {{ .Config.author | default "Unknown" }}
- **Year**: {{ .Config.year | default "2024" }}
- **License**: {{ .Config.license | default "MIT" }}
- **Cloud Provider**: {{ .Config.cloud_provider | upper }}
- **Environment**: {{ .Config.environment | title }}
- **Terraform Version**: {{ .Config.terraform_version | default "latest" }}

{{ if .Config.regions }}
## AWS Regions

This project is configured to deploy to the following AWS regions:
{{ if (kindIs "slice" .Config.regions) }}
{{ range .Config.regions }}
- {{ . | upper }}
{{ end }}
{{ else }}
{{ range (splitList "," (toString .Config.regions)) }}
- {{ trim . | upper }}
{{ end }}
{{ end }}

**Total Regions**: {{ if (kindIs "slice" .Config.regions) }}{{ len .Config.regions }}{{ else }}{{ len (splitList "," (toString .Config.regions)) }}{{ end }}
{{ end }}

{{ if .Config.enable_monitoring }}
## Monitoring

This project includes monitoring and alerting infrastructure.
{{ end }}

{{ if .Config.enable_logging }}
## Logging

This project includes centralized logging infrastructure.
{{ end }}

## License

{{ .Config.license }}

Copyright (c) {{ .Config.year }} {{ .Config.author }}`

	result, err := ui.processTemplate(richProjectTemplate, "/tmp/test", nil, userValues)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Check that key transformations worked
	expectedSnippets := []string{
		"My-Awesome-Project",          // title case
		"AWS",                         // upper case
		"Dev",                         // title case
		"US-EAST-1",                   // upper case regions
		"US-WEST-2",                   // upper case regions
		"EU-WEST-1",                   // upper case regions
		"**Total Regions**: 3",        // count
		"Monitoring",                  // conditional section
		"Logging",                     // conditional section
		"Copyright (c) 2024 John Doe", // copyright
	}

	for _, snippet := range expectedSnippets {
		if !strings.Contains(result, snippet) {
			t.Errorf("Expected result to contain %q, but it didn't. Result: %s", snippet, result)
		}
	}

	// Verify the structure is correct
	if !strings.Contains(result, "# My-Awesome-Project") {
		t.Error("Expected title to be properly formatted")
	}

	if !strings.Contains(result, "**Total Regions**: 3") {
		t.Error("Expected region count to be calculated correctly")
	}
}

func TestProcessFile_NewFile(t *testing.T) {
	ui := NewInitUI()
	tempDir := t.TempDir()

	file := embeds.File{
		Path:        "test.txt",
		Content:     "Hello World!",
		IsTemplate:  false,
		Permissions: 0644,
	}

	err := ui.processFile(file, tempDir, false, false)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	filePath := filepath.Join(tempDir, "test.txt")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Error("Expected file to be created")
	}
}

func TestProcessFile_ExistingFile_NoFlags(t *testing.T) {
	ui := NewInitUI()
	tempDir := t.TempDir()

	// Create existing file
	filePath := filepath.Join(tempDir, "test.txt")
	err := os.WriteFile(filePath, []byte("existing content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	file := embeds.File{
		Path:        "test.txt",
		Content:     "new content",
		IsTemplate:  false,
		Permissions: 0644,
	}

	err = ui.processFile(file, tempDir, false, false)
	if err == nil {
		t.Fatal("Expected error for existing file")
	}

	if !strings.Contains(err.Error(), "file already exists") {
		t.Errorf("Expected error about existing file, got: %v", err)
	}
}

func TestIsUserCustomization(t *testing.T) {
	ui := NewInitUI()

	tests := []struct {
		name     string
		line     string
		expected bool
	}{
		{"empty line", "", false},
		{"comment", "# This is a comment", false},
		{"custom comment", "# Custom configuration", false}, // All # comments are skipped
		{"normal line", "base_path: .", false},
		{"custom setting", "custom_setting: true", true},
		{"my setting", "my_setting: value", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ui.isUserCustomization(tt.line)
			if result != tt.expected {
				t.Errorf("isUserCustomization(%q) = %v, want %v", tt.line, result, tt.expected)
			}
		})
	}
}
