package config

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/viper"
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
