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
	"github.com/cloudposse/atmos/toolchain"
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

// TestGetStatusIndicator tests the getStatusIndicator function.
func TestGetStatusIndicator(t *testing.T) {
	tests := []struct {
		name        string
		isInstalled bool
		isInConfig  bool
		want        string
	}{
		{
			name:        "installed returns dot",
			isInstalled: true,
			isInConfig:  true,
			want:        statusIndicator,
		},
		{
			name:        "in config but not installed returns dot",
			isInstalled: false,
			isInConfig:  true,
			want:        statusIndicator,
		},
		{
			name:        "not in config returns space",
			isInstalled: false,
			isInConfig:  false,
			want:        " ",
		},
		{
			name:        "installed takes precedence",
			isInstalled: true,
			isInConfig:  false,
			want:        statusIndicator,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getStatusIndicator(tt.isInstalled, tt.isInConfig)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestUpdateColumnWidths tests the updateColumnWidths function.
func TestUpdateColumnWidths(t *testing.T) {
	tests := []struct {
		name         string
		initialWidth columnWidths
		tool         *toolchainregistry.Tool
		wantOwner    int
		wantRepo     int
		wantType     int
	}{
		{
			name: "updates owner width when longer",
			initialWidth: columnWidths{
				owner: 5,
				repo:  5,
				tType: 5,
			},
			tool: &toolchainregistry.Tool{
				RepoOwner: "verylongowner",
				RepoName:  "repo",
				Type:      "http",
			},
			wantOwner: 13,
			wantRepo:  5,
			wantType:  5,
		},
		{
			name: "updates repo width when longer",
			initialWidth: columnWidths{
				owner: 5,
				repo:  5,
				tType: 5,
			},
			tool: &toolchainregistry.Tool{
				RepoOwner: "own",
				RepoName:  "verylongreponame",
				Type:      "http",
			},
			wantOwner: 5,
			wantRepo:  16,
			wantType:  5,
		},
		{
			name: "updates type width when longer",
			initialWidth: columnWidths{
				owner: 5,
				repo:  5,
				tType: 5,
			},
			tool: &toolchainregistry.Tool{
				RepoOwner: "own",
				RepoName:  "repo",
				Type:      "github_release",
			},
			wantOwner: 5,
			wantRepo:  5,
			wantType:  14,
		},
		{
			name: "keeps original width when tool is shorter",
			initialWidth: columnWidths{
				owner: 20,
				repo:  20,
				tType: 20,
			},
			tool: &toolchainregistry.Tool{
				RepoOwner: "own",
				RepoName:  "repo",
				Type:      "http",
			},
			wantOwner: 20,
			wantRepo:  20,
			wantType:  20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := updateColumnWidths(tt.initialWidth, tt.tool)
			assert.Equal(t, tt.wantOwner, got.owner)
			assert.Equal(t, tt.wantRepo, got.repo)
			assert.Equal(t, tt.wantType, got.tType)
		})
	}
}

// TestApplyColumnPaddingAndTruncation tests column padding and width truncation.
func TestApplyColumnPaddingAndTruncation(t *testing.T) {
	tests := []struct {
		name      string
		widths    columnWidths
		termWidth int
		checkFunc func(t *testing.T, result columnWidths)
	}{
		{
			name: "adds padding to columns",
			widths: columnWidths{
				status: 1,
				owner:  10,
				repo:   10,
				tType:  10,
			},
			termWidth: 200, // Wide terminal, no truncation needed.
			checkFunc: func(t *testing.T, result columnWidths) {
				assert.Equal(t, 3, result.status) // status + 2
				assert.Equal(t, 14, result.owner) // 10 + 4
				assert.Equal(t, 14, result.repo)  // 10 + 4
				assert.Equal(t, 14, result.tType) // 10 + 4
			},
		},
		{
			name: "truncates columns when terminal is narrow",
			widths: columnWidths{
				status: 1,
				owner:  50,
				repo:   50,
				tType:  50,
			},
			termWidth: 100, // Narrow terminal, truncation needed.
			checkFunc: func(t *testing.T, result columnWidths) {
				// Should truncate but maintain minimum widths.
				assert.GreaterOrEqual(t, result.owner, minColumnWidthOwner)
				assert.GreaterOrEqual(t, result.repo, minColumnWidthRepo)
				assert.GreaterOrEqual(t, result.tType, minColumnWidthType)
			},
		},
		{
			name: "respects minimum column widths",
			widths: columnWidths{
				status: 1,
				owner:  100,
				repo:   100,
				tType:  100,
			},
			termWidth: 50, // Very narrow terminal.
			checkFunc: func(t *testing.T, result columnWidths) {
				// Should not go below minimum widths.
				assert.GreaterOrEqual(t, result.owner, minColumnWidthOwner)
				assert.GreaterOrEqual(t, result.repo, minColumnWidthRepo)
				assert.GreaterOrEqual(t, result.tType, minColumnWidthType)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := applyColumnPaddingAndTruncation(tt.widths, tt.termWidth)
			tt.checkFunc(t, result)
		})
	}
}

// TestGetToolVersion tests the getToolVersion function.
func TestGetToolVersion(t *testing.T) {
	tests := []struct {
		name      string
		fullName  string
		repoName  string
		tools     map[string][]string
		foundFull bool
		foundRepo bool
		want      string
	}{
		{
			name:      "returns version from full name",
			fullName:  "hashicorp/terraform",
			repoName:  "terraform",
			tools:     map[string][]string{"hashicorp/terraform": {"1.5.0", "1.4.0"}},
			foundFull: true,
			foundRepo: false,
			want:      "1.5.0",
		},
		{
			name:      "returns version from repo name when full not found",
			fullName:  "hashicorp/terraform",
			repoName:  "terraform",
			tools:     map[string][]string{"terraform": {"1.5.0"}},
			foundFull: false,
			foundRepo: true,
			want:      "1.5.0",
		},
		{
			name:      "returns empty when no versions",
			fullName:  "hashicorp/terraform",
			repoName:  "terraform",
			tools:     map[string][]string{"hashicorp/terraform": {}},
			foundFull: true,
			foundRepo: false,
			want:      "",
		},
		{
			name:      "returns empty when not found",
			fullName:  "hashicorp/terraform",
			repoName:  "terraform",
			tools:     map[string][]string{},
			foundFull: false,
			foundRepo: false,
			want:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toolVersions := &toolchain.ToolVersions{
				Tools: tt.tools,
			}
			got := getToolVersion(tt.fullName, tt.repoName, toolVersions, tt.foundFull, tt.foundRepo)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestRenderToolsTable tests the renderToolsTable function.
func TestRenderToolsTable(t *testing.T) {
	rows := []toolRow{
		{
			status:      statusIndicator,
			owner:       "hashicorp",
			repo:        "terraform",
			toolType:    "github_release",
			isInstalled: true,
			isInConfig:  true,
		},
		{
			status:      " ",
			owner:       "kubernetes",
			repo:        "kubectl",
			toolType:    "github_release",
			isInstalled: false,
			isInConfig:  false,
		},
	}

	widths := columnWidths{
		status: 3,
		owner:  15,
		repo:   15,
		tType:  15,
	}

	result := renderToolsTable(rows, widths)

	// Should contain table output.
	assert.NotEmpty(t, result)
	assert.Contains(t, result, "OWNER")
	assert.Contains(t, result, "REPO")
	assert.Contains(t, result, "TYPE")
	assert.Contains(t, result, "hashicorp")
	assert.Contains(t, result, "terraform")
	assert.Contains(t, result, "kubernetes")
	assert.Contains(t, result, "kubectl")
}

// TestBuildToolRows tests the buildToolRows function.
func TestBuildToolRows(t *testing.T) {
	tools := []*toolchainregistry.Tool{
		{
			RepoOwner: "hashicorp",
			RepoName:  "terraform",
			Type:      "github_release",
		},
		{
			RepoOwner: "kubernetes",
			RepoName:  "kubectl",
			Type:      "github_release",
		},
	}

	// Test with nil toolVersions.
	rows, widths := buildToolRows(tools, nil, nil)

	assert.Len(t, rows, 2)
	assert.Equal(t, "hashicorp", rows[0].owner)
	assert.Equal(t, "terraform", rows[0].repo)
	assert.Equal(t, "kubernetes", rows[1].owner)
	assert.Equal(t, "kubectl", rows[1].repo)

	// Check widths were calculated.
	assert.GreaterOrEqual(t, widths.owner, len("hashicorp"))
	assert.GreaterOrEqual(t, widths.repo, len("terraform"))
}

// TestBuildSingleToolRow tests the buildSingleToolRow function.
func TestBuildSingleToolRow(t *testing.T) {
	tool := &toolchainregistry.Tool{
		RepoOwner: "hashicorp",
		RepoName:  "terraform",
		Type:      "github_release",
	}

	// Test with nil toolVersions.
	row := buildSingleToolRow(tool, nil, nil)

	assert.Equal(t, "hashicorp", row.owner)
	assert.Equal(t, "terraform", row.repo)
	assert.Equal(t, "github_release", row.toolType)
	assert.False(t, row.isInstalled)
	assert.False(t, row.isInConfig)
	assert.Equal(t, " ", row.status) // Not in config, should be space.
}

// TestColumnWidths tests the columnWidths struct.
func TestColumnWidths(t *testing.T) {
	widths := columnWidths{
		status: 1,
		owner:  10,
		repo:   15,
		tType:  20,
	}

	assert.Equal(t, 1, widths.status)
	assert.Equal(t, 10, widths.owner)
	assert.Equal(t, 15, widths.repo)
	assert.Equal(t, 20, widths.tType)
}

// TestDisplayTableParams tests the displayTableParams struct.
func TestDisplayTableParams(t *testing.T) {
	params := &displayTableParams{
		registryName: "aqua",
		pagerEnabled: true,
	}

	assert.Equal(t, "aqua", params.registryName)
	assert.True(t, params.pagerEnabled)
}

// TestCheckToolStatus tests the checkToolStatus function.
func TestCheckToolStatus(t *testing.T) {
	tests := getToolStatusTestCases()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			installer := toolchain.NewInstaller()
			inConfig, installed := checkToolStatus(tt.tool, tt.toolVersions, installer)
			assert.Equal(t, tt.wantInConfig, inConfig)
			assert.Equal(t, tt.wantInstall, installed)
		})
	}
}

// TestBuildSingleToolRow_WithToolVersions tests buildSingleToolRow with actual toolVersions.
func TestBuildSingleToolRow_WithToolVersions(t *testing.T) {
	tests := []struct {
		name         string
		tool         *toolchainregistry.Tool
		toolVersions *toolchain.ToolVersions
		wantInConfig bool
	}{
		{
			name: "tool in config by full name",
			tool: &toolchainregistry.Tool{
				RepoOwner: "hashicorp",
				RepoName:  "terraform",
				Type:      "github_release",
			},
			toolVersions: &toolchain.ToolVersions{
				Tools: map[string][]string{
					"hashicorp/terraform": {"1.5.0"},
				},
			},
			wantInConfig: true,
		},
		{
			name: "tool in config by repo name",
			tool: &toolchainregistry.Tool{
				RepoOwner: "hashicorp",
				RepoName:  "terraform",
				Type:      "github_release",
			},
			toolVersions: &toolchain.ToolVersions{
				Tools: map[string][]string{
					"terraform": {"1.5.0"},
				},
			},
			wantInConfig: true,
		},
		{
			name: "tool not in config",
			tool: &toolchainregistry.Tool{
				RepoOwner: "hashicorp",
				RepoName:  "terraform",
				Type:      "github_release",
			},
			toolVersions: &toolchain.ToolVersions{
				Tools: map[string][]string{
					"other/tool": {"1.0.0"},
				},
			},
			wantInConfig: false,
		},
		{
			name: "nil tools map",
			tool: &toolchainregistry.Tool{
				RepoOwner: "hashicorp",
				RepoName:  "terraform",
				Type:      "github_release",
			},
			toolVersions: &toolchain.ToolVersions{
				Tools: nil,
			},
			wantInConfig: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			installer := toolchain.NewInstaller()
			row := buildSingleToolRow(tt.tool, tt.toolVersions, installer)

			assert.Equal(t, tt.tool.RepoOwner, row.owner)
			assert.Equal(t, tt.tool.RepoName, row.repo)
			assert.Equal(t, tt.tool.Type, row.toolType)
			assert.Equal(t, tt.wantInConfig, row.isInConfig)
		})
	}
}

// TestGetTerminalWidthOrDefault tests the getTerminalWidthOrDefault function.
func TestGetTerminalWidthOrDefault(t *testing.T) {
	// In test environments, terminal width detection often fails.
	// The function should return either the actual terminal width or the default.
	width := getTerminalWidthOrDefault()
	assert.GreaterOrEqual(t, width, defaultTerminalWidth, "should return at least default width")
}

// TestBuildToolRows_WithToolVersions tests buildToolRows with actual toolVersions.
func TestBuildToolRows_WithToolVersions(t *testing.T) {
	tools := []*toolchainregistry.Tool{
		{
			RepoOwner: "hashicorp",
			RepoName:  "terraform",
			Type:      "github_release",
		},
		{
			RepoOwner: "kubernetes",
			RepoName:  "kubectl",
			Type:      "github_release",
		},
	}

	toolVersions := &toolchain.ToolVersions{
		Tools: map[string][]string{
			"hashicorp/terraform": {"1.5.0"},
		},
	}

	installer := toolchain.NewInstaller()
	rows, widths := buildToolRows(tools, toolVersions, installer)

	assert.Len(t, rows, 2)
	assert.True(t, rows[0].isInConfig, "terraform should be in config")
	assert.False(t, rows[1].isInConfig, "kubectl should not be in config")
	assert.GreaterOrEqual(t, widths.owner, len("hashicorp"))
}

// TestApplyColumnPaddingAndTruncation_ExtremeWidths tests extreme width scenarios.
func TestApplyColumnPaddingAndTruncation_ExtremeWidths(t *testing.T) {
	tests := []struct {
		name      string
		widths    columnWidths
		termWidth int
	}{
		{
			name: "extremely narrow terminal",
			widths: columnWidths{
				status: 1,
				owner:  50,
				repo:   50,
				tType:  50,
			},
			termWidth: 20,
		},
		{
			name: "very wide terminal",
			widths: columnWidths{
				status: 1,
				owner:  10,
				repo:   10,
				tType:  10,
			},
			termWidth: 500,
		},
		{
			name: "zero terminal width",
			widths: columnWidths{
				status: 1,
				owner:  10,
				repo:   10,
				tType:  10,
			},
			termWidth: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := applyColumnPaddingAndTruncation(tt.widths, tt.termWidth)
			// Should never panic and should maintain minimum widths.
			assert.GreaterOrEqual(t, result.owner, minColumnWidthOwner)
			assert.GreaterOrEqual(t, result.repo, minColumnWidthRepo)
			assert.GreaterOrEqual(t, result.tType, minColumnWidthType)
		})
	}
}

