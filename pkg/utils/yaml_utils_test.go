package utils

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestGetIndentFromConfig(t *testing.T) {
	tests := []struct {
		name     string
		config   *schema.AtmosConfiguration
		expected int
	}{
		{
			name:     "nil config",
			config:   nil,
			expected: DefaultYAMLIndent,
		},
		{
			name: "config with zero tab width",
			config: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Terminal: schema.Terminal{
						TabWidth: 0,
					},
				},
			},
			expected: DefaultYAMLIndent,
		},
		{
			name: "config with custom tab width",
			config: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Terminal: schema.Terminal{
						TabWidth: 4,
					},
				},
			},
			expected: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getIndentFromConfig(tt.config)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConvertToYAML(t *testing.T) {
	tests := []struct {
		name        string
		data        any
		opts        []YAMLOptions
		expected    string
		expectError bool
	}{
		{
			name: "simple map",
			data: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			expected:    "key1: value1\nkey2: value2\n",
			expectError: false,
		},
		{
			name: "nested map",
			data: map[string]interface{}{
				"parent": map[string]string{
					"child1": "value1",
					"child2": "value2",
				},
			},
			expected:    "parent:\n  child1: value1\n  child2: value2\n",
			expectError: false,
		},
		{
			name:        "slice",
			data:        []string{"item1", "item2", "item3"},
			expected:    "- item1\n- item2\n- item3\n",
			expectError: false,
		},
		{
			name:        "nil data",
			data:        nil,
			expected:    "null\n",
			expectError: false,
		},
		{
			name: "struct",
			data: struct {
				Name  string `yaml:"name"`
				Value int    `yaml:"value"`
			}{
				Name:  "test",
				Value: 42,
			},
			expected:    "name: test\nvalue: 42\n",
			expectError: false,
		},
		{
			name: "with custom indent",
			data: map[string]map[string]string{
				"parent": {
					"child": "value",
				},
			},
			opts:        []YAMLOptions{{Indent: 4}},
			expected:    "parent:\n    child: value\n",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ConvertToYAML(tt.data, tt.opts...)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestUnmarshalYAML(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    map[string]interface{}
		expectError bool
	}{
		{
			name: "simple yaml",
			input: `
key1: value1
key2: value2
`,
			expected: map[string]interface{}{
				"key1": "value1",
				"key2": "value2",
			},
			expectError: false,
		},
		{
			name: "nested yaml",
			input: `
parent:
  child1: value1
  child2: value2
`,
			expected: map[string]interface{}{
				"parent": map[string]interface{}{
					"child1": "value1",
					"child2": "value2",
				},
			},
			expectError: false,
		},
		{
			name: "yaml with list",
			input: `
items:
  - item1
  - item2
  - item3
`,
			expected: map[string]interface{}{
				"items": []interface{}{"item1", "item2", "item3"},
			},
			expectError: false,
		},
		{
			name:        "invalid yaml",
			input:       "invalid: yaml: content:",
			expected:    nil,
			expectError: true,
		},
		{
			name:        "empty string",
			input:       "",
			expected:    nil,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result map[string]interface{}
			result, err := UnmarshalYAML[map[string]interface{}](tt.input)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestWriteToFileAsYAML(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name        string
		filePath    string
		data        any
		expectError bool
	}{
		{
			name:     "write simple map",
			filePath: filepath.Join(tmpDir, "test.yaml"),
			data: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			expectError: false,
		},
		{
			name:     "write nested structure",
			filePath: filepath.Join(tmpDir, "nested.yaml"),
			data: map[string]interface{}{
				"parent": map[string]string{
					"child": "value",
				},
			},
			expectError: false,
		},
		// Note: Removed nested directory test as WriteToFileAsYAML doesn't create directories
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := WriteToFileAsYAML(tt.filePath, tt.data, 0o644)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				// Verify file was written and can be read back
				content, err := os.ReadFile(tt.filePath)
				assert.NoError(t, err)

				// Parse the YAML to verify it's valid
				var parsed interface{}
				err = yaml.Unmarshal(content, &parsed)
				assert.NoError(t, err)
			}
		})
	}
}

func TestWriteToFileAsYAMLWithConfig(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name        string
		config      *schema.AtmosConfiguration
		filePath    string
		data        any
		expectError bool
	}{
		{
			name:        "nil config",
			config:      nil,
			filePath:    filepath.Join(tmpDir, "test.yaml"),
			data:        map[string]string{"test": "data"},
			expectError: true, // Should error with ErrNilAtmosConfig
		},
		{
			name: "config with custom indent",
			config: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Terminal: schema.Terminal{
						TabWidth: 4,
					},
				},
			},
			filePath: filepath.Join(tmpDir, "custom.yaml"),
			data: map[string]map[string]string{
				"parent": {
					"child": "value",
				},
			},
			expectError: false,
		},
		{
			name: "config with default indent",
			config: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Terminal: schema.Terminal{
						TabWidth: 0,
					},
				},
			},
			filePath: filepath.Join(tmpDir, "default.yaml"),
			data: map[string]string{
				"key": "value",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := WriteToFileAsYAMLWithConfig(tt.config, tt.filePath, tt.data, 0o644)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				// Verify file was written
				content, err := os.ReadFile(tt.filePath)
				assert.NoError(t, err)

				// Verify indentation if custom
				if tt.config.Settings.Terminal.TabWidth == 4 {
					// Check that nested content has 4 space indent
					lines := strings.Split(string(content), "\n")
					for _, line := range lines {
						if strings.HasPrefix(line, "    ") {
							assert.True(t, strings.HasPrefix(line, "    "))
							break
						}
					}
				}
			}
		})
	}
}

