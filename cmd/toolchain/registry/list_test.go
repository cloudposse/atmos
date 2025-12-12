package registry

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"

	toolchainregistry "github.com/cloudposse/atmos/toolchain/registry"
)

// TestListCommand_FormatFlagValidation tests format flag validation.
func TestListCommand_FormatFlagValidation(t *testing.T) {
	tests := []struct {
		name        string
		format      string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "table format is valid",
			format:      "table",
			expectError: false,
		},
		{
			name:        "json format is valid",
			format:      "json",
			expectError: false,
		},
		{
			name:        "yaml format is valid",
			format:      "yaml",
			expectError: false,
		},
		{
			name:        "invalid format xml",
			format:      "xml",
			expectError: true,
			errorMsg:    "format must be one of: table, json, yaml",
		},
		{
			name:        "invalid format csv",
			format:      "csv",
			expectError: true,
			errorMsg:    "format must be one of: table, json, yaml",
		},
		{
			name:        "uppercase JSON is valid after normalization",
			format:      "JSON",
			expectError: false,
		},
		{
			name:        "uppercase YAML is valid after normalization",
			format:      "YAML",
			expectError: false,
		},
		{
			name:        "uppercase TABLE is valid after normalization",
			format:      "TABLE",
			expectError: false,
		},
		{
			name:        "mixed case Table is valid after normalization",
			format:      "Table",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create new Viper instance for test isolation.
			v := viper.New()
			v.Set("format", tt.format)
			v.Set("limit", 5)
			v.Set("offset", 0)
			v.Set("sort", "name")

			// Test format validation logic.
			listFormat := strings.ToLower(v.GetString("format"))
			var err error

			// Validate format (same logic as in listRegistryTools).
			switch listFormat {
			case "table", "json", "yaml":
				// Valid formats.
			default:
				err = assert.AnError
			}

			if tt.expectError {
				assert.Error(t, err, "should reject invalid format: %s", tt.format)
			} else {
				assert.NoError(t, err, "should accept valid format: %s", tt.format)
			}
		})
	}
}

// TestListCommand_JSONMarshalling tests that tools can be marshalled to JSON.
func TestListCommand_JSONMarshalling(t *testing.T) {
	tools := []*toolchainregistry.Tool{
		{
			Name:      "terraform",
			RepoOwner: "hashicorp",
			RepoName:  "terraform",
			Type:      "github_release",
		},
		{
			Name:      "kubectl",
			RepoOwner: "kubernetes",
			RepoName:  "kubectl",
			Type:      "github_release",
		},
	}

	// Marshal to JSON.
	output, err := json.MarshalIndent(tools, "", "  ")
	assert.NoError(t, err, "should marshal tools to JSON")

	// Verify JSON structure.
	var unmarshalled []*toolchainregistry.Tool
	err = json.Unmarshal(output, &unmarshalled)
	assert.NoError(t, err, "should unmarshal JSON")
	assert.Len(t, unmarshalled, 2, "should have 2 tools")
	assert.Equal(t, "terraform", unmarshalled[0].Name)
	assert.Equal(t, "kubectl", unmarshalled[1].Name)
}

// TestListCommand_YAMLMarshalling tests that tools can be marshalled to YAML.
func TestListCommand_YAMLMarshalling(t *testing.T) {
	tools := []*toolchainregistry.Tool{
		{
			Name:      "terraform",
			RepoOwner: "hashicorp",
			RepoName:  "terraform",
			Type:      "github_release",
		},
		{
			Name:      "kubectl",
			RepoOwner: "kubernetes",
			RepoName:  "kubectl",
			Type:      "github_release",
		},
	}

	// Marshal to YAML.
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	err := encoder.Encode(tools)
	assert.NoError(t, err, "should marshal tools to YAML")

	// Verify YAML structure.
	var unmarshalled []*toolchainregistry.Tool
	err = yaml.Unmarshal(buf.Bytes(), &unmarshalled)
	assert.NoError(t, err, "should unmarshal YAML")
	assert.Len(t, unmarshalled, 2, "should have 2 tools")
	assert.Equal(t, "terraform", unmarshalled[0].Name)
	assert.Equal(t, "kubectl", unmarshalled[1].Name)
}
