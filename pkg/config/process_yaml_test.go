package config

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"go.yaml.in/yaml/v3"
)

func TestPreprocessAtmosYamlFunc(t *testing.T) {
	tests := []struct {
		name     string
		yamlStr  string                            // Static YAML or format string
		setup    func(*testing.T) (string, func()) // Returns dynamic YAML and cleanup
		expected map[string]interface{}
		wantErr  bool
	}{
		{
			name: "sequence of mappings with same key",
			setup: func(t *testing.T) (string, func()) {
				t.Setenv("TEST_SERVER_1_NAME", "a")
				t.Setenv("TEST_SERVER_2_NAME", "b")
				yamlContent := `
servers:
  - name: !env TEST_SERVER_1_NAME
  - name: !env TEST_SERVER_2_NAME
`
				return yamlContent, func() {
					os.Unsetenv("TEST_SERVER_1_NAME")
					os.Unsetenv("TEST_SERVER_2_NAME")
				}
			},
			expected: map[string]interface{}{
				"servers[0].name": "a",
				"servers[1].name": "b",
			},
			wantErr: false,
		},
		{
			name: "process !env directive with empty value",
			yamlStr: `
key: !env TEST_EMPTY_VAR
`,
			setup: func(t *testing.T) (string, func()) {
				t.Setenv("TEST_EMPTY_VAR", "")
				return `
key: !env TEST_EMPTY_VAR
`, func() {}
			},
			expected: map[string]interface{}{
				"key": "",
			},
			wantErr: false,
		},
		{
			name: "process !env directive",
			yamlStr: `
key: !env TEST_ENV_VAR
`,
			setup: func(t *testing.T) (string, func()) {
				t.Setenv("TEST_ENV_VAR", "test_value")
				return `
key: !env TEST_ENV_VAR
`, func() { os.Unsetenv("TEST_ENV_VAR") }
			},
			expected: map[string]interface{}{
				"key": "test_value",
			},
			wantErr: false,
		},
		{
			name: "process !exec directive",
			yamlStr: `
key: !exec "echo hello"
`,
			expected: map[string]interface{}{
				"key": "hello",
			},
			wantErr: false,
		},
		{
			name: "process !exec directive with empty output",
			yamlStr: `
key: !exec "echo ''"
`,
			expected: map[string]interface{}{
				"key": "",
			},
			wantErr: false,
		},
		{
			name:    "process !include directive",
			yamlStr: `key: !include %s`, // Format string placeholder
			setup: func(t *testing.T) (string, func()) {
				tmpfile, err := os.CreateTemp("", "include-test-*.yaml")
				if err != nil {
					t.Fatal(err)
				}
				defer tmpfile.Close()

				content := []byte("included_key: included_value")
				if _, err := tmpfile.Write(content); err != nil {
					t.Fatal(err)
				}

				// Generate the dynamic YAML with the temp file path
				dynamicYAML := fmt.Sprintf("key: !include %s", tmpfile.Name())
				return dynamicYAML, func() { os.Remove(tmpfile.Name()) }
			},
			expected: map[string]interface{}{
				"key": map[string]interface{}{
					"included_key": "included_value",
				},
			},
			wantErr: false,
		},
		{
			name:    "process !include directive with empty file",
			yamlStr: `key: !include %s`, // Format string placeholder
			setup: func(t *testing.T) (string, func()) {
				tmpfile, err := os.CreateTemp("", "include-empty-*.yaml")
				if err != nil {
					t.Fatal(err)
				}
				defer tmpfile.Close()

				// Write empty content
				if _, err := tmpfile.Write([]byte("")); err != nil {
					t.Fatal(err)
				}

				// Generate the dynamic YAML with the temp file path
				dynamicYAML := fmt.Sprintf("key: !include %s", tmpfile.Name())
				return dynamicYAML, func() { os.Remove(tmpfile.Name()) }
			},
			expected: map[string]interface{}{
				// Empty file results in no key being set
			},
			wantErr: false,
		},
		{
			name: "nested mappings and sequences",
			yamlStr: `
parent:
  child: !env NESTED_ENV_VAR
  list:
    - !exec "echo first"
    - !include %s
`,
			setup: func(t *testing.T) (string, func()) {
				t.Setenv("NESTED_ENV_VAR", "nested_value")

				tmpfile, err := os.CreateTemp("", "nested-include-*.yaml")
				if err != nil {
					t.Fatal(err)
				}
				defer tmpfile.Close()

				content := []byte("included: value")
				if _, err := tmpfile.Write(content); err != nil {
					t.Fatal(err)
				}

				// Generate dynamic YAML with the temp file path
				dynamicYAML := fmt.Sprintf(`
parent:
  child: !env NESTED_ENV_VAR
  list:
    - !exec "echo first"
    - !include %s
`, tmpfile.Name())
				return dynamicYAML, func() {
					os.Unsetenv("NESTED_ENV_VAR")
					os.Remove(tmpfile.Name())
				}
			},
			expected: map[string]interface{}{
				"parent.child":            "nested_value",
				"parent.list[0]":          "first",
				"parent.list[1].included": "value",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Determine the YAML content to use
			var yamlContent string

			if tt.setup != nil {
				// Use the dynamic YAML from setup
				var cleanup func()
				yamlContent, cleanup = tt.setup(t)
				if cleanup != nil {
					defer cleanup()
				}
			} else {
				// Use the static YAML
				yamlContent = tt.yamlStr
			}

			v := viper.New()
			err := preprocessAtmosYamlFunc([]byte(yamlContent), v)

			if (err != nil) != tt.wantErr {
				t.Fatalf("preprocessAtmosYamlFunc() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Verify expected values in Viper
			for key, expectedValue := range tt.expected {
				actualValue := v.Get(key)
				// check type if string trim spaces

				if str, ok := actualValue.(string); ok {
					actualValue = strings.TrimSpace(str)
				}

				if !reflect.DeepEqual(actualValue, expectedValue) {
					t.Errorf("Key %s: expected %v (%T), got %v (%T)",
						key, expectedValue, expectedValue, actualValue, actualValue)
				}
			}
		})
	}
}

func TestHasCustomTag(t *testing.T) {
	tests := []struct {
		name     string
		tag      string
		expected bool
	}{
		{
			name:     "env tag",
			tag:      "!env",
			expected: true,
		},
		{
			name:     "env tag with value",
			tag:      "!env VAR_NAME",
			expected: true,
		},
		{
			name:     "exec tag",
			tag:      "!exec",
			expected: true,
		},
		{
			name:     "include tag",
			tag:      "!include",
			expected: true,
		},
		{
			name:     "repo-root tag",
			tag:      "!repo-root",
			expected: true,
		},
		{
			name:     "random tag",
			tag:      "!random",
			expected: true,
		},
		{
			name:     "random tag with params",
			tag:      "!random.string",
			expected: true,
		},
		{
			name:     "non-custom tag",
			tag:      "!!str",
			expected: false,
		},
		{
			name:     "empty tag",
			tag:      "",
			expected: false,
		},
		{
			name:     "regular yaml tag",
			tag:      "tag:yaml.org,2002:str",
			expected: false,
		},
		{
			name:     "unknown custom tag",
			tag:      "!unknown",
			expected: false,
		},
		{
			name:     "store tag (not in hasCustomTag list)",
			tag:      "!store",
			expected: false,
		},
		{
			name:     "template tag (not in hasCustomTag list)",
			tag:      "!template",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasCustomTag(tt.tag)
			if result != tt.expected {
				t.Errorf("hasCustomTag(%q) = %v, expected %v", tt.tag, result, tt.expected)
			}
		})
	}
}

func TestContainsCustomTags(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() *yaml.Node
		expected bool
	}{
		{
			name: "nil node",
			setup: func() *yaml.Node {
				return nil
			},
			expected: false,
		},
		{
			name: "scalar node with env tag",
			setup: func() *yaml.Node {
				return &yaml.Node{
					Kind:  yaml.ScalarNode,
					Tag:   "!env",
					Value: "VAR_NAME",
				}
			},
			expected: true,
		},
		{
			name: "scalar node with exec tag",
			setup: func() *yaml.Node {
				return &yaml.Node{
					Kind:  yaml.ScalarNode,
					Tag:   "!exec",
					Value: "echo hello",
				}
			},
			expected: true,
		},
		{
			name: "scalar node without custom tag",
			setup: func() *yaml.Node {
				return &yaml.Node{
					Kind:  yaml.ScalarNode,
					Tag:   "!!str",
					Value: "plain text",
				}
			},
			expected: false,
		},
		{
			name: "mapping node with child containing env tag",
			setup: func() *yaml.Node {
				return &yaml.Node{
					Kind: yaml.MappingNode,
					Content: []*yaml.Node{
						{Kind: yaml.ScalarNode, Value: "key"},
						{Kind: yaml.ScalarNode, Tag: "!env", Value: "VAR"},
					},
				}
			},
			expected: true,
		},
		{
			name: "sequence node with element containing include tag",
			setup: func() *yaml.Node {
				return &yaml.Node{
					Kind: yaml.SequenceNode,
					Content: []*yaml.Node{
						{Kind: yaml.ScalarNode, Value: "item1"},
						{Kind: yaml.ScalarNode, Tag: "!include", Value: "file.yaml"},
					},
				}
			},
			expected: true,
		},
		{
			name: "nested structure with custom tag deep inside",
			setup: func() *yaml.Node {
				return &yaml.Node{
					Kind: yaml.MappingNode,
					Content: []*yaml.Node{
						{Kind: yaml.ScalarNode, Value: "parent"},
						{
							Kind: yaml.MappingNode,
							Content: []*yaml.Node{
								{Kind: yaml.ScalarNode, Value: "child"},
								{Kind: yaml.ScalarNode, Tag: "!random", Value: "string"},
							},
						},
					},
				}
			},
			expected: true,
		},
		{
			name: "nested structure without custom tags",
			setup: func() *yaml.Node {
				return &yaml.Node{
					Kind: yaml.MappingNode,
					Content: []*yaml.Node{
						{Kind: yaml.ScalarNode, Value: "key1"},
						{Kind: yaml.ScalarNode, Value: "value1"},
						{Kind: yaml.ScalarNode, Value: "key2"},
						{
							Kind: yaml.SequenceNode,
							Content: []*yaml.Node{
								{Kind: yaml.ScalarNode, Value: "item1"},
								{Kind: yaml.ScalarNode, Value: "item2"},
							},
						},
					},
				}
			},
			expected: false,
		},
		{
			name: "empty mapping node",
			setup: func() *yaml.Node {
				return &yaml.Node{
					Kind:    yaml.MappingNode,
					Content: []*yaml.Node{},
				}
			},
			expected: false,
		},
		{
			name: "empty sequence node",
			setup: func() *yaml.Node {
				return &yaml.Node{
					Kind:    yaml.SequenceNode,
					Content: []*yaml.Node{},
				}
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := tt.setup()
			result := containsCustomTags(node)
			if result != tt.expected {
				t.Errorf("containsCustomTags() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestProcessScalarNodeValue(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T) *yaml.Node
		wantErr   bool
		checkFunc func(t *testing.T, result any)
	}{
		{
			name: "env tag with existing variable",
			setup: func(t *testing.T) *yaml.Node {
				t.Setenv("TEST_PROCESS_SCALAR_VAR", "test_value")
				return &yaml.Node{
					Kind:  yaml.ScalarNode,
					Tag:   "!env",
					Value: "TEST_PROCESS_SCALAR_VAR",
				}
			},
			wantErr: false,
			checkFunc: func(t *testing.T, result any) {
				if str, ok := result.(string); !ok || str != "test_value" {
					t.Errorf("Expected 'test_value', got %v (%T)", result, result)
				}
			},
		},
		{
			name: "env tag with missing variable returns empty string",
			setup: func(t *testing.T) *yaml.Node {
				return &yaml.Node{
					Kind:  yaml.ScalarNode,
					Tag:   "!env",
					Value: "NONEXISTENT_VAR_12345",
				}
			},
			wantErr: false,
			checkFunc: func(t *testing.T, result any) {
				if str, ok := result.(string); !ok || str != "" {
					t.Errorf("Expected empty string for missing env var, got %v (%T)", result, result)
				}
			},
		},
		{
			name: "exec tag with simple command",
			setup: func(t *testing.T) *yaml.Node {
				return &yaml.Node{
					Kind:  yaml.ScalarNode,
					Tag:   "!exec",
					Value: "echo hello",
				}
			},
			wantErr: false,
			checkFunc: func(t *testing.T, result any) {
				if str, ok := result.(string); !ok || str == "" {
					t.Errorf("Expected non-empty string from exec, got %v (%T)", result, result)
				}
			},
		},
		{
			name: "random tag with min max values",
			setup: func(t *testing.T) *yaml.Node {
				return &yaml.Node{
					Kind:  yaml.ScalarNode,
					Tag:   "!random",
					Value: "1000 9999",
				}
			},
			wantErr: false,
			checkFunc: func(t *testing.T, result any) {
				if num, ok := result.(int); !ok || num < 1000 || num > 9999 {
					t.Errorf("Expected random int between 1000-9999, got %v (%T)", result, result)
				}
			},
		},
		{
			name: "unknown tag returns decoded value",
			setup: func(t *testing.T) *yaml.Node {
				return &yaml.Node{
					Kind:  yaml.ScalarNode,
					Tag:   "!!str",
					Value: "plain value",
				}
			},
			wantErr: false,
			checkFunc: func(t *testing.T, result any) {
				if str, ok := result.(string); !ok || str != "plain value" {
					t.Errorf("Expected 'plain value', got %v (%T)", result, result)
				}
			},
		},
		{
			name: "numeric node with standard tag",
			setup: func(t *testing.T) *yaml.Node {
				return &yaml.Node{
					Kind:  yaml.ScalarNode,
					Tag:   "!!int",
					Value: "42",
				}
			},
			wantErr: false,
			checkFunc: func(t *testing.T, result any) {
				if num, ok := result.(int); !ok || num != 42 {
					t.Errorf("Expected 42 (int), got %v (%T)", result, result)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := tt.setup(t)
			result, err := processScalarNodeValue(node)

			if (err != nil) != tt.wantErr {
				t.Errorf("processScalarNodeValue() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && tt.checkFunc != nil {
				tt.checkFunc(t, result)
			}
		})
	}
}