// TestListCommand_DefaultFlagValues tests default values for list command flags.
func TestListCommand_DefaultFlagValues(t *testing.T) {
	t.Run("limit default is 50", func(t *testing.T) {
		flag := listCmd.Flags().Lookup("limit")
		require.NotNil(t, flag)
		assert.Equal(t, "50", flag.DefValue)
	})

	t.Run("offset default is 0", func(t *testing.T) {
		flag := listCmd.Flags().Lookup("offset")
		require.NotNil(t, flag)
		assert.Equal(t, "0", flag.DefValue)
	})

	t.Run("format default is table", func(t *testing.T) {
		flag := listCmd.Flags().Lookup("format")
		require.NotNil(t, flag)
		assert.Equal(t, "table", flag.DefValue)
	})

	t.Run("sort default is name", func(t *testing.T) {
		flag := listCmd.Flags().Lookup("sort")
		require.NotNil(t, flag)
		assert.Equal(t, "name", flag.DefValue)
	})
}

// TestListCommand_CommandStructure tests the list command structure.
func TestListCommand_CommandStructure(t *testing.T) {
	t.Run("command has correct use string", func(t *testing.T) {
		assert.Contains(t, listCmd.Use, "list")
	})

	t.Run("command has short description", func(t *testing.T) {
		assert.NotEmpty(t, listCmd.Short)
	})

	t.Run("command has long description", func(t *testing.T) {
		assert.NotEmpty(t, listCmd.Long)
		assert.Contains(t, listCmd.Long, "registry")
	})

	t.Run("command has RunE function", func(t *testing.T) {
		assert.NotNil(t, listCmd.RunE)
	})

	t.Run("command accepts max 1 argument", func(t *testing.T) {
		assert.NotNil(t, listCmd.Args)
	})
}