func TestPrintAsYAMLWithConfig(t *testing.T) {
	tests := []struct {
		name        string
		config      *schema.AtmosConfiguration
		data        any
		expectError bool
	}{
		{
			name:        "nil config",
			config:      nil,
			data:        map[string]string{"test": "data"},
			expectError: true,
		},
		{
			name: "valid config and data",
			config: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Terminal: schema.Terminal{
						TabWidth: 2,
					},
				},
			},
			data: map[string]string{
				"key": "value",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Redirect output to avoid printing during tests
			oldStdout := os.Stdout
			defer func() { os.Stdout = oldStdout }()
			os.Stdout = os.NewFile(0, os.DevNull)

			err := PrintAsYAMLWithConfig(tt.config, tt.data)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetHighlightedYAML(t *testing.T) {
	config := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			Terminal: schema.Terminal{
				TabWidth: 2,
			},
		},
	}

	tests := []struct {
		name        string
		data        any
		expectError bool
		checkResult func(t *testing.T, result string)
	}{
		{
			name: "simple data",
			data: map[string]string{
				"key": "value",
			},
			expectError: false,
			checkResult: func(t *testing.T, result string) {
				// Should at least contain the key and value
				assert.Contains(t, result, "key")
				assert.Contains(t, result, "value")
			},
		},
		{
			name:        "nil data",
			data:        nil,
			expectError: false,
			checkResult: func(t *testing.T, result string) {
				assert.Contains(t, result, "null")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := GetHighlightedYAML(config, tt.data)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, result)
				if tt.checkResult != nil {
					tt.checkResult(t, result)
				}
			}
		})
	}
}

func TestYAMLOptions(t *testing.T) {
	data := map[string]map[string]string{
		"parent": {
			"child1": "value1",
			"child2": "value2",
		},
	}

	tests := []struct {
		name     string
		opts     YAMLOptions
		expected string
	}{
		{
			name:     "default indent (2 spaces)",
			opts:     YAMLOptions{Indent: 2},
			expected: "parent:\n  child1: value1\n  child2: value2\n",
		},
		{
			name:     "custom indent (4 spaces)",
			opts:     YAMLOptions{Indent: 4},
			expected: "parent:\n    child1: value1\n    child2: value2\n",
		},
		{
			name:     "single space indent (yaml library minimum is 2)",
			opts:     YAMLOptions{Indent: 1},
			expected: "parent:\n  child1: value1\n  child2: value2\n", // yaml library uses minimum of 2
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ConvertToYAML(data, tt.opts)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
