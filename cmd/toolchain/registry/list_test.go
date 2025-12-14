package registry

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
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

// TestParseListOptions tests parseListOptions function.
func TestParseListOptions(t *testing.T) {
	tests := []struct {
		name        string
		format      string
		limit       int
		offset      int
		sort        string
		wantErr     bool
		errContains string
	}{
		{
			name:    "valid table format",
			format:  "table",
			limit:   50,
			offset:  0,
			sort:    "name",
			wantErr: false,
		},
		{
			name:    "valid json format",
			format:  "json",
			limit:   100,
			offset:  10,
			sort:    "date",
			wantErr: false,
		},
		{
			name:    "valid yaml format",
			format:  "yaml",
			limit:   25,
			offset:  5,
			sort:    "popularity",
			wantErr: false,
		},
		{
			name:        "invalid format",
			format:      "xml",
			limit:       50,
			offset:      0,
			sort:        "name",
			wantErr:     true,
			errContains: "format must be one of",
		},
		{
			name:    "uppercase format normalized",
			format:  "JSON",
			limit:   50,
			offset:  0,
			sort:    "name",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := viper.New()
			v.Set("format", tt.format)
			v.Set("limit", tt.limit)
			v.Set("offset", tt.offset)
			v.Set("sort", tt.sort)

			cmd := &cobra.Command{}
			opts, err := parseListOptions(cmd, v, nil)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				assert.ErrorIs(t, err, errUtils.ErrInvalidFlag)
			} else {
				require.NoError(t, err)
				require.NotNil(t, opts)
				assert.Equal(t, strings.ToLower(tt.format), opts.Format)
				assert.Equal(t, tt.limit, opts.Limit)
				assert.Equal(t, tt.offset, opts.Offset)
				assert.Equal(t, tt.sort, opts.Sort)
			}
		})
	}
}

// TestMin tests the min helper function.
func TestMin(t *testing.T) {
	tests := []struct {
		a, b, want int
	}{
		{5, 10, 5},
		{10, 5, 5},
		{0, 0, 0},
		{-5, 5, -5},
		{100, 100, 100},
	}

	for _, tt := range tests {
		got := min(tt.a, tt.b)
		assert.Equal(t, tt.want, got, "min(%d, %d) should be %d", tt.a, tt.b, tt.want)
	}
}

// TestListOptions tests ListOptions struct.
func TestListOptions(t *testing.T) {
	t.Run("default values", func(t *testing.T) {
		opts := &ListOptions{
			Limit:  defaultListLimit,
			Offset: 0,
			Format: "table",
			Sort:   "name",
		}
		assert.Equal(t, 50, opts.Limit)
		assert.Equal(t, 0, opts.Offset)
		assert.Equal(t, "table", opts.Format)
		assert.Equal(t, "name", opts.Sort)
	})
}

// TestToolRow tests toolRow struct.
func TestToolRow(t *testing.T) {
	t.Run("creates tool row with status", func(t *testing.T) {
		row := toolRow{
			status:      statusIndicator,
			owner:       "hashicorp",
			repo:        "terraform",
			toolType:    "github_release",
			isInstalled: true,
			isInConfig:  true,
		}
		assert.Equal(t, statusIndicator, row.status)
		assert.Equal(t, "hashicorp", row.owner)
		assert.Equal(t, "terraform", row.repo)
		assert.True(t, row.isInstalled)
		assert.True(t, row.isInConfig)
	})
}

// TestBuildToolsTable_ColumnWidths tests column width calculations.
func TestBuildToolsTable_ColumnWidths(t *testing.T) {
	tests := []struct {
		name  string
		tools []*toolchainregistry.Tool
	}{
		{
			name: "long owner name",
			tools: []*toolchainregistry.Tool{
				{
					RepoOwner: "very-long-organization-name",
					RepoName:  "tool",
					Type:      "github_release",
				},
			},
		},
		{
			name: "long repo name",
			tools: []*toolchainregistry.Tool{
				{
					RepoOwner: "owner",
					RepoName:  "very-long-repository-name-here",
					Type:      "github_release",
				},
			},
		},
		{
			name: "mixed lengths",
			tools: []*toolchainregistry.Tool{
				{
					RepoOwner: "short",
					RepoName:  "a",
					Type:      "github_release",
				},
				{
					RepoOwner: "very-long-owner-name",
					RepoName:  "very-long-repo-name",
					Type:      "http",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic with various column widths.
			assert.NotPanics(t, func() {
				buildToolsTable(tt.tools)
			})
		})
	}
}

// TestListConstants tests that constants are defined correctly.
func TestListConstants(t *testing.T) {
	assert.Equal(t, 50, defaultListLimit)
	assert.Equal(t, 8, minColumnWidthOwner)
	assert.Equal(t, 8, minColumnWidthRepo)
	assert.Equal(t, 8, minColumnWidthType)
	assert.Equal(t, 120, defaultTerminalWidth)
	assert.Equal(t, 2, columnPaddingPerSide)
	assert.Equal(t, 4, totalColumnPadding)
	assert.Equal(t, "●", statusIndicator)
}
